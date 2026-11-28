package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/ansicht"
	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/jsonfile"
	"github.com/mdopp/homes/internal/papierkorb"
	"github.com/mdopp/homes/internal/usage"
)

// Haus was renamed Homes (laib#1021), and some data kept the old name
// where it was stored. Owner 2026-11-28: „Alles alte kann weg. Keine
// Rückwärtskompatibilität notwendig.“ Homes reads only the new names; the
// steps here rewrite what is still stored under the old ones, once, at
// start. Each step done leaves its marker in HOMES_DATA/umbenannt/, so a
// later start skips it; a step that fails is tried again on the next run.
// These are the only places the old names are written in Go.
const umbenanntDir = "umbenannt"

const (
	altesKonto  = "haus"                        // the household's Radicale account before the rename
	alteProdID  = "-//mdopp//Haus Haushalt//DE" // the PRODID Homes wrote on its cards (Card.AsWritten)
	alteApp     = "haus"                        // Homes' name in usage-metrics
	zaehlTage   = 90                            // usage-metrics' retention: every day it still holds
	zaehlFertig = "zaehlungen-tage.json"        // the days already copied, in umbenanntDir
)

// umbenennen is where the steps keep their markers and what they rewrite.
type umbenennen struct {
	dir     string         // HOMES_DATA
	konto   string         // HOMES_RADICALE_USER, what /haus/… becomes
	views   *ansicht.Store // the calendar views
	korb    *papierkorb.Korb
	karten  *caldav.Client // the household's address books; nil: module kontakte off
	zaehler *usage.Counter // nil: nothing counted
}

func (u umbenennen) marker(step string) string { return filepath.Join(u.dir, umbenanntDir, step) }

// einmal runs step unless its marker is there, and leaves the marker once
// it went through; what it changed is logged.
func (u umbenennen) einmal(step string, run func() (int64, error)) error {
	if _, err := os.Stat(u.marker(step)); err == nil {
		return nil
	}
	n, err := run()
	if err != nil {
		return fmt.Errorf("%s: %w", step, err)
	}
	if err := os.MkdirAll(filepath.Join(u.dir, umbenanntDir), 0o700); err != nil {
		return err
	}
	log.Printf("homes: umbenannt (laib#1021): %s, %d changed, once", step, n)
	return os.WriteFile(u.marker(step), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
}

// neuerPfad is a collection path under the old account as under the
// current one; any other path as it is.
func (u umbenennen) neuerPfad(p string) string {
	if u.konto == "" || u.konto == altesKonto {
		return p
	}
	if rest, ok := strings.CutPrefix(p, "/"+altesKonto+"/"); ok {
		return "/" + u.konto + "/" + rest
	}
	return p
}

// lokal is the steps on Homes' own files: before the pages answer.
func (u umbenennen) lokal() error {
	return errors.Join(
		u.einmal("ansicht", func() (int64, error) { n, err := u.views.Umschreiben(u.neuerPfad); return int64(n), err }),
		u.einmal("papierkorb", func() (int64, error) {
			n, err := u.korb.Umschreiben("kalender", func(d json.RawMessage) (json.RawMessage, bool) {
				var m map[string]json.RawMessage
				var p string
				if json.Unmarshal(d, &m) != nil || json.Unmarshal(m["path"], &p) != nil || u.neuerPfad(p) == p {
					return d, false
				}
				m["path"], _ = json.Marshal(u.neuerPfad(p))
				b, err := json.Marshal(m)
				return b, err == nil
			})
			return int64(n), err
		}))
}

// entfernt is the steps on the other services: the cards' PRODID in
// Radicale, the counts in usage-metrics.
func (u umbenennen) entfernt(ctx context.Context) error {
	var errs []error
	if u.karten != nil {
		errs = append(errs, u.einmal("prodid", func() (int64, error) {
			cards, partial, err := u.karten.Cards(ctx)
			if err != nil {
				return 0, err
			}
			var n int64
			for _, cd := range cards {
				ok, err := u.karten.ProdIDErsetzen(ctx, cd, alteProdID)
				if err != nil {
					return n, fmt.Errorf("%s: %w", cd.UID, err)
				}
				if ok {
					n++
				}
			}
			if partial {
				return n, errors.New("the address books were read only in part")
			}
			return n, nil
		}))
	}
	if u.zaehler != nil {
		errs = append(errs, u.einmal("zaehlungen", func() (int64, error) {
			path := filepath.Join(u.dir, umbenanntDir, zaehlFertig)
			fertig := map[string]bool{}
			if b, err := os.ReadFile(path); err == nil {
				json.Unmarshal(b, &fertig)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return 0, err
			}
			return u.zaehler.Umziehen(ctx, alteApp, zaehlTage, fertig, func(string) error { return jsonfile.Save(path, fertig) })
		}))
	}
	return errors.Join(errs...)
}

// umbenennenStart runs the local steps now, before a page reads the
// files, and the remote ones in the background until they went through,
// with a growing pause while a service does not answer.
func umbenennenStart(ctx context.Context, u umbenennen) {
	if err := u.lokal(); err != nil {
		log.Printf("homes: umbenennen (laib#1021): %v (again at the next start)", err)
	}
	go u.entferntBisFertig(ctx)
}

func (u umbenennen) entferntBisFertig(ctx context.Context) {
	for wait := 10 * time.Second; ; wait = min(wait*2, 10*time.Minute) {
		tctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err := u.entfernt(tctx)
		cancel()
		if err == nil {
			return
		}
		log.Printf("homes: umbenennen (laib#1021): %v (again in %v)", err, wait)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

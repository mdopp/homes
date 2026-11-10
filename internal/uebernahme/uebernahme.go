// Package uebernahme writes a relation the person answered in their own
// message archive (mdopp/archiv#27, homes ADR 0002) into the address
// books. The person's tap in their own open archive is their consent
// (owner 2026-11-10): the archive posts the line to Pfad at once, server to
// server, and its Rückgängig takes it back the same way (weg). It comes
// from the box (laib templates/archiv: ARCHIV_HOMES_INTERN, Homes' port on
// host.containers.internal), never through Caddy: a request that came
// through the public route (X-Homes-Proxy, a forwarded address or a
// login) is refused. The body (archiv apps/archiv/uebernahme.go, version 2):
//
//	d = base64url(JSON {"person", "am", "nonce", "weg"?, "zeilen": [{"x", "y", "art"}]}), no padding
//	s = base64url(HMAC-SHA256(<that person's archive secret>, "archiv-uebernahme-2." + d)), no padding
//
// s is checked with the secret of the person d names (HOMES_ARCHIV_SECRET_
// <PERSON>): an archive writes only for its own person. am within gilt of
// now and not before Homes started, each nonce once: no old or replayed
// packet writes. Where a line goes (owner 2026-11-10: where the contact
// lives): both ends with a household card → both household cards, each
// naming the other by UID only; else → the person's own phone, on each end
// that is a phone contact, by name. A household card never gets a name.
package uebernahme

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/netz"
)

const (
	// Pfad is where the archive posts, on Homes' own port only.
	Pfad    = "/intern/uebernahme"
	version = "archiv-uebernahme-2."
	gilt    = 2 * time.Minute
	// Haushalt and Handy are where a line goes.
	Haushalt = "haushalt"
	Handy    = "handy"
)

// Ende is one end of a line: its name and its cards' UIDs.
type Ende struct {
	Name     string `json:"name"`
	Haushalt string `json:"haushalt,omitempty"`
	Handy    string `json:"handy,omitempty"`
}

// Zeile is one relation: X is Art of Y (netz.Arten).
type Zeile struct {
	X   Ende   `json:"x"`
	Y   Ende   `json:"y"`
	Art string `json:"art"`
}

// Paket is what the archive sent.
type Paket struct {
	Person string    `json:"person"`
	Am     time.Time `json:"am"`
	Nonce  string    `json:"nonce"`
	Weg    bool      `json:"weg,omitempty"`
	Zeilen []Zeile   `json:"zeilen"`
}

// ErrUngueltig is a packet not signed for the person it names, out of
// time, or without a line Homes can write.
var ErrUngueltig = errors.New("uebernahme: kein gültiges Paket des Archivs")

// Lesen checks d and s with the secret of the person d names, at now.
func Lesen(d, s string, geheim map[string]string, now time.Time) (Paket, error) {
	var p Paket
	b, err := base64.RawURLEncoding.DecodeString(d)
	if err != nil || json.Unmarshal(b, &p) != nil || p.Person == "" || geheim[p.Person] == "" {
		return Paket{}, ErrUngueltig
	}
	m := hmac.New(sha256.New, []byte(geheim[p.Person]))
	m.Write([]byte(version + d))
	sig, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || !hmac.Equal(sig, m.Sum(nil)) || len(p.Nonce) < 16 || now.Sub(p.Am) > gilt || p.Am.Sub(now) > gilt/4 {
		return Paket{}, ErrUngueltig
	}
	p.Zeilen = slices.DeleteFunc(p.Zeilen, func(z Zeile) bool { return Wohin(z) == "" || !slices.Contains(netz.Arten, z.Art) })
	if len(p.Zeilen) == 0 {
		return Paket{}, ErrUngueltig
	}
	return p, nil
}

// Wohin is where z goes: Haushalt, Handy, "" nowhere.
func Wohin(z Zeile) string {
	switch {
	case z.X.Haushalt != "" && z.Y.Haushalt != "":
		return Haushalt
	case z.X.Handy != "" || z.Y.Handy != "":
		return Handy
	}
	return ""
}

// Schreiben writes z (weg: takes it back): into both household cards
// (kontakte), or through handy (the corrections the person's Homes app
// fetches) to wer's phone; each end gets the other, the kind seen from it.
func Schreiben(ctx context.Context, kontakte *caldav.Client, handy *kontaktantwort.Store, wer string, z Zeile, weg bool) error {
	wohin := Wohin(z)
	if wohin == Haushalt && kontakte == nil || wohin == Handy && handy == nil {
		return ErrUngueltig
	}
	var errs []error
	for _, s := range []struct {
		auf, mit Ende
		art      string
	}{{z.Y, z.X, z.Art}, {z.X, z.Y, netz.Gegen(z.Art)}} {
		switch {
		case wohin == Haushalt:
			wert := "urn:uuid:" + strings.TrimPrefix(s.mit.Haushalt, "urn:uuid:")
			ks := []caldav.Korrektur{{Prop: "RELATED", Alt: s.mit.Name}, // a line by name to the other goes
				{Prop: "RELATED", Alt: wert, Neu: wert, Art: cmp.Or(netz.Related(s.art), "kin")}}
			if weg {
				ks = []caldav.Korrektur{{Prop: "RELATED", Alt: wert}}
			}
			_, err := kontakte.CorrectShared(ctx, s.auf.Haushalt, ks)
			errs = append(errs, err)
		case s.auf.Handy != "":
			art := map[bool]string{false: s.art}[weg] // no kind: the phone takes the relation away
			errs = append(errs, handy.Handy(wer, gesichter.HandyAenderung{UID: s.auf.Handy, AltVorname: s.auf.Name, Mit: s.mit.Name, Art: art}))
		}
	}
	return errors.Join(errs...)
}

// Empfang is Homes' end of the archive's write (Pfad): each archive's
// secret by person, the household book and the phones' corrections.
type Empfang struct {
	Geheim   map[string]string
	Kontakte *caldav.Client
	Handy    *kontaktantwort.Store
	Now      func() time.Time
	Seit     time.Time // Homes' start: a packet made before it may have been taken already

	mu      sync.Mutex
	gesehen map[string]time.Time // the nonces taken, until their packet is out of time
}

// ServeHTTP takes one packet and says how it ended, as JSON.
func (e *Empfang) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	antwort := func(code int, grund string) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]any{"ok": code == http.StatusOK, "grund": grund})
	}
	switch {
	case r.Header.Get("X-Homes-Proxy") != "" || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Forwarded-Host") != "" || r.Header.Get("Remote-User") != "":
		antwort(http.StatusForbidden, "nur von der Box, nicht über die öffentliche Route")
		return
	case r.Method != http.MethodPost:
		antwort(http.StatusMethodNotAllowed, "nur POST")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	now := e.Now()
	p, err := Lesen(r.PostFormValue("d"), r.PostFormValue("s"), e.Geheim, now)
	if err != nil {
		antwort(http.StatusUnauthorized, "kein gültiges Paket des Archivs")
		return
	}
	if !e.neu(p, now) {
		antwort(http.StatusConflict, "dieses Paket wurde schon genommen")
		return
	}
	for _, z := range p.Zeilen {
		if err := Schreiben(r.Context(), e.Kontakte, e.Handy, p.Person, z, p.Weg); err != nil {
			log.Printf("homes: uebernahme: a line for %s not written: %v", p.Person, err)
			antwort(http.StatusBadGateway, "ins Adressbuch ließ sich nicht schreiben")
			return
		}
	}
	antwort(http.StatusOK, "")
}

// neu takes p's nonce once; a packet made before Homes started is not new.
func (e *Empfang) neu(p Paket, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gesehen == nil {
		e.gesehen = map[string]time.Time{}
	}
	for n, bis := range e.gesehen {
		if now.After(bis) {
			delete(e.gesehen, n)
		}
	}
	if _, da := e.gesehen[p.Nonce]; da || p.Am.Before(e.Seit) {
		return false
	}
	e.gesehen[p.Nonce] = p.Am.Add(gilt)
	return true
}

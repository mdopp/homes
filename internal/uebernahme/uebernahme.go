// Package uebernahme writes a relation the person answered in their own
// message archive (mdopp/archiv#27, homes ADR 0002) into the address
// books, and only after the person confirmed it on Homes' page
// (pages.Uebernehmen). The archive asks; Homes writes.
//
// The archive hands a packet in a POST body (archiv apps/archiv/uebernahme.go,
// version 1):
//
//	d = base64url(JSON {"person", "bis", "zeilen": [{"x", "y", "art"}]}), no padding
//	s = base64url(HMAC-SHA256(<that archive's secret>, "archiv-uebernahme-1." + d)), no padding
//
// Lesen takes it only for the login named person, signed with that
// person's archive secret (HOMES_ARCHIV_SECRET_<PERSON>), before bis and
// no more than two hours ahead: nobody else, and no page without the
// secret, can make one.
//
// Where a line goes (owner 2026-11-10: where the contact lives): both
// ends with a household card → both household cards, each naming the
// other by UID only; else → the person's own phone, on each end that is a
// phone contact, by name. A household card never gets a name here.
package uebernahme

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/netz"
)

const (
	version = "archiv-uebernahme-1."
	gilt    = 2 * time.Hour
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
	Bis    time.Time `json:"bis"`
	Zeilen []Zeile   `json:"zeilen"`
}

// ErrUngueltig is a packet not signed for this person, too old, or without
// a line Homes can write.
var ErrUngueltig = errors.New("uebernahme: kein gültiges Paket des Archivs")

// Lesen checks d and s with secret for person at now.
func Lesen(d, s, secret, person string, now time.Time) (Paket, error) {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(version + d))
	sig, err := base64.RawURLEncoding.DecodeString(s)
	var p Paket
	if secret == "" || person == "" || err != nil || !hmac.Equal(sig, m.Sum(nil)) {
		return p, ErrUngueltig
	}
	b, err := base64.RawURLEncoding.DecodeString(d)
	if err != nil || json.Unmarshal(b, &p) != nil || p.Person != person || !now.Before(p.Bis) || p.Bis.Sub(now) > gilt {
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

// Schreiben writes z: into both household cards (kontakte), or through
// handy (the corrections the person's Homes app fetches) to wer's phone;
// each end gets the other, the kind seen from it.
func Schreiben(ctx context.Context, kontakte *caldav.Client, handy *kontaktantwort.Store, wer string, z Zeile) error {
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
			_, err := kontakte.CorrectShared(ctx, s.auf.Haushalt, []caldav.Korrektur{{Prop: "RELATED", Alt: s.mit.Name}, // a line by name to the other goes
				{Prop: "RELATED", Alt: wert, Neu: wert, Art: cmp.Or(netz.Related(s.art), "kin")}})
			errs = append(errs, err)
		case s.auf.Handy != "":
			errs = append(errs, handy.Handy(wer, gesichter.HandyAenderung{UID: s.auf.Handy, AltVorname: s.auf.Name, Mit: s.mit.Name, Art: s.art}))
		}
	}
	return errors.Join(errs...)
}

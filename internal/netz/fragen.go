package netz

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/jsonfile"
	"github.com/mdopp/homes/internal/texte"
)

// The key owner's answers about a pair (owner 2026-11-09), given on a
// contact's page (aendern.go): kept per key owner in their own file,
// which only an answer writes, so the daily run cannot lose one given
// while it read. A kind becomes a relation of the pair in place of the
// phone books'. The questions „Wie steht X zu Y?“ and their suggestions
// moved into the person's archive (archiv#27 step 2, ADR 0002); what the
// person takes over from there, internal/uebernahme writes.

const antwortNam = "immich-netz-antworten.json"

// The answers that are no kind; Fremd (owner 2026-11-10): the two do not know each other, their tie leaves the net.
const (
	Keine   = "keine"
	Fremd   = "fremd"
	Spaeter = "spaeter"
)

// Arten are the kinds a question offers, in its order; „X ist <Art> von Y“.
var Arten = []string{"partner", "expartner", "eltern", "kind", "geschwister", "grosseltern", "enkel", "onkel", "neffe", "cousin",
	"schwiegereltern", "schwiegerkind", "schwager", "freund", "kollege", "nachbar", "bekannt", "verwandt"}

// gegen is a kind seen from the other end; one not here is its own.
var gegen = map[string]string{"eltern": "kind", "kind": "eltern", "grosseltern": "enkel", "enkel": "grosseltern", "onkel": "neffe",
	"neffe": "onkel", "schwiegereltern": "schwiegerkind", "schwiegerkind": "schwiegereltern"}

// related is a kind as vCard's RELATED TYPE (RFC 6350 6.6.6); kin the rest.
var related = map[string]string{"partner": "spouse", "eltern": "parent", "kind": "child", "geschwister": "sibling",
	"expartner": "acquaintance", "freund": "friend", "kollege": "colleague", "nachbar": "neighbor", "bekannt": "acquaintance"}

// Gegen is art seen from the other end; Related its vCard TYPE ("" kin).
func Gegen(art string) string   { return cmp.Or(gegen[art], art) }
func Related(art string) string { return related[art] }

// Wort is a kind as the family reads it (netz.art.<kind>).
func Wort(art string) string { return texte.T(texte.ID("netz.art." + art)) }

// antwort is the key owner's answer about a pair: X is Art of the other.
type antwort struct {
	X   string    `json:"x"`
	Art string    `json:"art"`
	Am  time.Time `json:"am"`
}

// ErrKeineFrage is a pair not asked about (any more).
var ErrKeineFrage = errors.New("netz: diese Frage gibt es nicht mehr")

func paarKey(a, b string) string { return min(a, b) + "|" + max(a, b) }

// merken keeps wer's answer about x and y (x is art of y; "" forgets it)
// and rebuilds; n.mu is held.
func (n *Netz) merken(wer, x, y, art string) error {
	if n.antworten[wer] == nil {
		n.antworten[wer] = map[string]antwort{}
	}
	n.antworten[wer][paarKey(x, y)] = antwort{X: x, Art: art, Am: n.now()}
	if art == "" {
		delete(n.antworten[wer], paarKey(x, y))
	}
	n.bauen()
	return jsonfile.Save(filepath.Join(n.G.Dir, antwortNam), n.antworten)
}

// Eintragen writes the kind art (X is that of Y; Keine, Fremd: none any more)
// into the cards of both: a household card's RELATED with its TYPE and the
// other household card's UID (a line by name to it goes); the key owner's
// own phone contact through the phone (gesichter.HandyAenderung), by name.
// A household card only ever names household contacts (owner 2026-11-10).
func (n *Netz) Eintragen(ctx context.Context, x, y Knoten, art string) error {
	weg := art == Keine || art == Fremd
	if !weg && !slices.Contains(Arten, art) {
		return nil
	}
	var errs []error
	for _, s := range []struct {
		auf, mit Knoten
		art      string
	}{{y, x, art}, {x, y, cmp.Or(gegen[art], art)}} {
		buch, uid, _ := strings.Cut(s.auf.Kontakt, "/")
		wert, typ := s.mit.Name, cmp.Or(related[s.art], "kin")
		ks := []caldav.Korrektur{}
		if u, ok := strings.CutPrefix(s.mit.Kontakt, "haushalt/"); ok {
			wert = "urn:uuid:" + strings.TrimPrefix(u, "urn:uuid:")
			ks = append(ks, caldav.Korrektur{Prop: "RELATED", Alt: s.mit.Name})
		}
		neu := map[bool]string{true: wert}[!weg]
		switch {
		case buch == "haushalt" && n.G.Kontakte != nil && (weg || strings.HasPrefix(s.mit.Kontakt, "haushalt/")):
			_, err := n.G.Kontakte.CorrectShared(ctx, uid, append(ks, caldav.Korrektur{Prop: "RELATED", Alt: wert, Neu: neu, Art: typ}))
			errs = append(errs, err)
		case buch == "privat" && n.G.Handy != nil && n.G.Wessen() != "":
			errs = append(errs, n.G.Handy(n.G.Wessen(), gesichter.HandyAenderung{UID: uid, AltVorname: s.auf.Name, Mit: s.mit.Name, Art: map[bool]string{true: s.art}[!weg]}))
		}
	}
	return errors.Join(errs...)
}

package netz

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/jsonfile"
	"github.com/mdopp/homes/internal/namen"
	"github.com/mdopp/homes/internal/texte"
)

// Beziehungen klären (owner 2026-11-09: „Frage, ähnlich wie bei den
// Fotos, auch den Beziehungsgrad ab. Mache gute Vorschläge und ein
// Dropdown zur Auswahl.“): a tie among either end's closest (as „Steht
// nahe“ shows them) that no phone book and no answer names a relation for
// is asked as „Wie steht X zu Y?“, the strongest first, with a kind
// suggested from the phone books and the photos (vorschlag). The answers
// are kept per key owner in their own file, which only an answer writes:
// the daily run never holds them, so it cannot lose one given while it
// read. A kind becomes a relation of the pair in place of the phone
// books'; „Keine Beziehung“ is never asked again, Überspringen last.

const (
	antwortNam = "immich-netz-antworten.json"
	fragenJe   = 5 // a tie is asked about while among either end's closest five
	freundTage = 5 // photo days from which a tie without a sign of family suggests Freund/in
)

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

// Wort is a kind as the family reads it (netz.art.<kind>).
func Wort(art string) string { return texte.T(texte.ID("netz.art." + art)) }

// antwort is the key owner's answer about a pair: X is Art of the other.
type antwort struct {
	X   string    `json:"x"`
	Art string    `json:"art"`
	Am  time.Time `json:"am"`
}

// merkmale are what a node's contacts and Immich say for the suggestions.
type merkmale struct {
	family   string
	geb      time.Time
	strassen []string
}

// Frage is „Wie steht X zu Y?“; Ich: Y is the key owner. Vorschlag is
// the kind suggested ("" none), Weil why.
type Frage struct {
	X, Y            Knoten
	Ich             bool
	Kante           Kante
	Vorschlag, Weil string
}

// ErrKeineFrage is a pair not asked about (any more).
var ErrKeineFrage = errors.New("netz: diese Frage gibt es nicht mehr")

func paarKey(a, b string) string { return min(a, b) + "|" + max(a, b) }

// strassen are a card's streets without the house number, each with its
// postcode: the same one of another house is a neighbour.
func strassen(ct caldav.Contact) (out []string) {
	for _, a := range ct.Postal {
		var ws []string
		for _, w := range strings.Fields(a.Street) {
			if !strings.ContainsFunc(w, unicode.IsDigit) {
				ws = append(ws, w)
			}
		}
		if s := namen.Plain(strings.Join(ws, "")); s != "" && a.Code != "" {
			out = append(out, s+"|"+a.Code)
		}
	}
	return out
}

func datum(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }

// nachname is a family name compared: N's, else the name's last word;
// none for „Oma Hilde“ (a given name).
func nachname(family, name string) string {
	ws := strings.Fields(name)
	if family == "" && len(ws) > 1 && rufnamen[strings.ToLower(ws[0])] == "" {
		family = ws[len(ws)-1]
	}
	return namen.Plain(family)
}

// merkmaleBauen gathers each node's merkmale, a contact's over Immich's; n.mu is held.
func (n *Netz) merkmaleBauen() {
	n.merkmale = map[string]merkmale{}
	for id, name := range n.st.Namen {
		n.merkmale[id] = merkmale{family: nachname("", name), geb: datum(n.st.Geb[id])}
	}
	for _, k := range n.st.Kontakte {
		id := cmp.Or(k.Person, k.Key)
		m := n.merkmale[id]
		m.family = cmp.Or(nachname(k.Family, k.Name), m.family)
		if d := datum(k.Geb); !d.IsZero() {
			m.geb = d
		}
		m.strassen = append(m.strassen, k.Strassen...)
		n.merkmale[id] = m
	}
}

// gesagtDazu puts the key owner's answers into alle, each a relation in
// place of what the phone books say of the pair; n.mu is held.
func (n *Netz) gesagtDazu(alle map[[2]string]*Kante, nicht map[string]bool) {
	for key, a := range n.antworten[n.G.Wessen()] {
		x, y, _ := strings.Cut(key, "|")
		if a.X == y {
			x, y = y, x
		}
		_, dx := n.knoten[x]
		p := [2]string{min(x, y), max(x, y)}
		if a.Art == Fremd {
			delete(alle, p)
		}
		if _, dy := n.knoten[y]; !dx || !dy || nicht[x] || nicht[y] || !slices.Contains(Arten, a.Art) {
			continue
		}
		k := alle[p]
		if k == nil {
			k = &Kante{A: p[0], B: p[1]}
			alle[p] = k
		}
		if !slices.ContainsFunc(k.Fest, func(f Fest) bool { return f.Art == Beziehung }) {
			k.Staerke, k.Rang = k.Staerke+festGewicht[Beziehung], k.Rang+festGewicht[Beziehung]
		}
		k.Fest = append(slices.DeleteFunc(k.Fest, func(f Fest) bool { return f.Art == Beziehung }), Fest{A: y, B: x, Art: Beziehung, Wie: a.Art, Gesagt: true})
	}
}

// gesagtGrund is an answer as a reason seen from mitte: „Kind (von dir angegeben)“.
func gesagtGrund(mitte string, f Fest) string {
	art := f.Wie
	if f.A != mitte {
		art = cmp.Or(gegen[art], art)
	}
	return fmt.Sprintf(texte.T(texte.NetzGesagt), Kurz(art))
}

// Fragen are the open questions, the strongest tie first, the skipped
// last; none about a hidden person.
func (n *Netz) Fragen() []Frage {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	return n.offen()
}

// offen are the open questions; n.mu is held.
func (n *Netz) offen() (out []Frage) {
	wer := n.G.Wessen()
	weg, gesagt := n.verborgen[wer], n.antworten[wer]
	gefragt := map[*Kante]bool{}
	for _, ks := range n.kanten {
		ks = slices.DeleteFunc(slices.Clone(ks), func(k *Kante) bool { return weg[k.A] != "" || weg[k.B] != "" })
		slices.SortFunc(ks, func(a, b *Kante) int { return cmp.Compare(b.Rang, a.Rang) })
		for _, k := range ks[:min(fragenJe, len(ks))] {
			gefragt[k] = !slices.ContainsFunc(k.Fest, func(f Fest) bool { return f.Art == Beziehung }) && cmp.Or(gesagt[paarKey(k.A, k.B)].Art, Spaeter) == Spaeter
		}
	}
	for k, ja := range gefragt {
		if ja {
			x, y := k.A, k.B
			if x == n.ich {
				x, y = y, x
			}
			art, weil := n.vorschlag(x, y, k)
			out = append(out, Frage{X: n.knoten[x], Y: n.knoten[y], Ich: y == n.ich, Kante: *k, Vorschlag: art, Weil: weil})
		}
	}
	slices.SortFunc(out, func(a, b Frage) int {
		sa, sb := gesagt[paarKey(a.X.ID, a.Y.ID)].Art == Spaeter, gesagt[paarKey(b.X.ID, b.Y.ID)].Art == Spaeter
		return cmp.Or(cmp.Compare(btoi(sa), btoi(sb)), cmp.Compare(b.Kante.Rang, a.Kante.Rang), strings.Compare(paarKey(a.X.ID, a.Y.ID), paarKey(b.X.ID, b.Y.ID)))
	})
	return out
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// rufnamen are a name's first words that say the relation to the key owner.
var rufnamen = map[string]string{"oma": "grosseltern", "opa": "grosseltern", "tante": "onkel", "onkel": "onkel"}

// vorschlag is the kind X likely is of Y, and why; "" none. Deterministic,
// no gender guessed: a „Oma“ in the name toward the key owner; the same
// family name and address make family, the age gap its kind; the same
// address and a similar age Partner/in; the same firm Kollege/Kollegin;
// the same street, another house Nachbar/in; many photo days without a
// sign of family (both family names known and different) Freund/in.
func (n *Netz) vorschlag(x, y string, k *Kante) (string, string) {
	mx, my := n.merkmale[x], n.merkmale[y]
	fest := func(art string) bool { return slices.ContainsFunc(k.Fest, func(f Fest) bool { return f.Art == art }) }
	familie := mx.family != "" && mx.family == my.family
	jahre := -1.0
	if !mx.geb.IsZero() && !my.geb.IsZero() {
		jahre = math.Abs(my.geb.Sub(mx.geb).Hours()) / 24 / 365.25
	}
	aelter := mx.geb.Before(my.geb)
	abstand := fmt.Sprintf(texte.T(texte.NetzWeilJahre), int(math.Round(jahre)))
	erstes, _, _ := strings.Cut(n.knoten[x].Name, " ")
	switch {
	case y == n.ich && rufnamen[strings.ToLower(erstes)] != "":
		return rufnamen[strings.ToLower(erstes)], fmt.Sprintf(texte.T(texte.NetzWeilName), erstes)
	case fest(Adresse) && familie && jahre < 0:
		return "verwandt", texte.T(texte.NetzWeilFamilie)
	case fest(Adresse) && familie:
		weil := texte.T(texte.NetzWeilFamilie) + ", " + abstand
		juenger := n.now().Sub(map[bool]time.Time{true: my.geb, false: mx.geb}[aelter]).Hours() / 24 / 365.25
		switch {
		case jahre < 12 && juenger < 25:
			return "geschwister", weil
		case jahre < 12:
			return "partner", weil
		case jahre < 16:
			return "verwandt", weil
		case jahre <= 45:
			return map[bool]string{true: "eltern", false: "kind"}[aelter], weil
		}
		return map[bool]string{true: "grosseltern", false: "enkel"}[aelter], weil
	case fest(Adresse) && jahre >= 0 && jahre < 12:
		return "partner", texte.T(texte.NetzAdresse) + ", " + abstand
	case fest(Firma):
		return "kollege", texte.T(texte.NetzFirma)
	case !fest(Adresse) && slices.ContainsFunc(mx.strassen, func(s string) bool { return slices.Contains(my.strassen, s) }):
		return "nachbar", texte.T(texte.NetzWeilNachbar)
	case mx.family != "" && my.family != "" && !familie && k.Tage >= freundTage:
		return "freund", fmt.Sprintf(texte.T(texte.NetzWeilFotos), texte.Anzahl(k.Tage, "Tag", "Tage"))
	}
	return "", ""
}

// Beantworten keeps the key owner's answer about x and y: a kind (x is
// that of y), Keine, Fremd or Spaeter; the question as it was asked, x first.
func (n *Netz) Beantworten(x, y, art string) (Frage, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	wer, fs := n.G.Wessen(), n.offen()
	i := slices.IndexFunc(fs, func(f Frage) bool { return paarKey(f.X.ID, f.Y.ID) == paarKey(x, y) })
	if wer == "" || i < 0 || art != Keine && art != Fremd && art != Spaeter && !slices.Contains(Arten, art) {
		return Frage{}, ErrKeineFrage
	}
	f := fs[i]
	if f.X.ID != x {
		f.X, f.Y = f.Y, f.X
		f.Ich = f.Y.ID == n.ich
	}
	return f, n.merken(wer, x, y, art)
}

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
// own phone contact through the phone (owner 2026-11-09,
// gesichter.HandyAenderung), by name. A household card never gets a
// person who is not in the household book: the family would read the key
// owner's phone contact or photo person (owner 2026-11-10: „Das Leck bitte
// schließen“); taking such a line away still works.
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

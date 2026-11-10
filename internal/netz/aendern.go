package netz

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/namen"
	"github.com/mdopp/homes/internal/texte"
)

// Änderbar (owner 2026-11-09: „Baue den Rest inkl. Änderbarkeit und mache
// es zum Teil des Kontaktformulars.“): every relation of a node, the key
// owner's answers over the phone books', is listed on its contact's page
// and changed or taken away there (Aendern, then Eintragen); the contact's
// own form shows them and its answer, once taken, goes the same way
// (Uebernehmen). Open questions count on Deine Aufgaben (Offen).

// Bezug is a relation of a node: Mit is its Art (one of Arten, Keine
// the owner's „none“, "" a phone book's label that is none of them); Wie
// as the family reads it, with where it comes from.
type Bezug struct {
	Mit    Knoten
	Art    string
	Wie    string
	Gesagt bool
}

// relArt is a phone's relation (netz.rel.<key>) as a kind.
var relArt = map[string]string{"spouse": "partner", "partner": "partner", "child": "kind", "son": "kind", "daughter": "kind", "parent": "eltern",
	"mother": "eltern", "father": "eltern", "sibling": "geschwister", "brother": "geschwister", "sister": "geschwister", "friend": "freund",
	"relative": "verwandt", "kin": "verwandt", "colleague": "kollege", "coworker": "kollege", "neighbor": "nachbar", "acquaintance": "bekannt"}

// artVon is a phone book's label (label) as a kind; "" none.
func artVon(wie string) string {
	for k, a := range relArt {
		if texte.T(texte.ID("netz.rel."+k)) == wie {
			return a
		}
	}
	for _, a := range Arten {
		if strings.EqualFold(Wort(a), wie) || strings.EqualFold(Kurz(a), wie) {
			return a
		}
	}
	return ""
}

// Beziehungen are id's relations by name: the key owner's answers (Keine
// too), else what the phone books say; none with a hidden node.
func (n *Netz) Beziehungen(id string) []Bezug {
	if n == nil || id == "" {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	weg, m := n.verborgen[n.G.Wessen()], map[string]Bezug{}
	for _, f := range n.st.Fest {
		a, b := n.von[f.A], n.von[f.B]
		art := artVon(f.Wie)
		switch {
		case f.Art != Beziehung || a == b:
		case a == id: // A's card: B is A's Wie
			m[b] = Bezug{Mit: n.knoten[b], Art: art, Wie: f.Wie}
		case b == id && m[a].Mit.ID == "":
			art = cmp.Or(gegen[art], art)
			m[a] = Bezug{Mit: n.knoten[a], Art: art, Wie: map[bool]string{true: Kurz(art), false: f.Wie}[art != ""]}
		}
	}
	for key, a := range n.antworten[n.G.Wessen()] {
		x, y, _ := strings.Cut(key, "|")
		mit := map[bool]string{true: y, false: x}[x == id]
		if x != id && y != id || a.Art == Spaeter || mit == id {
			continue
		}
		art := a.Art
		if a.X == id {
			art = cmp.Or(gegen[art], art)
		}
		m[mit] = Bezug{Mit: n.knoten[mit], Art: art, Wie: Kurz(art), Gesagt: true}
	}
	var out []Bezug
	for mit, b := range m {
		if _, da := n.knoten[mit]; da && weg[mit] == "" && weg[id] == "" {
			out = append(out, b)
		}
	}
	slices.SortFunc(out, func(a, b Bezug) int {
		return cmp.Or(strings.Compare(a.Mit.Name, b.Mit.Name), strings.Compare(a.Mit.ID, b.Mit.ID))
	})
	return out
}

// Kurz is a kind without its explanation in brackets: „Mutter/Vater“;
// Keine „Keine Beziehung“.
func Kurz(art string) string {
	switch art {
	case Keine:
		return texte.T(texte.NetzKeineBez)
	case Fremd:
		return texte.T(texte.NetzFremd)
	}
	w, _, _ := strings.Cut(Wort(art), " (")
	return w
}

// Aendern keeps the key owner's answer about x and y given on a contact's
// page or taken from a contact's form, whether asked or not: x is art of y
// (a kind, Keine, Fremd), or "" forgets it. The two as nodes.
func (n *Netz) Aendern(x, y, art string) (Knoten, Knoten, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	wer := n.G.Wessen()
	kx, dx := n.knoten[x]
	ky, dy := n.knoten[y]
	if wer == "" || !dx || !dy || x == y || art != "" && art != Keine && art != Fremd && !slices.Contains(Arten, art) {
		return kx, ky, ErrKeineFrage
	}
	return kx, ky, n.merken(wer, x, y, art)
}

// Uebernehmen takes a relation from a contact's form: on ich's contact
// mit is art ("" none any more). mit without a node is found by its name;
// with both nodes it is an answer (Aendern), else it goes into ich's card
// alone. Only the key owner's (wer).
func (n *Netz) Uebernehmen(ctx context.Context, wer string, ich, mit Knoten, art string) error {
	if n == nil || wer == "" || wer != n.G.Wessen() {
		return ErrKeineFrage
	}
	art = cmp.Or(art, Keine)
	n.mu.Lock()
	n.laden()
	for id, k := range n.knoten {
		if mit.ID == "" && namen.Plain(k.Name) == namen.Plain(mit.Name) {
			mit.ID = id
		}
	}
	ich.ID = cmp.Or(ich.ID, n.von[ich.Kontakt])
	n.mu.Unlock()
	if ich.ID != "" && mit.ID != "" {
		x, y, err := n.Aendern(mit.ID, ich.ID, art)
		if err != nil {
			return err
		}
		mit, ich = x, y
	}
	return n.Eintragen(ctx, mit, ich, art)
}

// Offen is how many relation questions wait for person: the key owner's;
// 0 for anyone else (Deine Aufgaben).
func (n *Netz) Offen(person string) int {
	if n == nil || person == "" || n.G.Wessen() != person {
		return 0
	}
	return len(n.Fragen())
}

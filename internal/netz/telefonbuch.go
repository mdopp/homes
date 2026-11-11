package netz

import (
	"context"
	"net/url"
	"strings"
	"unicode"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/namen"
	"github.com/mdopp/homes/internal/texte"
)

// What the phone books say (owner 2026-11-09): the household's book and
// the key owner's mirrored Handy-Kontakte, firm cards aside, and the
// relations a phone wrote: vCard 4's RELATED, iOS's grouped
// X-ABRELATEDNAMES with its X-ABLabel, Android's X-ANDROID-CUSTOM
// vnd.android.cursor.item/relation. A relation names its other by UID or
// by name.

// karte is a card read: its key, its contact, its relations (UID or name → label).
type karte struct {
	key string
	ct  caldav.Contact
	rel [][2]string
}

// telefonbuch reads both books: the contacts with their persons and the
// relations.
func (n *Netz) telefonbuch(ctx context.Context) ([]kontakt, []Fest, error) {
	c := n.G.Kontakte
	if c == nil {
		return nil, nil, nil
	}
	var ks []karte
	cards, _, err := c.Cards(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, cd := range cards {
		ks = append(ks, lesen("haushalt/", cd.Text(), cd.Contact)...)
	}
	if login := n.G.Wessen(); login != "" {
		bcs, _, err := c.BookCards(ctx, "/"+url.PathEscape(login)+"/handy-kontakte/")
		if err != nil {
			return nil, nil, err
		}
		for _, cd := range bcs {
			ks = append(ks, lesen("privat/", cd.Data, caldav.Contact{})...)
		}
	}
	personen := n.G.Zuordnungen()
	var out []kontakt
	for _, k := range ks {
		out = append(out, kontakt{k.key, strings.Join(strings.Fields(k.ct.Name), " "), personen[k.key]})
	}
	return out, beziehungen(ks), nil
}

// lesen are the cards of one text (or ct alone when there is none), firms
// and nameless ones left out, each with its relations.
func lesen(buch, text string, ct caldav.Contact) (out []karte) {
	root, err := caldav.Parse(text)
	if text == "" || err != nil {
		root = nil
	}
	if root == nil {
		if ct.UID != "" && strings.TrimSpace(ct.Name) != "" && !ct.IsFirm() {
			out = append(out, karte{buch + ct.UID, ct, nil})
		}
		return out
	}
	root.Walk(func(c *caldav.Component) {
		if c.Name != "VCARD" {
			return
		}
		for _, ct := range caldav.Contacts(c) {
			if ct.UID != "" && strings.TrimSpace(ct.Name) != "" && !ct.IsFirm() {
				out = append(out, karte{buch + ct.UID, ct, relationen(c)})
			}
		}
	})
	return out
}

// android are Android's relation types (ContactsContract.CommonDataKinds.Relation).
var android = map[string]string{"1": "assistant", "2": "brother", "3": "child", "4": "partner", "5": "father", "6": "friend", "7": "manager",
	"8": "mother", "9": "parent", "10": "partner", "11": "referredby", "12": "relative", "13": "sister", "14": "spouse"}

// relationen are a card's relations, each its other (UID or name) and label.
func relationen(c *caldav.Component) (out [][2]string) {
	labels := map[string]string{}
	for _, p := range c.Props {
		if g, name, ok := strings.Cut(p.Name, "."); ok && strings.EqualFold(name, "X-ABLABEL") {
			labels[strings.ToUpper(g)] = caldav.UnescapeText(p.Value)
		}
	}
	for _, p := range c.Props {
		g, name, _ := strings.Cut(p.Name, ".")
		if name == "" {
			g, name = "", g
		}
		v := strings.TrimSpace(caldav.UnescapeText(p.Value))
		switch strings.ToUpper(name) {
		case "RELATED":
			typ, _, _ := strings.Cut(p.Params["TYPE"], ",")
			out = append(out, [2]string{v, typ})
		case "X-ABRELATEDNAMES":
			out = append(out, [2]string{v, labels[strings.ToUpper(g)]})
		case "X-ANDROID-CUSTOM":
			f := append(strings.Split(p.Value, ";"), "", "", "")
			if f[0] == "vnd.android.cursor.item/relation" {
				out = append(out, [2]string{caldav.UnescapeText(f[1]), map[bool]string{true: f[3]}[f[2] == "0"] + android[f[2]]})
			}
		}
	}
	return out
}

// beziehungen are the relations whose other is a contact of the same book
// (by UID, else by name) or of the other.
func beziehungen(ks []karte) (out []Fest) {
	uid, name := map[string]string{}, map[string]string{}
	for _, k := range ks {
		_, u, _ := strings.Cut(k.key, "/")
		uid[strings.TrimPrefix(strings.ToLower(u), "urn:uuid:")] = k.key
		name[namen.Plain(k.ct.Name)] = k.key
	}
	for _, k := range ks {
		for _, r := range k.rel {
			ziel := uid[strings.TrimPrefix(strings.ToLower(r[0]), "urn:uuid:")]
			if ziel == "" {
				ziel = name[namen.Plain(r[0])]
			}
			if ziel != "" && ziel != k.key {
				out = append(out, Fest{A: k.key, B: ziel, Art: Beziehung, Wie: label(r[1])})
			}
		}
	}
	return out
}

// label is a relation's label as the family reads it: a known kind in
// German (netz.rel.<kind>), iOS's _$!<Spouse>!$_ too; another as written.
func label(s string) string {
	s = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "_$!<"), ">!$_")
	kind := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
	if id := texte.ID("netz.rel." + kind); texte.T(id) != string(id) {
		return texte.T(id)
	}
	return s
}

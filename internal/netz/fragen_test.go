package netz

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func fragenVon(fs []Frage) (out []string) {
	for _, f := range fs {
		out = append(out, f.X.Name+">"+f.Y.Name+"="+f.Vorschlag)
	}
	return out
}

// Beziehungen klären (owner 2026-11-09): a tie without a relation is asked
// about, the key owner second („zu dir“); none where a phone book names a
// relation (Anna–Ben, Anna–Fritz), none about a hidden person; the
// strongest first.
func TestFragenOhneBeziehung(t *testing.T) {
	n, _, _ := netzBox(t)
	fs := n.Fragen()
	if got := fragenVon(fs); !slices.Equal(got, []string{"Fritz Kurz>Kim Berg=", "Ben Berg>Kim Berg=", "Kim Berg>Anna Berg=", "Carla Wolf>Anna Berg=kollege"}) {
		t.Fatalf("the questions: %v", got)
	}
	if !fs[2].Ich || fs[1].Ich || fs[3].Weil != "gleiche Firma" {
		t.Fatalf("%+v", fs)
	}
	n.Verbergen("p-kim", Aus)
	if got := fragenVon(n.Fragen()); !slices.Equal(got, []string{"Carla Wolf>Anna Berg=kollege"}) {
		t.Fatalf("kim hidden: %v", got)
	}
}

// An answer is a relation of its own, shown right from both ends, never
// asked again, kept over the daily run that read before it and over a
// restart; Keine is never asked again, Spaeter last; a pair not asked is
// refused.
func TestBeantworten(t *testing.T) {
	n, _, jetzt := netzBox(t)
	if f, err := n.Beantworten("p-anna", "p-kim", "eltern"); err != nil || f.X.ID != "p-anna" || f.Ich {
		t.Fatalf("%+v %v", f, err)
	}
	kim := n.Nahe("p-anna", 10)[1]
	if kim.Mit.ID != "p-kim" || n.Gruende("p-anna", kim.Kante) != "2 Tage auf deinen Fotos seit 2019 · Kind (von dir angegeben)" ||
		n.Gruende("p-kim", kim.Kante) != "2 Tage auf deinen Fotos seit 2019 · Mutter/Vater (von dir angegeben)" {
		t.Fatalf("anna–kim: %+v %q", kim, n.Gruende("p-anna", kim.Kante))
	}
	n.Beantworten("p-kim", "p-ben", Keine)
	if _, err := n.Beantworten(n.Von("haushalt/c"), "p-anna", Spaeter); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Beantworten("p-gerd", "p-anna", "freund"); err != ErrKeineFrage {
		t.Fatalf("gerd, one day only: %v", err)
	}
	if got := fragenVon(n.Fragen()); !slices.Equal(got, []string{"Fritz Kurz>Kim Berg=", "Carla Wolf>Anna Berg=kollege"}) {
		t.Fatalf("after the answers: %v", got)
	}
	if _, err := n.Beantworten("p-anna", "p-ben", "partner"); err != ErrKeineFrage {
		t.Fatalf("a relation the phone book names: %v", err)
	}
	*jetzt = jetzt.Add(25 * time.Hour)
	if err := n.Rechnen(t.Context()); err != nil {
		t.Fatal(err)
	}
	again := &Netz{G: n.G, Loc: time.UTC, Now: n.Now}
	for _, m := range []*Netz{n, again} {
		if got := fragenVon(m.Fragen()); len(got) != 2 || !strings.Contains(m.Gruende("p-kim", m.Nahe("p-kim", 10)[0].Kante), "Mutter/Vater (von dir angegeben)") {
			t.Fatalf("kept: %v", got)
		}
	}
}

// Eintragen writes the relation into both household cards, each the other
// by its UID with the kind seen from it; the next read of the phone books
// shows it once, the answer's.
func TestEintragen(t *testing.T) {
	n, _, jetzt := netzBox(t)
	carla := n.Von("haushalt/c")
	f, err := n.Beantworten(carla, "p-anna", "kollege")
	if err == nil {
		err = n.Eintragen(t.Context(), f.X, f.Y, "kollege")
	}
	if err != nil {
		t.Fatal(err)
	}
	if f, _ := n.Beantworten("p-kim", "p-anna", "kind"); n.Eintragen(t.Context(), f.X, f.Y, "kind") != nil { // kim has no card: anna's says her name
		t.Fatal("kim")
	}
	cards, _, _ := n.G.Kontakte.Cards(t.Context())
	text := map[string]string{}
	for _, c := range cards {
		text[c.UID] = c.Text()
	}
	if !strings.Contains(text["a"], "RELATED;TYPE=colleague:urn:uuid:c\r\n") || !strings.Contains(text["c"], "RELATED;TYPE=colleague:urn:uuid:a\r\n") ||
		!strings.Contains(text["a"], "RELATED;TYPE=child;VALUE=text:Kim Berg\r\n") {
		t.Fatalf("the cards:\n%s\n%s", text["a"], text["c"])
	}
	*jetzt = jetzt.Add(25 * time.Hour)
	n.Rechnen(t.Context())
	nahe := n.Nahe("p-anna", 10)
	v := nahe[slices.IndexFunc(nahe, func(v Verbindung) bool { return v.Mit.ID == carla })]
	if got := n.Gruende("p-anna", v.Kante); got != "gleiche Firma · Kollege/Kollegin (von dir angegeben)" {
		t.Fatalf("carla: %q", got)
	}
}

// The suggestions: a name's „Oma“ toward the key owner; the family name and
// address with the age gap (siblings, partners, parents, grandparents, or
// only kin); an address and a similar age; a street; many photo days.
func TestVorschlag(t *testing.T) {
	jetzt := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	n := &Netz{Now: func() time.Time { return jetzt }, ich: "ich", knoten: map[string]Knoten{"oma": {Name: "Oma Hilde"}}}
	geb := func(j int) time.Time { return time.Date(j, 6, 1, 0, 0, 0, 0, time.UTC) }
	haus := Kante{Fest: []Fest{{Art: Adresse}}}
	for _, c := range []struct {
		x, y       merkmale
		k          Kante
		name, want string
	}{
		{merkmale{}, merkmale{}, Kante{}, "oma", "grosseltern „Oma“ im Namen"},
		{merkmale{family: "berg", geb: geb(2012)}, merkmale{family: "berg", geb: geb(2015)}, haus, "", "geschwister gleicher Nachname und gleiche Adresse, 3 Jahre Altersunterschied"},
		{merkmale{family: "berg", geb: geb(1980)}, merkmale{family: "berg", geb: geb(1983)}, haus, "", "partner gleicher Nachname und gleiche Adresse, 3 Jahre Altersunterschied"},
		{merkmale{family: "berg", geb: geb(1980)}, merkmale{family: "berg", geb: geb(2012)}, haus, "", "eltern gleicher Nachname und gleiche Adresse, 32 Jahre Altersunterschied"},
		{merkmale{family: "berg", geb: geb(2012)}, merkmale{family: "berg", geb: geb(1950)}, haus, "", "enkel gleicher Nachname und gleiche Adresse, 62 Jahre Altersunterschied"},
		{merkmale{family: "berg"}, merkmale{family: "berg"}, haus, "", "verwandt gleicher Nachname und gleiche Adresse"},
		{merkmale{family: "berg", geb: geb(1980)}, merkmale{family: "wolf", geb: geb(1978)}, haus, "", "partner gleiche Adresse, 2 Jahre Altersunterschied"},
		{merkmale{family: "berg", geb: geb(1980)}, merkmale{family: "wolf", geb: geb(1950)}, haus, "", " "},
		{merkmale{strassen: []string{"gartenweg|66111"}}, merkmale{strassen: []string{"gartenweg|66111"}}, Kante{}, "", "nachbar gleiche Straße, anderes Haus"},
		{merkmale{family: "berg"}, merkmale{family: "wolf"}, Kante{Tage: 6}, "", "freund 6 Tage zusammen auf deinen Fotos, kein Zeichen von Familie"},
		{merkmale{family: "berg"}, merkmale{family: "berg"}, Kante{Tage: 6}, "", " "},
		{merkmale{family: nachname("", "Oma Hilde")}, merkmale{family: "wolf"}, Kante{Tage: 6}, "", " "}, // Hilde is no family name
	} {
		x := oder(c.name, "x")
		n.merkmale = map[string]merkmale{x: c.x, "ich": c.y}
		if art, weil := n.vorschlag(x, "ich", &c.k); art+" "+weil != c.want {
			t.Errorf("%+v %+v: %q %q, want %q", c.x, c.y, art, weil, c.want)
		}
	}
}

func oder(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Fremd (owner 2026-11-10: „die kennen sich nicht“) takes the tie out of
// the net: neither end shows the other, and the pair is not asked again;
// forgetting it brings the tie back.
func TestFremd(t *testing.T) {
	n, _, _ := netzBox(t)
	nahe := func() bool {
		return slices.ContainsFunc(n.Nahe("p-kim", 10), func(k Verbindung) bool { return k.Mit.ID == "p-ben" })
	}
	if !nahe() {
		t.Fatal("kim–ben: no tie to start with")
	}
	if _, err := n.Beantworten("p-kim", "p-ben", Fremd); err != nil {
		t.Fatal(err)
	}
	if nahe() || slices.ContainsFunc(fragenVon(n.Fragen()), func(s string) bool { return strings.Contains(s, "Ben") && strings.Contains(s, "Kim") }) {
		t.Fatalf("fremd: still a tie or a question: %v", fragenVon(n.Fragen()))
	}
	if _, _, err := n.Aendern("p-kim", "p-ben", ""); err != nil || !nahe() {
		t.Fatalf("forgotten: %v, tie back %v", err, nahe())
	}
}

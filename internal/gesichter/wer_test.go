package gesichter

import (
	"errors"
	"fmt"
	"image/color"
	"slices"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/immich/immichfake"
)

// werBox: Anna and Ben named; unnamed faces x1–x3 in 30 photos each (x2
// twice beside Anna in Münster, x3 once beside her), x4 in 8, x5 in 6, x6
// in 4 (too few to ask, owner 2026-11-09), x7 in none, x8 without a face Immich can show.
func werBox(t *testing.T) (*Gesichter, *immichfake.Server) {
	t.Helper()
	fake := immichfake.New("key")
	t.Cleanup(fake.Close)
	for i, p := range []struct {
		id, name string
		fotos    int
	}{{"anna", "Anna", 100}, {"ben", "Ben", 20}, {"x1", "", 30}, {"x2", "", 30}, {"x3", "", 30}, {"x4", "", 8}, {"x5", "", 6}, {"x6", "", 4}, {"x7", "", 0}, {"x8", "", 40}} {
		var face []byte
		if p.id != "x8" {
			face = Face(t, color.RGBA{uint8(i * 20), 90, 90, 255})
		}
		fake.Person(p.id, p.name, "1", false, face)
		fake.Lage(p.id, p.fotos, float64(i*10), 0, false)
	}
	fake.Bibliothek = []immichfake.Foto{
		{Wann: "2024-07-01T10:00:00.000Z", Stadt: "Münster", Wer: []string{"anna", "x2"}},
		{Wann: "2023-07-01T10:00:00.000Z", Stadt: "Münster", Wer: []string{"anna", "x2"}},
		{Wann: "2022-07-01T10:00:00.000Z", Stadt: "Berlin", Wer: []string{"anna", "x3"}},
		{Wann: "2021-07-01T10:00:00.000Z", Stadt: "Rom", Wer: []string{"x1"}},
	}
	g := &Gesichter{Immich: &immich.Client{URL: fake.URL, Key: "key"}, Dir: t.TempDir(), Now: func() time.Time { return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC) }}
	pflegen(t, g)
	return g, fake
}

// pflegen is the hourly run's part that „Wer ist das?“ reads.
func pflegen(t *testing.T, g *Gesichter) {
	t.Helper()
	for _, f := range []func() error{func() error { return g.Auffrischen(t.Context()) }, func() error { return g.Vorschlagen(t.Context()) }, func() error { return g.Umfelder(t.Context()) }} {
		if err := f(); err != nil {
			t.Fatal(err)
		}
	}
}

func wer(g *Gesichter) (out []string) {
	for _, s := range g.Wer() {
		out = append(out, s.ID)
	}
	return out
}

// „Wer ist das?“ (owner 2026-11-09): five unnamed faces, the most photos
// first, on a tie the one beside named persons more often, then at
// recurring places; none without a face or a photo; each with its photos,
// places and company; nothing asked of Immich by reading them.
func TestWer(t *testing.T) {
	g, fake := werBox(t)
	if got := fmt.Sprint(wer(g)); got != "[x2 x3 x1 x4 x5]" {
		t.Fatal(got)
	}
	s := g.Wer()[0]
	if s.Name != "" || s.Fotos != 30 || fmt.Sprint(s.Orte) != "[Münster]" || fmt.Sprint(s.Mit) != "[Anna]" || !Gesehen(s.Bild) || s.Kontakt != "" {
		t.Fatalf("%+v", s)
	}
	n := fake.Abgerufen("x2")
	g.Wer()
	if fake.Abgerufen("x2") != n {
		t.Fatal("a read asked Immich")
	}
}

// A name goes to Immich and the face leaves the list (the next comes in);
// „Kenne ich nicht“ is kept, also for a Homes started anew and after
// Immich is read again; a face not offered is refused, and so is a name
// without person.update.
func TestBenennen(t *testing.T) {
	g, fake := werBox(t)
	if name, _, err := g.Benennen(t.Context(), "x2", "  Yvonne   Huber "); err != nil || name != "Yvonne Huber" {
		t.Fatalf("%q %v", name, err)
	}
	if !slices.Contains(fake.Alle(), "Yvonne Huber|") || slices.Contains(wer(g), "x2") || g.Bild("Yvonne Huber") == nil {
		t.Fatalf("%v %v", fake.Alle(), wer(g))
	}
	if _, _, err := g.Benennen(t.Context(), "x3", ""); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(wer(g)); got != "[x1 x4 x5]" { // x6 is in 4 photos: never asked
		t.Fatal(got)
	}
	neu := &Gesichter{Immich: g.Immich, Dir: g.Dir, Now: g.Now}
	pflegen(t, neu)
	if got := fmt.Sprint(wer(neu)); got != "[x1 x4 x5]" {
		t.Fatalf("anew: %s", got)
	}
	for _, id := range []string{"x3", "x7", "x8", "anna", "nie"} {
		if _, _, err := neu.Benennen(t.Context(), id, "Z"); !errors.Is(err, ErrKeineFrage) {
			t.Fatalf("%s: %v", id, err)
		}
	}
	fake.Ohne = []string{"person.update"}
	var fehlt immich.FehltRecht
	if _, _, err := neu.Benennen(t.Context(), "x1", "Zoe"); !errors.As(err, &fehlt) || !slices.Contains(wer(neu), "x1") {
		t.Fatalf("%v %v", err, wer(neu))
	}
}

// A named side of a pair carries its contact (the household's before the
// owner's own), an unnamed one none.
func TestSeiteKontakt(t *testing.T) {
	g, _ := naeheBox(t)
	g.mu.Lock()
	g.stand.Zuordnung["privat/k"] = zuordnung{ID: "anna", Name: "Anna"}
	g.stand.Zuordnung["haushalt/h"] = zuordnung{ID: "anna", Name: "Anna"}
	g.stand.Zuordnung["haushalt/u"] = zuordnung{ID: "u1", Name: "Unbekannt"}
	g.mu.Unlock()
	p := g.Paare()[0]
	if p.A.ID != "anna" || p.A.Kontakt != "haushalt/h" || p.B.ID != "u1" || p.B.Kontakt != "" {
		t.Fatalf("%+v %+v", p.A.Kontakt, p.B.Kontakt)
	}
}

// After a pair's „Nein“ its unnamed face can be named right there (owner
// 2026-11-11): offered by Dann only then, named in Immich like „Wer ist
// das?“; the named side's own name merges the two after all and the
// „Nein“ goes; „Kenne ich nicht“ ends it; a face of no „Nein“ is refused.
func TestDann(t *testing.T) {
	g, fake := naeheBox(t)
	if _, ok := g.Dann("u1"); ok {
		t.Fatal("offered before a Nein")
	}
	for _, ab := range [][2]string{{"anna", "u1"}, {"carl", "u3"}} {
		if _, err := g.Antworten(t.Context(), ab[0], ab[1], "nein"); err != nil {
			t.Fatal(err)
		}
	}
	if s, ok := g.Dann("u1"); !ok || s.Name != "" || s.Fotos != 7 || !Gesehen(s.Bild) {
		t.Fatalf("%v %+v", ok, s)
	}
	if name, doch, err := g.Benennen(t.Context(), "u1", " Zoe  Berg"); err != nil || doch || name != "Zoe Berg" || !slices.Contains(fake.Alle(), "Zoe Berg|") {
		t.Fatalf("%q %v %v %v", name, doch, err, fake.Alle())
	}
	if _, ok := g.Dann("u1"); ok {
		t.Fatal("a named face still offered")
	}
	if name, doch, err := g.Benennen(t.Context(), "u3", " carl "); err != nil || !doch || name != "Carl" || fmt.Sprint(fake.Merges()) != "[[carl u3]]" {
		t.Fatalf("%q %v %v %v", name, doch, err, fake.Merges())
	}
	if _, nein := g.stand.Nein[paarKey("carl", "u3")]; nein {
		t.Fatal("the Nein stayed after the merge")
	}
	if _, _, err := g.Benennen(t.Context(), "u2", "Udo"); !errors.Is(err, ErrKeineFrage) {
		t.Fatalf("no Nein, not offered: %v", err)
	}
	g2, _ := naeheBox(t)
	g2.Antworten(t.Context(), "anna", "u1", "nein")
	if _, _, err := g2.Benennen(t.Context(), "u1", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := g2.Dann("u1"); ok {
		t.Fatal("offered after Kenne ich nicht")
	}
}

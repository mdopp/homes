package gesichter

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"slices"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/immich/immichfake"
)

// The empty faces on „Dieselbe Person?“ (owner 2026-11-07): a pair is
// offered only when both sides have a face kept that is a JPEG, and each
// is in a photo. Immich answers 404 for a face it has no thumbnail of
// (u4), a face kept may be broken bytes (u5), and a person may be in no
// photo (u6); none of them is asked about, and a face fetched later
// brings its pair back.
func TestPaareNurMitGesicht(t *testing.T) {
	fake := immichfake.New("key")
	t.Cleanup(fake.Close)
	for _, p := range []struct {
		id, name string
		fotos    int
		x        float64
		face     bool
	}{{"anna", "Anna", 100, 0, true}, {"u4", "", 7, 1, false}, {"u5", "", 5, 2, true}, {"u6", "", 0, 3, true}, {"u7", "", 4, 9, true}} {
		var face []byte
		if p.face {
			face = Face(t, color.RGBA{uint8(p.fotos), 90, 90, 255})
		}
		fake.Person(p.id, p.name, "1", false, face)
		fake.Lage(p.id, p.fotos, p.x, 0, false)
	}
	g := &Gesichter{Immich: &immich.Client{URL: fake.URL, Key: "key"}, Dir: t.TempDir(), Now: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }}
	if err := g.Auffrischen(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := g.Vorschlagen(t.Context()); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	g.stand.Bilder["u5"] = bild{JPEG: []byte{0xff, 0xd8, 0, 0}, Stand: "1", Geholt: g.now()}
	if !slices.Equal(g.stand.Naehe["anna"].Kandidaten, []string{"u4"}) { // only the closest (owner 2026-11-09)
		t.Fatalf("closest: %v", g.stand.Naehe["anna"])
	}
	g.mu.Unlock()
	if got := paare(g); len(got) != 0 {
		t.Fatalf("pairs without a face to show: %v", got)
	}
	for _, p := range g.Paare() {
		if !Gesehen(p.A.Bild) || !Gesehen(p.B.Bild) {
			t.Fatalf("%+v", p)
		}
	}
	fake.Person("u4", "", "2", false, Face(t, color.RGBA{7, 90, 90, 255}))
	g.mu.Lock()
	g.stand.Andere["u4"] = immich.Person{ID: "u4", Stand: "2"}
	g.mu.Unlock()
	g.nachladen(t.Context(), []string{"u4"})
	if got := paare(g); !slices.Equal(got, []string{"anna-u4"}) {
		t.Fatalf("the face came, the pair not: %v", got)
	}
	if Gesehen(nil) || Gesehen([]byte{}) || !Gesehen(Face(t, color.Black)) {
		t.Fatal("Gesehen")
	}
}

// The places and company of a pair's persons: read once on the first run,
// then not again for a day; the most frequent first, the city else the
// country, the span of years, named companions only, the person itself
// never; the links to Immich; without asset.read nothing and the page's
// line names it.
func TestUmfelder(t *testing.T) {
	g, fake := naeheBox(t)
	fake.Bibliothek = []immichfake.Foto{
		{Wann: "2019-07-01T10:00:00.000Z", Stadt: "Münster", Land: "Deutschland", Wer: []string{"anna", "ben", "u1"}},
		{Wann: "2016-05-01T10:00:00.000Z", Stadt: "Münster", Land: "Deutschland", Wer: []string{"anna", "ben"}},
		{Wann: "2014-01-01T10:00:00.000Z", Land: "Italien", Wer: []string{"anna", "carl"}},
		{Wann: "2012-03-01T10:00:00.000Z", Stadt: "Berlin", Land: "Deutschland", Wer: []string{"anna", "u2"}},
	}
	if err := g.Umfelder(t.Context()); err != nil {
		t.Fatal(err)
	}
	p := g.Paare()[0]
	if p.A.ID != "anna" || fmt.Sprint(p.A.Orte) != "[Münster Berlin Italien]" || p.A.Von != 2012 || p.A.Bis != 2019 || fmt.Sprint(p.A.Mit) != "[Ben Carl]" {
		t.Fatalf("%+v", p.A)
	}
	if fmt.Sprint(p.B.Orte) != "[Münster]" || p.B.Von != 2019 || fmt.Sprint(p.B.Mit) != "[Anna Ben]" {
		t.Fatalf("%+v", p.B)
	}
	fake.Bibliothek = nil
	g.Umfelder(t.Context()) // within the day: nothing read
	if p := g.Paare()[0]; p.A.Von != 2012 {
		t.Fatalf("read again within the day: %+v", p.A)
	}
	if got := g.PersonSeite("anna"); got != "" {
		t.Fatalf("no address, yet a link: %q", got)
	}
	g.Web = "https://photos.box.test"
	if g.PersonSeite("anna") != "https://photos.box.test/people/anna" || g.FotoSeite("a-1") != "https://photos.box.test/photos/a-1" || g.FotoSeite("") != "" {
		t.Fatal(g.PersonSeite("anna"), g.FotoSeite("a-1"))
	}
	g.Now = func() time.Time { return time.Date(2026, 11, 7, 13, 0, 0, 0, time.UTC) }
	if err := g.Umfelder(t.Context()); err != nil {
		t.Fatal(err)
	}
	if p := g.Paare()[0]; p.A.Von != 0 || p.A.Orte != nil {
		t.Fatalf("a day later, read again from an empty library: %+v", p.A)
	}
	fake.Ohne = []string{"asset.read"}
	g.Auffrischen(t.Context())
	if g.Fehlt() != "asset.read" {
		t.Fatalf("%q", g.Fehlt())
	}
}

// Offen counts the pairs and questions for the key's owner only.
func TestOffen(t *testing.T) {
	g, _ := naeheBox(t)
	if g.Offen("anna") != 0 {
		t.Fatal("before the owner is known")
	}
	g.mu.Lock()
	g.stand.Login = "anna"
	g.mu.Unlock()
	if n := g.Offen("anna"); n != len(g.Paare()) || n == 0 {
		t.Fatalf("%d of %d", n, len(g.Paare()))
	}
	if g.Offen("ben") != 0 || g.Offen("") != 0 {
		t.Fatal("someone else sees the owner's questions")
	}
}

// „Ist das …?“ the same way: a question only with both pictures kept as
// JPEGs and a person in a photo; it carries Homes' upload for the link.
func TestFragenNurMitBildern(t *testing.T) {
	g, _ := naeheBox(t)
	face := Face(t, color.White)
	g.mu.Lock()
	g.stand.Fotos["leer"] = zahl{N: 0, Geholt: g.now()}
	g.stand.Kontaktbilder = map[string]kontaktbild{
		"privat/a": {Stand: bildFrage, Asset: "a-1", Person: "anna", Bild: face, Dessen: face},
		"privat/b": {Stand: bildFrage, Asset: "a-2", Person: "ben", Bild: face},
		"privat/c": {Stand: bildFrage, Asset: "a-3", Person: "carl", Bild: []byte("kaputt"), Dessen: face},
		"privat/d": {Stand: bildFrage, Asset: "a-4", Person: "leer", Bild: face, Dessen: face},
	}
	g.mu.Unlock()
	fr := g.Fragen()
	if len(fr) != 1 || fr[0].Key != "privat/a" || fr[0].Asset != "a-1" {
		t.Fatalf("%+v", fr)
	}
}

// Three faces of a pair's person, each cut from another of its photos
// (owner 2026-11-11): the photos with the fewest people first; a face
// under a twentieth of its photo's width is none, nor one too dark to
// say anything. Immich's own face, one colour here, says little, so the
// first cut face is shown big in its place; a good one stays. The faces
// kept are not cut again while their photos stay among the newest, and
// without asset.view the page's line names it.
func TestAusschnitte(t *testing.T) {
	g, fake := naeheBox(t)
	hell, dunkel := Foto(t, false), Foto(t, true)
	gross, klein := [4]int{1000, 1000, 1600, 1700}, [4]int{1000, 1000, 1100, 1100}
	f := func(id string, bild []byte, anna [4]int, wer ...string) immichfake.Foto {
		return immichfake.Foto{ID: id, Wann: "2020-01-01T10:00:00.000Z", Wer: append([]string{"anna"}, wer...), Breite: 4000, Hoehe: 3000,
			Rahmen: map[string][4]int{"anna": anna, "u1": {2400, 800, 3000, 1500}}, Bild: bild}
	}
	fake.Bibliothek = []immichfake.Foto{f("viele", hell, gross, "u1", "ben", "carl"), f("klein", hell, klein), f("dunkel", dunkel, gross),
		f("a", hell, gross, "u1"), f("b", hell, gross), f("c", hell, gross, "ben")}
	g.mu.Lock()
	g.stand.Bilder["u1"] = bild{JPEG: hell, Stand: "1", Geholt: g.now()}
	g.mu.Unlock()
	if err := g.Umfelder(t.Context()); err != nil {
		t.Fatal(err)
	}
	asset := func(as []Ausschnitt) (out []string) {
		for _, a := range as {
			out = append(out, a.Asset)
		}
		return out
	}
	p := g.Paare()[0]
	if p.A.ID != "anna" || !Gesehen(p.A.Bild) || schlecht(p.A.Bild) || fmt.Sprint(asset(p.A.Weitere)) != "[a c]" {
		t.Fatalf("anna: %v %v", schlecht(p.A.Bild), asset(p.A.Weitere))
	}
	if c, _ := jpeg.DecodeConfig(bytes.NewReader(p.A.Weitere[0].JPEG)); c.Width != c.Height || c.Width > ausschnittKante || c.Width < 90 {
		t.Fatalf("%+v", c)
	}
	if !bytes.Equal(p.B.Bild, hell) || fmt.Sprint(asset(p.B.Weitere)) != "[a viele]" {
		t.Fatalf("u1, its own face good: %v", asset(p.B.Weitere))
	}
	for i := range fake.Bibliothek {
		fake.Bibliothek[i].Bild = nil
	}
	g.Now = func() time.Time { return time.Date(2026, 11, 7, 13, 0, 0, 0, time.UTC) }
	if err := g.Umfelder(t.Context()); err != nil {
		t.Fatal(err)
	}
	if p := g.Paare()[0]; fmt.Sprint(asset(p.A.Weitere)) != "[a c]" {
		t.Fatalf("the kept faces lost: %v", asset(p.A.Weitere))
	}
	fake.Ohne = []string{"asset.view"}
	g.Auffrischen(t.Context())
	if g.Fehlt() != "asset.view" {
		t.Fatalf("%q", g.Fehlt())
	}
}

// Foto is a photo's preview of 400×300, its brightness varying, or black.
func Foto(t testing.TB, schwarz bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := range 300 {
		for x := range 400 {
			if !schwarz {
				img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
			}
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

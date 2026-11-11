package gesichter

import (
	"encoding/base64"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/immich/immichfake"
)

// fotoBox: abgleichBox, plus an unnamed face u7 in two photos, and photos
// on the household's cards: Carla's face is u7's (a second Carla card the
// same bytes), Ella's Anna Müller's, a group of two faces, a firm's logo,
// a face Homes wrote itself, and anna's own Handy-Kontakt with a picture
// (never read here). A library photo shows u7 too.
func fotoBox(t *testing.T) (*Gesichter, *immichfake.Server, func(string, string, color.RGBA, ...string) string) {
	t.Helper()
	g, fake, dav := abgleichBox(t)
	fake.Person("u7", "", "1", false, Face(t, color.RGBA{10, 200, 10, 255}))
	fake.Lage("u7", 2, 0, 0, false)
	fake.Bibliothek = []immichfake.Foto{{ID: "fam-1", Wann: "2025-06-01T10:00:00Z", Stadt: "Kiel", Wer: []string{"u7"}}}
	erkannt := map[string][]string{}
	fake.Erkennen = func(_ string, jpeg []byte) []string { return erkannt[string(jpeg)] }
	karte := func(pfad, fn string, c color.RGBA, faces ...string) string {
		b := Face(t, c)
		erkannt[string(b)] = faces
		dav.Put(pfad, vcard(strings.TrimSuffix(pfad[strings.LastIndex(pfad, "/")+1:], ".vcf"), fn, foto(b)))
		return bildHash(b)
	}
	karte("/homes/haushalt-kontakte/c.vcf", "Carla  Berg", color.RGBA{40, 0, 0, 255}, "u7")
	karte("/homes/haushalt-kontakte/c3.vcf", "Carla Berg", color.RGBA{40, 0, 0, 255}, "u7")
	karte("/homes/haushalt-kontakte/e.vcf", "Ella", color.RGBA{80, 0, 0, 255}, "p-anna")
	karte("/homes/haushalt-kontakte/g.vcf", "Gruppe", color.RGBA{120, 0, 0, 255}, "u7", "p-anna")
	dav.Put("/homes/haushalt-kontakte/f2.vcf", vcard("f2", "Bäckerei", "CATEGORIES:Firma\r\nORG:Bäckerei\r\n"+foto(Face(t, color.RGBA{0, 0, 90, 255}))))
	dav.Put("/homes/haushalt-kontakte/h.vcf", vcard("h", "Hanna", "homes-foto.PHOTO;ENCODING=b;TYPE=JPEG;X-HOMES=immich:"+base64.StdEncoding.EncodeToString(Face(t, color.RGBA{0, 40, 0, 255}))+"\r\n"))
	dav.Put("/anna/handy-kontakte/m.vcf", vcard("m", "Mama", foto(Face(t, color.RGBA{0, 90, 90, 255}))))
	g.Merke("anna", "anna@box.test")
	if err := g.Auffrischen(t.Context()); err != nil {
		t.Fatal(err)
	}
	return g, fake, karte
}

func album(t *testing.T, fake *immichfake.Server) []string {
	t.Helper()
	alben := fake.Alben()
	if len(alben) != 1 {
		t.Fatalf("albums: %v", alben)
	}
	for _, namen := range alben {
		slices.Sort(namen)
		return namen
	}
	return nil
}

// A run puts each household photo up once, archived, all in one album;
// a firm's logo, a face Homes wrote and a phone book's picture are not.
// The next run reads the faces: the one face in an unnamed person
// suggests its contact's name on „Wer ist das?“, first, even in fewer
// than werMinFotos photos; a named person's and a group's suggest none.
// Homes' uploads are in no search of the family's photos nor in a count.
func TestKontaktfotos(t *testing.T) {
	g, fake, _ := fotoBox(t)
	for range 2 {
		if err := g.Kontaktfotos(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	fotos := fake.Fotos()
	if len(fotos) != 3 || slices.ContainsFunc(fotos, func(f string) bool {
		return !strings.HasPrefix(f, "homes-kontaktfoto-") || !strings.HasSuffix(f, ".jpg|archive")
	}) {
		t.Fatalf("uploads: %v", fotos)
	}
	if got := album(t, fake); len(got) != 3 {
		t.Fatalf("album: %v", got)
	}
	if got := fmt.Sprint(g.stand.vorschlaege()); got != "map[p-anna:Ella u7:Carla Berg]" { // the group's none; a named person's is never asked
		t.Fatalf("suggestions: %s", got)
	}
	if _, ok := g.zaehlen(t.Context(), []string{"u7"}); !ok {
		t.Fatal("not counted")
	}
	wer := g.Wer()
	if len(wer) == 0 || wer[0].ID != "u7" || wer[0].Vorschlag != "Carla Berg" || wer[0].Fotos != 1 {
		t.Fatalf("Wer: %+v", wer)
	}
	umfeld, err := g.Immich.Umfeld(t.Context(), "u7", 10)
	if err != nil || len(umfeld) != 1 || umfeld[0].ID != "fam-1" {
		t.Fatalf("Umfeld: %+v %v", umfeld, err)
	}
	aufnahmen, err := g.Immich.Aufnahmen(t.Context())
	if err != nil || len(aufnahmen) != 1 {
		t.Fatalf("Aufnahmen: %+v %v", aufnahmen, err)
	}
	if name, doch, err := g.Benennen(t.Context(), "u7", wer[0].Vorschlag); err != nil || doch || name != "Carla Berg" || fake.ID("Carla Berg") == "" {
		t.Fatalf("named: %q %v %v", name, doch, err)
	}
}

// A photo changed replaces its upload, one on no card any more is deleted
// (the second card of the same bytes keeps it until it goes too), one
// deleted in Immich is not put there again, and an album deleted in
// Immich is made again with every upload.
func TestKontaktfotosFolgen(t *testing.T) {
	g, fake, karte := fotoBox(t)
	run := func() {
		t.Helper()
		if err := g.Kontaktfotos(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	run()
	vorher := album(t, fake)
	if err := g.Kontakte.DeleteCard(t.Context(), g.Kontakte.URL+"/homes/haushalt-kontakte/c3.vcf", ""); err != nil {
		t.Fatal(err)
	}
	run()
	if got := album(t, fake); !slices.Equal(got, vorher) {
		t.Fatalf("Carla's photo went with the second card: %v", got)
	}
	neu := karte("/homes/haushalt-kontakte/c.vcf", "Carla Berg", color.RGBA{60, 0, 0, 255}, "u7")
	if err := g.Kontakte.DeleteCard(t.Context(), g.Kontakte.URL+"/homes/haushalt-kontakte/e.vcf", ""); err != nil {
		t.Fatal(err)
	}
	run()
	got := album(t, fake)
	if len(got) != 2 || !slices.Contains(got, "homes-kontaktfoto-"+neu+".jpg") || len(fake.Fotos()) != 2 {
		t.Fatalf("after the change: %v %v", got, fake.Fotos())
	}
	fake.AssetWeg("homes-kontaktfoto-" + neu + ".jpg")
	run()
	run()
	if len(fake.Fotos()) != 1 || g.stand.Kontaktfotos[neu].Stand != fotoWeg {
		t.Fatalf("put up again: %v", fake.Fotos())
	}
	for id := range fake.Alben() {
		fake.AlbumWeg(id)
	}
	run()
	if got := album(t, fake); len(got) != 1 {
		t.Fatalf("album made again: %v", got)
	}
}

// Without the album's permissions nothing goes up, and Fehlt names them.
func TestKontaktfotosOhneRecht(t *testing.T) {
	g, fake, _ := fotoBox(t)
	fake.Ohne = []string{"album.create", "albumAsset.create"}
	if err := g.Auffrischen(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := g.Kontaktfotos(t.Context()); err != nil || len(fake.Fotos()) != 0 {
		t.Fatalf("%v %v", fake.Fotos(), err)
	}
	if f := g.Fehlt(); !strings.Contains(f, "album.create, albumAsset.create") {
		t.Fatalf("Fehlt: %q", f)
	}
}

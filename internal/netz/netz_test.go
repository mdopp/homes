package netz

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/immich/immichfake"
)

func vcard(uid, fn, extra string) string {
	return "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + fn + "\r\n" + extra + "END:VCARD\r\n"
}

// bei is a photo on day d (2006-01-02) at noon UTC, in Saarbrücken's cell
// or at lat 0/lng 0 (none) with a city, of wer.
func bei(id, d string, lat, lng float64, stadt string, wer ...string) immichfake.Foto {
	return immichfake.Foto{ID: id, Wann: d + "T12:00:00.000Z", Lat: lat, Lng: lng, Stadt: stadt, Wer: wer}
}

// netzBox: anna's key. Immich knows Anna, Ben, Kim (a child on most
// photos), Fritz, Gerd, a hidden Hanna and an unnamed face. Anna and Ben
// are together on three days, Kim with each, Fritz with Kim on two, Gerd
// with Anna on one day (two photos at one place); the household's book
// has Anna (the key's e-mail, iOS: Ben her spouse) and Ben at one
// address, Carla (no face) at Anna's firm; anna's phone has Fritz, whom
// Android calls her friend and vCard 4 her brother.
func netzBox(t *testing.T) (*Netz, *immichfake.Server, *time.Time) {
	t.Helper()
	fake := immichfake.New("key")
	t.Cleanup(fake.Close)
	for _, p := range [][2]string{{"p-anna", "Anna Berg"}, {"p-ben", "Ben Berg"}, {"p-kim", "Kim Berg"}, {"p-fritz", "Fritz Kurz"}, {"p-gerd", "Gerd Lang"}, {"u1", ""}} {
		fake.Person(p[0], p[1], "1", false, nil)
	}
	fake.Person("p-hanna", "Hanna", "1", true, nil)
	fake.Bibliothek = []immichfake.Foto{
		bei("a1", "2019-05-01", 49.23, 7.0, "Saarbrücken", "p-anna", "p-ben", "p-kim"),
		bei("a2", "2019-05-01", 49.231, 7.001, "Saarbrücken", "p-anna", "p-ben", "u1", "p-hanna"), // the same event: once
		bei("a3", "2022-08-10", 0, 0, "Rom", "p-anna", "p-ben"),
		bei("a4", "2025-12-24", 0, 0, "", "p-anna", "p-ben", "p-kim"),
		bei("a5", "2024-03-01", 52.5, 13.4, "Berlin", "p-fritz", "p-kim"),
		bei("a6", "2025-03-01", 52.5, 13.4, "Berlin", "p-fritz", "p-kim"),
		bei("a7", "2023-01-01", 49.23, 7.0, "Saarbrücken", "p-anna", "p-gerd"),
		bei("a8", "2023-01-01", 49.23, 7.0, "Saarbrücken", "p-anna", "p-gerd"),
		bei("a9", "2024-06-01", 49.23, 7.0, "Saarbrücken", "p-kim"),
	}
	dav := davfake.New("homes", "pw")
	t.Cleanup(dav.Close)
	dav.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	dav.Put("/homes/haushalt-kontakte/a.vcf", vcard("a", "Anna Berg", "EMAIL:anna@box.test\r\nADR;TYPE=HOME:;;Gartenweg 2;Saarbrücken;;66111;Deutschland\r\nORG:Werk AG\r\n"+
		"item1.X-ABRELATEDNAMES:Ben Berg\r\nitem1.X-ABLabel:_$!<Spouse>!$_\r\n"))
	dav.Put("/homes/haushalt-kontakte/b.vcf", vcard("b", "Ben Berg", "ADR;TYPE=HOME:;;Gartenweg 2;Saarbrücken;;66111;Deutschland\r\n"))
	dav.Put("/homes/haushalt-kontakte/c.vcf", vcard("c", "Carla Wolf", "ORG:Werk AG;Einkauf\r\n"))
	dav.Put("/homes/haushalt-kontakte/f.vcf", vcard("f", "Werk AG", "ORG:Werk AG\r\nCATEGORIES:Firma\r\n"))
	dav.AddPrincipal("anna")
	dav.AddAddressBook("/anna/handy-kontakte/", "Handy-Kontakte")
	dav.Put("/anna/handy-kontakte/k1.vcf", vcard("k1", "Anna Berg", "X-ANDROID-CUSTOM:vnd.android.cursor.item/relation;Fritz Kurz;6;;;;;;;;;;;;;\r\n"+
		"RELATED;TYPE=sibling:urn:uuid:k2\r\n"))
	dav.Put("/anna/handy-kontakte/k2.vcf", vcard("urn:uuid:k2", "Fritz Kurz", ""))
	jetzt := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	g := &gesichter.Gesichter{Immich: &immich.Client{URL: fake.URL, Key: "key"}, Kontakte: &caldav.Client{URL: dav.URL, User: "homes", Password: "pw", Loc: time.UTC},
		Web: "https://photos.box.test", Dir: t.TempDir(), Now: func() time.Time { return jetzt }}
	fake.Email = "anna@box.test"
	if err := g.Auffrischen(t.Context()); err != nil {
		t.Fatal(err)
	}
	g.Merke("anna", "anna@box.test")
	if err := g.Abgleichen(t.Context()); err != nil {
		t.Fatal(err)
	}
	n := &Netz{G: g, Loc: time.UTC, Now: func() time.Time { return jetzt }}
	if err := n.Rechnen(t.Context()); err != nil {
		t.Fatal(err)
	}
	return n, fake, &jetzt
}

func namenVon(vs []Verbindung) (out []string) {
	for _, v := range vs {
		out = append(out, v.Mit.Name)
	}
	return out
}

// The ties of all people in the key owner's photos, not only hers: a day
// at one place counts once, a tie from photos needs two days (Gerd has
// one), the phone books add the address, the firm and the relations
// (iOS, Android, vCard 4) with the relation as seen from the centre.
func TestNetzAusFotosUndTelefonbuch(t *testing.T) {
	n, _, _ := netzBox(t)
	anna := n.Mitte()
	if anna != "p-anna" {
		t.Fatalf("the middle is the key owner's: %q", anna)
	}
	nahe := n.Nahe(anna, 10)
	if got := namenVon(nahe); !slices.Equal(got[:1], []string{"Ben Berg"}) || slices.Contains(got, "Gerd Lang") || slices.Contains(got, "Hanna") || !slices.Contains(got, "Carla Wolf") {
		t.Fatalf("anna's: %v", got)
	}
	if got := n.Gruende(anna, nahe[0].Kante); got != "3 Tage auf deinen Fotos seit 2019 · gleiche Adresse · Ehepartner/in (Telefonbuch)" {
		t.Fatalf("anna–ben: %q", got)
	}
	ben := n.Von("haushalt/b")
	if ben != "p-ben" || n.Gruende(ben, nahe[0].Kante) != "3 Tage auf deinen Fotos seit 2019 · gleiche Adresse · Ehepartner/in von Anna Berg (Telefonbuch)" {
		t.Fatalf("ben's view: %q %q", ben, n.Gruende(ben, nahe[0].Kante))
	}
	fritz := n.Nahe(n.Von("privat/urn:uuid:k2"), 5)
	if got := namenVon(fritz); !slices.Equal(got, []string{"Anna Berg", "Kim Berg"}) {
		t.Fatalf("fritz: %v", got)
	}
	if got := n.Gruende(fritz[0].Mit.ID, fritz[0].Kante); got != "Freund/in (Telefonbuch) · Geschwister (Telefonbuch)" {
		t.Fatalf("anna–fritz: %q", got)
	}
	if got := n.Gruende("p-kim", fritz[1].Kante); got != "2 Tage auf deinen Fotos seit 2024" {
		t.Fatalf("fritz–kim, not anna's: %q", got)
	}
	carla := slices.IndexFunc(nahe, func(v Verbindung) bool { return v.Mit.Name == "Carla Wolf" })
	if n.Gruende(anna, nahe[carla].Kante) != "gleiche Firma" || nahe[carla].Mit.Kontakt != "haushalt/c" {
		t.Fatalf("carla: %+v", nahe[carla])
	}
	if z := n.Zwischen([]string{"p-ben", "p-kim", "p-gerd"}); !slices.Equal(z, [][2]int{{0, 1}}) {
		t.Fatalf("among them: %v", z)
	}
	fi, err := os.Stat(filepath.Join(n.G.Dir, tabelle))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the table: %v %v", fi, err)
	}
}

// Rang divides by how often both appear at all: Kim, on more events
// without Anna than with her, comes after Ben. A big group counts little
// per pair: two days of a ten-person photo are less than half of two
// days as a pair.
func TestNetzGruppeUndHub(t *testing.T) {
	n, _, _ := netzBox(t)
	var ben, kim Kante
	for _, v := range n.Nahe("p-anna", 10) {
		switch v.Mit.ID {
		case "p-ben":
			ben = v.Kante
		case "p-kim":
			kim = v.Kante
		}
	}
	if kim.Tage != 2 || kim.Rang >= ben.Rang {
		t.Fatalf("kim %+v, ben %+v", kim, ben)
	}
	viele := []string{"p-anna", "p-ben", "p-kim", "p-fritz", "p-gerd", "x1", "x2", "x3", "x4", "x5"}
	n.st.Namen["x1"], n.st.Namen["x2"], n.st.Namen["x3"], n.st.Namen["x4"], n.st.Namen["x5"] = "X1", "X2", "X3", "X4", "X5"
	n.st.Fotos = []foto{{T: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Wer: viele}, {T: time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC), Wer: viele},
		{T: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Stadt: "Rom", Wer: []string{"x1", "x2"}}, {T: time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC), Stadt: "Rom", Wer: []string{"x1", "x2"}}}
	n.bauen()
	x := n.Nahe("x1", 10)
	if x[0].Mit.ID != "x2" || x[1].Kante.Staerke >= x[0].Kante.Staerke/2 {
		t.Fatalf("x1: %v %+v", namenVon(x), x[:2])
	}
}

// Once a day: a second run within it reads nothing; the next day's
// rebuilds from Immich as it is, a person gone is gone (no drift).
func TestNetzTaeglichNeu(t *testing.T) {
	n, fake, jetzt := netzBox(t)
	fake.Bibliothek = fake.Bibliothek[3:]
	if err := n.Rechnen(t.Context()); err != nil || n.Nahe("p-anna", 10)[0].Kante.Tage != 3 {
		t.Fatalf("read again within the day: %v", err)
	}
	*jetzt = jetzt.Add(25 * time.Hour)
	if err := n.Rechnen(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := n.Nahe("p-anna", 10)[0]; got.Mit.ID != "p-ben" || got.Kante.Tage != 0 || n.Gruende("p-anna", got.Kante) != "gleiche Adresse · Ehepartner/in (Telefonbuch)" { // one day left: the phone book's only
		t.Fatalf("after a day: %+v", got)
	}
	again := &Netz{G: n.G, Loc: time.UTC, Now: n.Now} // after a restart, from the file
	if got := namenVon(again.Nahe("p-anna", 10)); !slices.Equal(got, namenVon(n.Nahe("p-anna", 10))) {
		t.Fatalf("from the file: %v", got)
	}
}

// Ausblenden hides a person where it would show, restorable; Nicht
// berechnen takes it out before any tie, the others' ties among
// themselves too; both are kept per key owner and come back.
func TestNetzVerbergen(t *testing.T) {
	n, _, _ := netzBox(t)
	if err := n.Verbergen("p-ben", Aus); err != nil {
		t.Fatal(err)
	}
	if got := namenVon(n.Nahe("p-anna", 10)); slices.Contains(got, "Ben Berg") || len(n.Nahe("p-ben", 10)) != 0 {
		t.Fatalf("ben shown: %v", got)
	}
	if err := n.Verbergen("p-kim", Nicht); err != nil {
		t.Fatal(err)
	}
	if got := namenVon(n.Nahe("p-fritz", 10)); slices.Contains(got, "Kim Berg") || len(n.kanten["p-kim"]) != 0 {
		t.Fatalf("kim computed: %v", got)
	}
	again := &Netz{G: n.G, Loc: time.UTC, Now: n.Now}
	if v := again.Verborgene(); len(v) != 2 || v[0].Name != "Ben Berg" || v[0].Art != Aus || v[1].Art != Nicht {
		t.Fatalf("kept: %+v", v)
	}
	again.Verbergen("p-kim", "")
	again.Verbergen("p-ben", "")
	if got := namenVon(again.Nahe("p-anna", 10)); got[0] != "Ben Berg" || !slices.Contains(got, "Kim Berg") || len(again.Verborgene()) != 0 {
		t.Fatalf("back: %v", got)
	}
	b, _ := os.ReadFile(filepath.Join(n.G.Dir, verborgenNam))
	if !strings.Contains(string(b), `"anna"`) {
		t.Fatalf("not per key owner: %s", b)
	}
}

// relationen reads what phones write: iOS's grouped label, Android's
// custom row with a type or its own label, vCard 4's RELATED.
func TestRelationen(t *testing.T) {
	root, err := caldav.Parse(vcard("x", "X", "item2.X-ABRELATEDNAMES:Ute\r\nitem2.X-ABLABEL:Patentante\r\nitem3.X-ABRELATEDNAMES:Max\r\nitem3.X-ABLabel:_$!<Mother>!$_\r\n"+
		"X-ANDROID-CUSTOM:vnd.android.cursor.item/relation;Lea;0;Nichte;;;;;;;;;;;;\r\nRELATED;TYPE=child,contact;VALUE=text:Tom\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range relationen(root.Children[0]) {
		got = append(got, r[0]+"="+label(r[1]))
	}
	if want := []string{"Ute=Patentante", "Max=Mutter", "Lea=Nichte", "Tom=Kind"}; !slices.Equal(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}
}

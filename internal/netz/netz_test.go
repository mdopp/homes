package netz

import (
	"os"
	"path/filepath"
	"slices"
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
	n := &Netz{G: g, Now: func() time.Time { return jetzt }}
	if err := n.Rechnen(t.Context()); err != nil {
		t.Fatal(err)
	}
	return n, fake, &jetzt
}

// knotenVon is node id as the net holds it.
func knotenVon(n *Netz, id string) Knoten {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	return n.knoten[id]
}

// The nodes are the named Immich persons not hidden and the contacts of
// both books, a contact on its person's node; the table is kept private.
func TestNetzKnoten(t *testing.T) {
	n, _, _ := netzBox(t)
	if n.Von("haushalt/b") != "p-ben" || n.Von("privat/urn:uuid:k2") != "p-fritz" || knotenVon(n, n.Von("haushalt/c")).Kontakt != "haushalt/c" {
		t.Fatalf("von: %q %q %q", n.Von("haushalt/b"), n.Von("privat/urn:uuid:k2"), n.Von("haushalt/c"))
	}
	if k := knotenVon(n, "p-kim"); k.Name != "Kim Berg" || k.Kontakt != "" {
		t.Fatalf("kim: %+v", k)
	}
	if k := knotenVon(n, "p-anna"); k.Kontakt != "haushalt/a" {
		t.Fatalf("anna: %+v", k)
	}
	if k := knotenVon(n, "p-hanna"); k.ID != "" {
		t.Fatalf("a hidden person is a node: %+v", k)
	}
	fi, err := os.Stat(filepath.Join(n.G.Dir, tabelle))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the table: %v %v", fi, err)
	}
}

// Once a day: a second run within it reads nothing; the next day's reads
// the phone books as they are (no drift); after a restart the table is
// read back.
func TestNetzTaeglichNeu(t *testing.T) {
	n, _, jetzt := netzBox(t)
	carla := n.Von("haushalt/c")
	if err := n.Eintragen(t.Context(), knotenVon(n, carla), knotenVon(n, "p-anna"), "kollege"); err != nil { // the cards only, no answer kept
		t.Fatal(err)
	}
	if err := n.Rechnen(t.Context()); err != nil || len(n.Beziehungen(carla)) != 0 {
		t.Fatalf("read again within the day: %v %v", err, n.Beziehungen(carla))
	}
	*jetzt = jetzt.Add(25 * time.Hour)
	if err := n.Rechnen(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := bezuegeVon(n.Beziehungen(carla)); !slices.Equal(got, []string{"Anna Berg=kollege|Kollege/Kollegin"}) {
		t.Fatalf("after a day: %v", got)
	}
	again := &Netz{G: n.G, Now: n.Now} // after a restart, from the file
	if got := bezuegeVon(again.Beziehungen(carla)); !slices.Equal(got, bezuegeVon(n.Beziehungen(carla))) {
		t.Fatalf("from the file: %v", got)
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

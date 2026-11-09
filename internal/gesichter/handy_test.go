package gesichter

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
)

// handyAufnahme records what the owner's phone is asked for.
func handyAufnahme(g *Gesichter) *[]string {
	var got []string
	g.Handy = func(wer string, a HandyAenderung) error {
		got = append(got, fmt.Sprintf("%s:%s|%s %s>%s %s|%v", wer, a.UID, a.AltVorname, a.AltNachname, a.Vorname, a.Nachname, a.Bild != nil))
		return nil
	}
	return &got
}

// The owner's own contact of a person renamed in Immich (owner
// 2026-11-09, „App erweitern“): the phone is asked once, the mirror is not
// written, Immich is not renamed back while the phone has not written it,
// and once its upload brings the name nothing more is asked or written.
func TestHandyFolgt(t *testing.T) {
	g, fake, dav := abgleichBox(t)
	got := handyAufnahme(g)
	dav.Put("/anna/handy-kontakte/a2.vcf", vcard("a2", "Anna Müller", "N:Müller;Anna;;;\r\n"))
	g.Merke("anna", "anna@box.test")
	for range 2 {
		g.Abgleichen(t.Context())
	}
	fake.Person("p-anna", "Anna Schmidt", "2026-11-09T10:00:00.000Z", false, Face(t, color.RGBA{200, 50, 50, 255}))
	writes := fake.Writes()
	for range 3 {
		g.Auffrischen(t.Context())
		if err := g.Abgleichen(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if fmt.Sprint(*got) != "[anna:a2|Anna Müller>Anna Schmidt|false]" {
		t.Fatalf("asked: %v", *got)
	}
	if a2 := karte(t, dav.Objects(), "/handy-kontakte/a2.vcf"); !strings.Contains(a2, "FN:Anna Müller") || fake.Writes() != writes {
		t.Fatalf("written meanwhile: %d to Immich, card\n%s", fake.Writes()-writes, a2)
	}
	dav.Put("/anna/handy-kontakte/a2.vcf", vcard("a2", "Anna Schmidt", "N:Schmidt;Anna;;;\r\n")) // the phone's next upload
	for range 2 {
		g.Auffrischen(t.Context())
		g.Abgleichen(t.Context())
	}
	if z := g.stand.Zuordnung["privat/a2"]; len(*got) != 1 || fake.Writes() != writes || z.Name != "Anna Schmidt" || z.Handy != "" {
		t.Fatalf("after the upload: %v, %d writes, %+v", *got, fake.Writes()-writes, z)
	}
}

// A person newly named in Immich that no contact has becomes a new contact
// on the owner's phone, with its face; one named before Homes first looked,
// one a contact has by person or by name, and one asked for already are
// not; nothing without the owner's login.
func TestNeuanlegen(t *testing.T) {
	g, fake, _ := abgleichBox(t)
	got := handyAufnahme(g)
	g.Abgleichen(t.Context())
	fake.Person("p-yv", "Yvonne  Huber", "1", false, Face(t, color.RGBA{10, 200, 10, 255}))
	g.Auffrischen(t.Context())
	if err := g.Neuanlegen(t.Context()); err != nil || len(*got) != 0 {
		t.Fatalf("without the owner: %v %v", *got, err)
	}
	g.Merke("anna", "anna@box.test")
	g.Neuanlegen(t.Context()) // the first look: what is named now is known
	if len(*got) != 0 {
		t.Fatalf("named before: %v", *got)
	}
	fake.Person("p-li", "Lia", "1", false, Face(t, color.RGBA{10, 10, 200, 255}))
	fake.Person("p-carla", "carla berg", "1", false, Face(t, color.RGBA{90, 10, 200, 255}))
	g.Auffrischen(t.Context())
	for range 2 {
		if err := g.Neuanlegen(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if fmt.Sprint(*got) != "[anna:| >Lia |true]" {
		t.Fatalf("asked: %v", *got)
	}
	neu := &Gesichter{Immich: g.Immich, Dir: g.Dir, Now: g.Now}
	again := handyAufnahme(neu)
	neu.Auffrischen(t.Context())
	if neu.Neuanlegen(t.Context()); len(*again) != 0 {
		t.Fatalf("asked again after a restart: %v", *again)
	}
}

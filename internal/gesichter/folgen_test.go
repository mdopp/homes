package gesichter

import (
	"image/color"
	"slices"
	"strings"
	"testing"
)

// karte is the household's card a, or the owner's own a2, as Radicale has it.
func karte(t *testing.T, objects map[string]string, pfad string) string {
	t.Helper()
	for p, text := range objects {
		if strings.HasSuffix(p, pfad) {
			return text
		}
	}
	t.Fatalf("no %s", pfad)
	return ""
}

// „Automatisch zurück“ (owner 2026-11-09): a person renamed in Immich
// renames its Haushalts-Kontakt (FN and N), and nothing goes back and
// forth: further runs write neither side. The owner's own Handy-Kontakt of
// the same person is not written (it is the phone's). Both renamed since
// the last run: the later wins, Immich's updatedAt against the card's REV.
func TestFolgenImmichZumKontakt(t *testing.T) {
	g, fake, dav := abgleichBox(t)
	dav.Put("/anna/handy-kontakte/a2.vcf", vcard("a2", "Anna Müller", ""))
	g.Merke("anna", "anna@box.test")
	for range 2 { // the first links, the second notes Immich's name
		if err := g.Abgleichen(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	face := Face(t, color.RGBA{200, 50, 50, 255})
	fake.Person("p-anna", "Anna Schmidt", "2026-11-09T10:00:00.000Z", false, face)
	g.Auffrischen(t.Context())
	if err := g.Abgleichen(t.Context()); err != nil {
		t.Fatal(err)
	}
	a := karte(t, dav.Objects(), "/haushalt-kontakte/a.vcf")
	if !strings.Contains(a, "FN:Anna Schmidt") || !strings.Contains(a, "N:Schmidt;Anna") {
		t.Fatalf("the card did not follow:\n%s", a)
	}
	if a2 := karte(t, dav.Objects(), "/handy-kontakte/a2.vcf"); !strings.Contains(a2, "FN:Anna Müller") {
		t.Fatalf("the phone's card was written:\n%s", a2)
	}
	if z := g.stand.Zuordnung["haushalt/a"]; z.ID != "p-anna" || z.Name != "Anna Schmidt" || z.Immich != "Anna Schmidt" {
		t.Fatalf("%+v", z)
	}
	writes, objects := fake.Writes(), dav.Objects()
	for range 2 {
		g.Auffrischen(t.Context())
		g.Abgleichen(t.Context())
	}
	if fake.Writes() != writes || karte(t, dav.Objects(), "/haushalt-kontakte/a.vcf") != objects["/homes/haushalt-kontakte/a.vcf"] || !slices.Contains(fake.Alle(), "Anna Schmidt|") {
		t.Fatalf("ping-pong: %d writes to Immich, card\n%s", fake.Writes()-writes, karte(t, dav.Objects(), "/haushalt-kontakte/a.vcf"))
	}

	// both renamed, Immich the later: the card takes Immich's
	dav.Put("/homes/haushalt-kontakte/a.vcf", vcard("a", "Anna Schmitt", "N:Schmitt;Anna;;;\r\nREV:20261109T100000Z\r\n"))
	fake.Person("p-anna", "Anna Schulz", "2026-11-09T11:00:00.000Z", false, face)
	g.Auffrischen(t.Context())
	g.Abgleichen(t.Context())
	if a := karte(t, dav.Objects(), "/haushalt-kontakte/a.vcf"); !strings.Contains(a, "FN:Anna Schulz") {
		t.Fatalf("Immich was the later:\n%s", a)
	}

	// both renamed, the card the later: the card keeps its name
	dav.Put("/homes/haushalt-kontakte/a.vcf", vcard("a", "Anna Maria Schulz", "N:Schulz;Anna Maria;;;\r\nREV:20261109T130000Z\r\n"))
	fake.Person("p-anna", "Anna Sommer", "2026-11-09T12:00:00.000Z", false, face)
	g.Auffrischen(t.Context())
	g.Abgleichen(t.Context())
	if a := karte(t, dav.Objects(), "/haushalt-kontakte/a.vcf"); !strings.Contains(a, "FN:Anna Maria Schulz") {
		t.Fatalf("the card was the later:\n%s", a)
	}
	if !slices.Contains(fake.Alle(), "Anna Sommer|") {
		t.Fatalf("a person Homes did not make was renamed: %v", fake.Alle())
	}
}

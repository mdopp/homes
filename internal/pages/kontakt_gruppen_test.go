package pages

import (
	"net/url"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
)

// gruppenBuch is the household's address book: Anna in Stammtisch and Homes' „Firma“, Ben in Chor and
// Stammtisch, Carl in none.
func gruppenBuch(t *testing.T) (*davfake.Server, *caldav.Client) {
	t.Helper()
	dav, c := addressBook(t, "b|Ben||", "c|Carl||")
	dav.Put("/homes/haushalt-kontakte/a.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:a\r\nFN:Anna\r\nCATEGORIES:Firma,Stammtisch\r\nEND:VCARD\r\n")
	dav.Put("/homes/haushalt-kontakte/b.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:b\r\nFN:Ben\r\nCATEGORIES:Chor,Stammtisch\r\nEND:VCARD\r\n")
	return dav, c
}

// owner 2026-11-10 (ADR 0002, „Gruppen“): everyone sees a shared card's groups, each a link to
// Kontakte picked by it; an admin — who corrects a shared card on its page — has each one's ✕ and
// „Zu Gruppe hinzufügen“ with the names used elsewhere; „Firma“ is never shown.
func TestKontaktGruppenShown(t *testing.T) {
	_, c := gruppenBuch(t)
	cfg := StartConfig{Contacts: c}
	_, body, _ := familyPage(t, Kontakt, cfg, "GET", "/kontakte/a", "admins")
	mustContain(t, body, "<h2>Gruppen</h2>", `<a href="/kontakte?gruppe=Stammtisch">Stammtisch</a><button name=gruppe-weg value="Stammtisch" aria-label="Aus „Stammtisch“ entfernen"`,
		`<form method=post class=gruppe-dazu action="/kontakte/a"><input type=hidden name=uid value="a">`, `Zu Gruppe hinzufügen<input name=gruppe-dazu list=gruppen-namen required maxlength=60`, `<datalist id=gruppen-namen><option value="Chor"></datalist>`)
	mustNotContain(t, body, ">Firma<", `value="Firma"`, `<option value="Stammtisch">`)
	_, body, _ = familyPage(t, Kontakt, cfg, "GET", "/kontakte/a", "family")
	mustContain(t, body, "<h2>Gruppen</h2>", `<a href="/kontakte?gruppe=Stammtisch">Stammtisch</a></li>`)
	mustNotContain(t, body, "name=gruppe-weg", "name=gruppe-dazu")
	if _, body, _ = familyPage(t, Kontakt, cfg, "GET", "/kontakte/c", "family"); strings.Contains(body, "Gruppen") {
		t.Error("a card in no group shows the section to who cannot add one")
	}
	_, body, _ = familyPage(t, Kontakt, cfg, "GET", "/kontakte/c", "admins")
	mustContain(t, body, "Noch in keiner Gruppe.", `<option value="Chor"><option value="Stammtisch">`)
}

// An admin adds a group (a new name or one used) and takes one away: one value of the card's
// CATEGORIES each, „Firma“ kept, said on the page; anyone else, an empty name or „Firma“ is refused.
func TestKontaktGruppeAendern(t *testing.T) {
	dav, c := gruppenBuch(t)
	cfg := StartConfig{Contacts: c}
	karte := func() string { return dav.Objects()["/homes/haushalt-kontakte/a.vcf"] }
	rec := frag(Kontakt(cfg), "POST", "/kontakte/a", "anna", "family", url.Values{"uid": {"a"}, "gruppe-dazu": {"Chor"}})
	if !strings.Contains(rec.Body.String(), "Gruppen eines Haushalts-Kontakts ändert nur ein Admin.") || !strings.Contains(karte(), "CATEGORIES:Firma,Stammtisch\r\n") {
		t.Fatalf("a family member's add:\n%s", karte())
	}
	rec = frag(Kontakt(cfg), "POST", "/kontakte/a", "anna", "admins", url.Values{"uid": {"a"}, "gruppe-dazu": {"  Kickermannschaft   Renegades "}})
	mustContain(t, rec.Body.String(), "Hinzugefügt: „Kickermannschaft Renegades“.", `<a href="/kontakte?gruppe=Kickermannschaft+Renegades">Kickermannschaft Renegades</a>`)
	if !strings.Contains(karte(), "CATEGORIES:Firma,Stammtisch,Kickermannschaft Renegades\r\n") {
		t.Fatalf("added:\n%s", karte())
	}
	rec = frag(Kontakt(cfg), "POST", "/kontakte/a", "anna", "admins", url.Values{"uid": {"a"}, "gruppe-weg": {"Stammtisch"}})
	mustContain(t, rec.Body.String(), "Aus „Stammtisch“ entfernt.")
	if !strings.Contains(karte(), "CATEGORIES:Firma,Kickermannschaft Renegades\r\n") {
		t.Fatalf("taken away:\n%s", karte())
	}
	for form, want := range map[string]string{"": "Bitte gib den Namen der Gruppe ein.", "firma": "„Firma“ nutzt Homes schon"} {
		mustContain(t, frag(Kontakt(cfg), "POST", "/kontakte/a", "anna", "admins", url.Values{"uid": {"a"}, "gruppe-dazu": {form}}).Body.String(), want)
	}
	mustContain(t, frag(Kontakt(cfg), "POST", "/kontakte/a", "anna", "admins", url.Values{"uid": {"weg"}, "gruppe-dazu": {"Chor"}}).Body.String(), "Diesen Kontakt gibt es nicht mehr.")
}

// Kontakte: the groups above the list, „Alle“ first; one picked lists its contacts alone, one
// nobody is in says so.
func TestKontakteNachGruppe(t *testing.T) {
	_, c := gruppenBuch(t)
	cfg := StartConfig{Contacts: c}
	_, body, _ := familyPage(t, Kontakte, cfg, "GET", "/kontakte", "family")
	mustContain(t, body, `<ul class=chips aria-label="Gruppen"><li><a href="/kontakte" aria-current=page>Alle</a></li><li><a href="/kontakte?gruppe=Chor">Chor</a></li><li><a href="/kontakte?gruppe=Stammtisch">Stammtisch</a></li></ul>`,
		">Anna<", ">Ben<", ">Carl<")
	_, body, _ = familyPage(t, Kontakte, cfg, "GET", "/kontakte?gruppe=stammtisch", "family")
	mustContain(t, body, `<a href="/kontakte?gruppe=Stammtisch" aria-current=page>Stammtisch</a>`, ">Anna<", ">Ben<")
	mustNotContain(t, body, ">Carl<", `<a href="/kontakte" aria-current=page>`)
	_, body, _ = familyPage(t, Kontakte, cfg, "GET", "/kontakte?gruppe=Golf", "family")
	mustContain(t, body, "In dieser Gruppe ist niemand.")
}

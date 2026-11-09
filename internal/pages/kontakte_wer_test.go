package pages

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// „Wer ist das?“ (owner 2026-11-09): above the pairs the five unnamed
// faces in the most photos, each with a name field, Speichern and Kenne
// ich nicht; Speichern names the face in Immich and says so, an empty
// name is said, Kenne ich nicht keeps it away, a face gone is said. A
// named side of a pair whose person a contact has links that contact's
// page; an unnamed side, and a named one without a contact, nothing.
func TestDieselbeWerIstDas(t *testing.T) {
	cfg, fake := dieselbeConfig(t, "a|Anna||")
	if err := cfg.Gesichter.Abgleichen(t.Context()); err != nil {
		t.Fatal(err)
	}
	rec := as(Dieselbe, cfg, DieselbePath, "anna", "anna@box.test")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, body)
	}
	mustContain(t, body, "<h2>Wer ist das?</h2>", `<input type=hidden name=wer value="u17"><input name=name list=wernamen required maxlength=120`,
		`<datalist id=wernamen>`, `<option value="Anna">`, `<option value="Xaver">`, // the household's people, one Immich does not know too (owner 2026-11-09)
		`<button name=antwort value=ja data-laeuft="Speichert …">Speichern</button>`, `class=leise formnovalidate data-laeuft="Merkt es sich …">Kenne ich nicht</button>`,
		"<i>ohne Namen</i><span>18 Fotos</span>", "<h2>Dieselbe Person?</h2>", `<small><a href="/kontakte/a" aria-label="Kontakt von Anna öffnen">Kontakt</a></small>`)
	if n := strings.Count(body, `<section class="paar wer">`); n != 5 || strings.Index(body, "Wer ist das?") > strings.Index(body, "<section class=paar>") {
		t.Fatalf("%d faces, or below the pairs", n)
	}
	if n := strings.Count(body, ">Kontakt</a>"); n != 2 { // Anna's two pairs among the first ten; Ben has no contact
		t.Fatalf("%d contact links", n)
	}
	mustNotContain(t, body, `name=wer value="u12"`) // the sixth

	rec = antworte(cfg, "anna", url.Values{"wer": {"u17"}, "name": {"  Yvonne "}, "antwort": {"ja"}})
	mustContain(t, rec.Body.String(), "Gespeichert: Das Gesicht heißt in Immich jetzt Yvonne.", `name=wer value="u12"`)
	if fake.ID("Yvonne") != "u17" || strings.Contains(rec.Body.String(), `name=wer value="u17"`) {
		t.Fatalf("not named: %q", fake.ID("Yvonne"))
	}
	mustContain(t, antworte(cfg, "anna", url.Values{"wer": {"u16"}, "name": {" "}, "antwort": {"ja"}}).Body.String(), "Der Name fehlt.", `name=wer value="u16"`)
	rec = antworte(cfg, "anna", url.Values{"wer": {"u16"}, "name": {"Egal"}, "antwort": {"nein"}})
	mustContain(t, rec.Body.String(), "Gemerkt: Nach diesem Gesicht fragt Homes nicht wieder.")
	mustNotContain(t, rec.Body.String(), `name=wer value="u16"`)
	if fake.ID("Egal") != "" {
		t.Fatal("Kenne ich nicht named it")
	}
	mustContain(t, antworte(cfg, "anna", url.Values{"wer": {"u16"}, "name": {"Zoe"}, "antwort": {"ja"}}).Body.String(), "Diesen Vorschlag gibt es nicht mehr.")
	if rec := antworte(cfg, "ben", url.Values{"wer": {"u15"}, "name": {"Zoe"}, "antwort": {"ja"}}); rec.Code != http.StatusSeeOther || fake.ID("Zoe") != "" {
		t.Fatalf("ben named a face: %d", rec.Code)
	}
}

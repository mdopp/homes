package pages

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/color"
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
	if n := strings.Count(body, ">Kontakt</a>"); n != 1 { // Anna's pair; Ben has no contact
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

// owner 2026-11-11 („bei Nein gleich den richtigen Namen“): a pair's Nein
// turns its unnamed face into „Wer ist das dann?“ at the top, the same
// field and buttons; a name is said and set in Immich, a missing one is
// said and the card stays, the named side's own name merges the two after
// all and says so. On the contact's page the same, posted there.
func TestDieselbeNeinWerDann(t *testing.T) {
	cfg, fake := dieselbeConfig(t, "a|Anna||")
	rec := antworte(cfg, "anna", url.Values{"a": {"p-ben"}, "b": {"u3"}, "antwort": {"nein"}})
	body := rec.Body.String()
	mustContain(t, body, "Gemerkt: Nach diesen beiden fragt Homes nicht wieder.", "<h2>Wer ist das dann?</h2>", `<input type=hidden name=wer value="u3"><input name=name list=wernamen required`, `<option value="Xaver">`)
	if strings.Index(body, "Wer ist das dann?") > strings.Index(body, "<h2>Wer ist das?</h2>") || strings.Count(body, "<datalist") != 1 {
		t.Fatal("not first, or two lists of names")
	}
	mustNotContain(t, as(Dieselbe, cfg, DieselbePath, "anna", "anna@box.test").Body.String(), "Wer ist das dann?")
	mustContain(t, antworte(cfg, "anna", url.Values{"wer": {"u3"}, "name": {" "}, "antwort": {"ja"}}).Body.String(), "Der Name fehlt.", "Wer ist das dann?", `name=wer value="u3"`)
	rec = antworte(cfg, "anna", url.Values{"wer": {"u3"}, "name": {"Yvonne"}, "antwort": {"ja"}})
	mustContain(t, rec.Body.String(), "Gespeichert: Das Gesicht heißt in Immich jetzt Yvonne.")
	mustNotContain(t, rec.Body.String(), "Wer ist das dann?")
	if fake.ID("Yvonne") != "u3" {
		t.Fatal("not named")
	}
	antworte(cfg, "anna", url.Values{"a": {"p-carla"}, "b": {"u6"}, "antwort": {"nein"}})
	mustContain(t, antworte(cfg, "anna", url.Values{"wer": {"u6"}, "name": {"carla"}, "antwort": {"ja"}}).Body.String(), "also doch dieselbe Person: Die Fotos gehören jetzt zu Carla.")
	if fmt.Sprint(fake.Merges()) != "[[p-carla u6]]" {
		t.Fatalf("%v", fake.Merges())
	}
	rec = antworteBei(Kontakt(cfg), "/kontakte/a", "anna", url.Values{"a": {"p-anna"}, "b": {"u0"}, "antwort": {"nein"}})
	mustContain(t, rec.Body.String(), "<h1>Anna</h1>", "<h2>Wer ist das dann?</h2>", `<form method=post action="/kontakte/a"><input type=hidden name=wer value="u0">`)
}

// owner 2026-11-11 (Android Chrome showed a datalist's names only now and
// then): the family pages' script shows a datalist field's matching names
// as rows to tap and drops the field's list; the page keeps the datalist
// for a browser without script (archiv#41), also on a page swapped in. e2e drives it at 411 px.
func TestNamenVorschlaege(t *testing.T) {
	for _, want := range []string{"querySelectorAll('input[list]').forEach(feld)", "namen();restore(k,same)", "removeAttribute('list')", ".slice(0,6)", "replace(/ä/g,'ae')", "b.textContent=n", "u.addEventListener('mousedown',function(e){e.preventDefault()})"} {
		if !strings.Contains(homesScript, want) {
			t.Fatalf("the script has no %q", want)
		}
	}
	mustContain(t, homesStyle, ".vorschlaege[hidden]{display:none}", ".vorschlaege button{display:block;width:100%;min-height:44px")
}

// homes#352: a face Immich holds for the one on a Haushalts-Kontakt's
// photo offers that name first, as one tap; nothing is named before it.
// The tap names the face in Immich and says so; an Immich that does not
// answer is said, and nothing changes.
func TestDieselbeWerVorschlagAusKontaktfoto(t *testing.T) {
	cfg, fake := dieselbeConfig(t, "a|Anna||")
	bild := faceJPEG(t, color.RGBA{90, 20, 160, 255})
	fake.Erkennen = func(_ string, jpeg []byte) []string {
		if bytes.Equal(jpeg, bild) {
			return []string{"u12"}
		}
		return nil
	}
	karte := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:k\r\nFN:Kerstin Kühn\r\nPHOTO;ENCODING=b;TYPE=JPEG:" + base64.StdEncoding.EncodeToString(bild) + "\r\nEND:VCARD\r\n"
	if err := cfg.Contacts.PutCard(t.Context(), cfg.Contacts.URL+"/homes/haushalt-kontakte/k.vcf", "", karte); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := cfg.Gesichter.Kontaktfotos(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	cfg.Gesichter.Auffrischen(t.Context())
	body := as(Dieselbe, cfg, DieselbePath, "anna", "anna@box.test").Body.String()
	mustContain(t, body, `<form method=post class=vorschlag action="/kontakte/dieselbe"><input type=hidden name=wer value="u12"><input type=hidden name=name value="Kerstin Kühn"><p>Sieht aus wie <b>Kerstin Kühn</b> aus deinen Kontakten.</p><button name=antwort value=ja data-laeuft="Speichert …">Ja, das ist Kerstin Kühn</button></form>`)
	if strings.Index(body, `name=wer value="u12"`) > strings.Index(body, `name=wer value="u17"`) || strings.Count(body, "class=vorschlag") != 1 {
		t.Fatal("the suggested face is not first, or suggested twice")
	}
	if fake.ID("Kerstin Kühn") != "" {
		t.Fatal("named before the tap")
	}
	rec := antworte(cfg, "anna", url.Values{"wer": {"u12"}, "name": {"Kerstin Kühn"}, "antwort": {"ja"}})
	mustContain(t, rec.Body.String(), "Gespeichert: Das Gesicht heißt in Immich jetzt Kerstin Kühn.")
	mustNotContain(t, rec.Body.String(), "class=vorschlag")
	if fake.ID("Kerstin Kühn") != "u12" {
		t.Fatal("not named")
	}
	fake.Close()
	mustContain(t, antworte(cfg, "anna", url.Values{"wer": {"u17"}, "name": {"Kerstin Kühn"}, "antwort": {"ja"}}).Body.String(), "Immich hat gerade nicht geantwortet, nichts wurde gespeichert.")
}

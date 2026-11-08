package napi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/haushalt"
	"github.com/mdopp/homes/internal/kontaktantwort"
)

// Kontakt prüfen lassen (owner 2026-11-07): the phone gets only its
// person's accepted changes, says which it wrote, and gets none again.
func TestKorrekturenForThePhone(t *testing.T) {
	store := &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)}
	vorher := kontaktantwort.Felder{Vorname: "Oma", Nachname: "Hilde", Telefone: []kontaktantwort.Telefon{{Nummer: "0711 445566", Art: "privat"}}}
	nachher := vorher
	nachher.Telefone = []kontaktantwort.Telefon{{Nummer: "0711 445577", Art: "privat"}}
	nachher.Position = "Rentnerin"
	token, f, _ := store.Neu("anna", "Anna", kontaktantwort.Privat, "3789r1-2B4D", vorher)
	store.Beantworten(token, kontaktantwort.Antwort{Aenderungen: kontaktantwort.Vergleich(vorher, nachher)})
	people := map[string]string{"anna1": "anna", "ben1": "ben"}
	h := Handler(Config{Validate: StaticValidator([]string{"anna1", "ben1"}), Haushalt: haushalt.Config{Contacts: &caldav.Client{URL: "http://127.0.0.1:1"}},
		Antworten: store, Whoami: func(b string) (string, bool) { p, ok := people[b]; return p, ok }})
	call := func(method, path, bearer, body string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bearer)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	if code, body := call("GET", "/napi/kontakte/korrekturen", "anna1", ""); code != 200 || body != `{"korrekturen":[]}`+"\n" {
		t.Fatalf("undecided: %d %s", code, body)
	}
	got, _ := store.Eine(f.ID, "anna")
	store.Entscheiden(f.ID, "anna", kontaktantwort.Stand(got, false, []int{0}))
	code, body := call("GET", "/napi/kontakte/korrekturen", "anna1", "")
	var out struct {
		Korrekturen []struct {
			ID, UID     string
			Aenderungen []map[string]any
		}
	}
	json.Unmarshal([]byte(body), &out)
	if code != 200 || len(out.Korrekturen) != 1 || out.Korrekturen[0].UID != "3789r1-2B4D" || len(out.Korrekturen[0].Aenderungen) != 1 {
		t.Fatalf("accepted: %d %s", code, body)
	}
	a := out.Korrekturen[0].Aenderungen[0]
	if a["nr"] != 0.0 || a["art"] != "telefon" || a["vorher"] != "0711 445566" || a["nachher"] != "0711 445577" || a["nachher_art"] != "privat" || a["stand"] != nil {
		t.Errorf("the change as the app reads it: %v", a)
	}
	if _, body := call("GET", "/napi/kontakte/korrekturen", "ben1", ""); strings.Contains(body, "3789") {
		t.Errorf("ben's phone gets anna's: %s", body)
	}
	if code, _ := call("POST", "/napi/kontakte/korrekturen/"+f.ID, "ben1", `{"geschrieben":[0]}`); code != http.StatusNotFound {
		t.Errorf("ben's phone marks anna's: %d", code)
	}
	if code, body := call("POST", "/napi/kontakte/korrekturen/"+f.ID, "anna1", `{"geschrieben":[0]}`); code != 200 {
		t.Fatalf("done: %d %s", code, body)
	}
	if _, body := call("GET", "/napi/kontakte/korrekturen", "anna1", ""); body != `{"korrekturen":[]}`+"\n" {
		t.Errorf("a written one again: %s", body)
	}
	if done, _ := store.Eine(f.ID, "anna"); done.Aenderungen[0].Stand != kontaktantwort.Uebernommen || done.Aenderungen[1].Stand != kontaktantwort.Abgelehnt {
		t.Errorf("stand %+v", done.Aenderungen)
	}
}

// owner 2026-11-08: the phone gets one street line (the card's own as
// before, the new one street and number as the country writes them), the
// main number, the photo as base64 and the message for the note.
func TestKorrekturenCarryTheStreetLineTheMainOneThePhotoAndTheNote(t *testing.T) {
	store := &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)}
	vorher := kontaktantwort.Felder{Vorname: "Oma", Adressen: []kontaktantwort.Adresse{{Strasse: "Hauptstraße", Hausnummer: "5", PLZ: "12345", Ort: "Stadt", Wie: "5 Hauptstraße"}}}
	nachher := kontaktantwort.Felder{Vorname: "Oma", Telefone: []kontaktantwort.Telefon{{Nummer: "0711 1", Art: "privat", Haupt: true}},
		Adressen: []kontaktantwort.Adresse{{Strasse: "Hauptstraße", Hausnummer: "7", PLZ: "12345", Ort: "Stadt"}}, Foto: []byte{0xff, 0xd8}, Nachricht: "Neu hier"}
	token, f, _ := store.Neu("anna", "Anna", kontaktantwort.Privat, "k", vorher)
	store.Beantworten(token, kontaktantwort.Antwort{Aenderungen: kontaktantwort.Vergleich(vorher, nachher)})
	got, _ := store.Eine(f.ID, "anna")
	store.Entscheiden(f.ID, "anna", kontaktantwort.Stand(got, false, []int{0, 1, 2, 3}))
	h := Handler(Config{Validate: StaticValidator([]string{"anna1"}), Haushalt: haushalt.Config{Contacts: &caldav.Client{URL: "http://127.0.0.1:1"}},
		Antworten: store, Whoami: func(string) (string, bool) { return "anna", true }})
	req := httptest.NewRequest("GET", "/napi/kontakte/korrekturen", nil)
	req.Header.Set("Authorization", "Bearer anna1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	for _, want := range []string{`"nachher_haupt":true`, `"vorher_adresse":{"strasse":"5 Hauptstraße","plz":"12345","ort":"Stadt"}`,
		`"nachher_adresse":{"strasse":"Hauptstraße 7","plz":"12345","ort":"Stadt"}`, `"art":"foto"`, `"bild":"/9g="`, `"art":"notiz"`, `"nachher":"Neu hier"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %s in %s", want, rec.Body.String())
		}
	}
	if strings.Contains(rec.Body.String(), `"wie"`) || strings.Contains(rec.Body.String(), `"hausnummer"`) {
		t.Errorf("the page's parts reach the phone: %s", rec.Body.String())
	}
}

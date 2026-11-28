package napi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/caldav/davfake"
	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/haushalt"
	"github.com/mdopp/homes/internal/kontaktantwort"
)

// fragenHandler is a house with Anna (lacks address and birthday), two Beispiels on one landline
// and a complete Ben; michael and petra admins, anna not (as einrichten.Admins last saw them).
func fragenHandler(t *testing.T, basis string) (http.Handler, *davfake.Server, *kontaktantwort.Store) {
	t.Helper()
	fake := davfake.New("homes", "pw")
	t.Cleanup(fake.Close)
	fake.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	fake.Put("/homes/haushalt-kontakte/a.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:a\r\nFN:Anna Beispiel\r\nN:Beispiel;Anna;;;\r\nTEL;TYPE=CELL:+49 151 23456789\r\nEMAIL:anna@example.org\r\nEND:VCARD\r\n")
	fake.Put("/homes/haushalt-kontakte/c.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:c\r\nFN:Clara Beispiel\r\nTEL;TYPE=HOME:0711 445566\r\nEND:VCARD\r\n")
	fake.Put("/homes/haushalt-kontakte/d.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:d\r\nFN:Dora Beispiel\r\nTEL;TYPE=HOME:0711 445566\r\nEND:VCARD\r\n")
	fake.Put("/homes/haushalt-kontakte/b.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:b\r\nFN:Ben Muster\r\nN:Muster;Ben;;;\r\nTEL:0160 9876543\r\nEMAIL:ben@example.org\r\n"+
		"ADR:;;Hauptstraße 5;Stadt;;12345;\r\nBDAY:1990-04-01\r\nEND:VCARD\r\n")
	q := &einrichten.Quellen{Admins: einrichten.OpenAdmins(t.TempDir())}
	q.SahAdmin("michael", true)
	q.SahAdmin("petra", true)
	q.SahAdmin("anna", false)
	store := &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)}
	people := map[string]string{"m1": "michael", "p1": "petra", "a1": "anna", "x1": "../x"}
	h := Handler(Config{Validate: StaticValidator([]string{"m1", "p1", "a1", "x1"}), Whoami: func(b string) (string, bool) { p, ok := people[b]; return p, ok },
		Haushalt:  haushalt.Config{Contacts: &caldav.Client{URL: fake.URL, User: "homes", Password: "pw"}, PhoneCountry: "49"},
		Antworten: store, KontaktURL: basis, Einrichtung: q})
	return h, fake, store
}

// fragenAntwort is what the app reads.
type fragenAntwort struct {
	OK         bool                `json:"ok"`
	Error      string              `json:"error"`
	Kontakt    string              `json:"kontakt"`
	Vorname    string              `json:"vorname"`
	Link       string              `json:"link"`
	Text       string              `json:"text"`
	Fehlt      []string            `json:"fehlt"`
	Telefon    *string             `json:"telefon"`
	Neu        bool                `json:"neu"`
	Anlegen    bool                `json:"anlegen"`
	Kandidaten []map[string]string `json:"kandidaten"`
}

func frage(t *testing.T, h http.Handler, bearer, body string) (int, fragenAntwort, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/napi/kontakte/fragen", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out fragenAntwort
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("not JSON (%d): %s", rec.Code, rec.Body.String())
	}
	return rec.Code, out, rec.Body.String()
}

var fragenLink = regexp.MustCompile(`^https://kontakt\.example\.org/[A-Za-z0-9_-]{43}$`)

// owner 2026-11-28: the phone's contact found by its number, the link as „Anna selbst fragen“
// makes it, the message, what it lacks, the mobile number — for an admin of the box or not.
func TestKontakteFragenFoundByNumber(t *testing.T) {
	h, _, store := fragenHandler(t, "https://kontakt.example.org/")
	for _, bearer := range []string{"m1", "p1"} {
		code, out, raw := frage(t, h, bearer, `{"name":"Anni","telefon":["0151 2345 6789"],"mail":["falsch@example.org"]}`)
		if code != 200 || !out.OK || out.Kontakt != "Anna Beispiel" || out.Vorname != "Anna" || !fragenLink.MatchString(out.Link) ||
			out.Telefon == nil || *out.Telefon != "+4915123456789" || strings.Join(out.Fehlt, ",") != "Adresse,Geburtstag" || out.Neu || strings.Contains(raw, `"neu"`) {
			t.Fatalf("%s: %d %s", bearer, code, raw)
		}
		if !strings.HasPrefix(out.Text, "Hallo Anna, ") || !strings.HasSuffix(out.Text, out.Link) || !strings.Contains(out.Text, "nur einmal") {
			t.Errorf("the message: %q", out.Text)
		}
		f, err := store.Zum(strings.TrimPrefix(out.Link, "https://kontakt.example.org/"))
		if err != nil || f.Buch != kontaktantwort.Haushalt || f.UID != "a" || f.Wer == "" || f.Vorher.Vorname != "Anna" {
			t.Errorf("the link's question: %+v %v", f, err)
		}
	}
	// a complete card lacks nothing; by its e-mail when no number fits
	code, out, raw := frage(t, h, "m1", `{"name":"B. Muster","telefon":["0170 0000000"],"mail":["BEN@example.org"]}`)
	if code != 200 || out.Kontakt != "Ben Muster" || len(out.Fehlt) != 0 || !strings.Contains(raw, `"fehlt":[]`) || *out.Telefon != "+491609876543" {
		t.Errorf("by e-mail: %d %s", code, raw)
	}
	// by the exact name last; no mobile: telefon ""
	code, out, raw = frage(t, h, "m1", `{"name":"Clara Beispiel"}`)
	if code != 200 || out.Kontakt != "Clara Beispiel" || out.Telefon == nil || *out.Telefon != "" || strings.Join(out.Fehlt, ",") != "Mobilnummer,E-Mail,Adresse,Geburtstag" {
		t.Errorf("by name: %d %s", code, raw)
	}
}

// Two cards on one number, no other hint: 409 with both; the app's choice by uid; a uid gone 404.
func TestKontakteFragenAmbiguous(t *testing.T) {
	h, _, _ := fragenHandler(t, "https://kontakt.example.org")
	code, out, raw := frage(t, h, "m1", `{"name":"Beispiel","telefon":["+49 711 445566"]}`)
	if code != http.StatusConflict || out.OK || out.Error != "Zu Beispiel passen mehrere Kontakte im Haushalts-Adressbuch. Wähle den richtigen." || len(out.Kandidaten) != 2 ||
		out.Kandidaten[0]["kontakt"] != "Clara Beispiel" || out.Kandidaten[0]["uid"] != "c" || out.Kandidaten[1]["uid"] != "d" {
		t.Fatalf("ambiguous: %d %s", code, raw)
	}
	if code, out, raw := frage(t, h, "m1", `{"name":"Dora","telefon":["+49 711 445566"]}`); code != http.StatusConflict || len(out.Kandidaten) != 2 {
		t.Errorf("a name that is neither: %d %s", code, raw) // narrowing keeps both when the name fits none
	}
	if code, out, raw := frage(t, h, "m1", `{"name":"Dora Beispiel","telefon":["+49 711 445566"]}`); code != 200 || out.Kontakt != "Dora Beispiel" {
		t.Errorf("narrowed by name: %d %s", code, raw)
	}
	if code, out, raw := frage(t, h, "m1", `{"name":"Beispiel","telefon":["+49 711 445566"],"uid":"d"}`); code != 200 || out.Kontakt != "Dora Beispiel" || out.Vorname != "Dora" {
		t.Errorf("chosen: %d %s", code, raw)
	}
	if code, out, raw := frage(t, h, "m1", `{"name":"Beispiel","uid":"weg"}`); code != 404 || out.Error != "Diesen Kontakt gibt es im Haushalts-Adressbuch nicht mehr." || out.Anlegen {
		t.Errorf("a uid gone: %d %s", code, raw)
	}
}

// Not in the book: 404 in words with anlegen; with "anlegen":true the card is made there from the
// phone's name, numbers (the household's form) and e-mails, and asked as found, neu.
func TestKontakteFragenNotFoundAndMade(t *testing.T) {
	h, fake, _ := fragenHandler(t, "https://kontakt.example.org")
	body := `{"name":"Emil Neu","telefon":["0171 7654321","kein Telefon"],"mail":["emil@example.org","kaputt@"]}`
	code, out, raw := frage(t, h, "m1", body)
	if code != 404 || out.OK || out.Error != "Emil Neu steht nicht im Haushalts-Adressbuch." || !out.Anlegen {
		t.Fatalf("not found: %d %s", code, raw)
	}
	if n := len(fake.Objects()); n != 4 {
		t.Fatalf("a card written without anlegen: %d", n)
	}
	code, out, raw = frage(t, h, "m1", strings.Replace(body, `{`, `{"anlegen":true,`, 1))
	if code != 200 || !out.OK || !out.Neu || out.Kontakt != "Emil Neu" || out.Vorname != "Emil" || *out.Telefon != "+491717654321" ||
		strings.Join(out.Fehlt, ",") != "Adresse,Geburtstag" || !fragenLink.MatchString(out.Link) {
		t.Fatalf("made: %d %s", code, raw)
	}
	var card string
	for p, v := range fake.Objects() {
		if strings.Contains(v, "Emil Neu") && strings.HasPrefix(p, "/homes/haushalt-kontakte/") {
			card = v
		}
	}
	if !strings.Contains(card, "+49 171 7654321") || !strings.Contains(card, "emil@example.org") || strings.Contains(card, "kein Telefon") || strings.Contains(card, "kaputt") {
		t.Errorf("the new card:\n%s", card)
	}
	// asked again: found by its number now, made once
	if code, out, raw := frage(t, h, "m1", strings.Replace(body, `{`, `{"anlegen":true,`, 1)); code != 200 || out.Neu {
		t.Errorf("again: %d %s", code, raw)
	}
	// without a name there is nothing to make it from
	if code, out, raw := frage(t, h, "m1", `{"telefon":["0170 2222222"],"anlegen":true}`); code != 404 || out.Anlegen || out.Error != "0170 2222222 steht nicht im Haushalts-Adressbuch." {
		t.Errorf("no name: %d %s", code, raw)
	}
}

// Who may ask, and what is refused in words: a family member 403, a device without a person 403,
// nothing sent 400, the request unreadable 400, no public page 503; with kontakte off no route.
func TestKontakteFragenRefusals(t *testing.T) {
	h, _, store := fragenHandler(t, "https://kontakt.example.org")
	for _, c := range []struct {
		bearer, body, want string
		code               int
	}{
		{"a1", `{"name":"Anna Beispiel"}`, "Kontakte des Hauses lässt nur ein Admin prüfen.", 403},
		{"x1", `{"name":"Anna Beispiel"}`, "", 403},
		{"m1", `{}`, "Der Kontakt kam ohne Namen, Nummer und E-Mail an.", 400},
		{"m1", `{"name":`, "Die Anfrage war nicht lesbar.", 400},
	} {
		code, out, raw := frage(t, h, c.bearer, c.body)
		if code != c.code || out.OK || out.Error == "" || !strings.Contains(out.Error, c.want) || strings.Contains(raw, "kontakt.example.org") {
			t.Errorf("%s %s: %d %s", c.bearer, c.body, code, raw)
		}
	}
	if fs := store.ZuPruefen("anna"); len(fs) != 0 {
		t.Errorf("a link for anna: %v", fs)
	}
	ohne, _, _ := fragenHandler(t, "")
	if code, out, _ := frage(t, ohne, "m1", `{"name":"Anna Beispiel"}`); code != 503 || !strings.Contains(out.Error, "Sag dem Admin Bescheid") {
		t.Errorf("no public page: %d %+v", code, out)
	}
	aus := Handler(Config{Validate: StaticValidator([]string{"m1"}), Whoami: func(string) (string, bool) { return "michael", true }, Antworten: store, KontaktURL: "https://k.example.org"})
	req := httptest.NewRequest("POST", "/napi/kontakte/fragen", strings.NewReader(`{"name":"Anna"}`))
	req.Header.Set("Authorization", "Bearer m1")
	rec := httptest.NewRecorder()
	aus.ServeHTTP(rec, req)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "not_found") {
		t.Errorf("kontakte off: %d %s", rec.Code, rec.Body.String())
	}
}

// Radicale not answering: 502 in words, nothing made.
func TestKontakteFragenRadicaleDown(t *testing.T) {
	q := &einrichten.Quellen{Admins: einrichten.OpenAdmins(t.TempDir())}
	q.SahAdmin("michael", true)
	h := Handler(Config{Validate: StaticValidator([]string{"m1"}), Whoami: func(string) (string, bool) { return "michael", true }, Einrichtung: q,
		Haushalt: haushalt.Config{Contacts: &caldav.Client{URL: "http://127.0.0.1:1"}}, Antworten: &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), "a.json")}, KontaktURL: "https://k.example.org"})
	if code, out, _ := frage(t, h, "m1", `{"name":"Anna","anlegen":true}`); code != http.StatusBadGateway || out.OK || out.Error != "Das Haushalts-Adressbuch lässt sich gerade nicht lesen. Versuch es gleich noch einmal." {
		t.Errorf("down: %d %+v", code, out)
	}
}

package napi

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/teilen"
)

// teilenHandler: michael an admin, anna not; both paired.
func teilenHandler(t *testing.T) (*teilen.Store, *kontaktantwort.Store, func(method, path, bearer, body string) (int, map[string]any)) {
	t.Helper()
	q := &einrichten.Quellen{Admins: einrichten.OpenAdmins(t.TempDir())}
	q.SahAdmin("michael", true)
	q.SahAdmin("anna", false)
	st, antworten := &teilen.Store{}, &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)}
	people := map[string]string{"m1": "michael", "a1": "anna", "x1": "../x"}
	h := Handler(Config{Validate: StaticValidator([]string{"m1", "a1", "x1"}), Whoami: func(b string) (string, bool) { p, ok := people[b]; return p, ok },
		Teilen: st, Antworten: antworten, KontaktURL: "https://kontakt.example.org", Einrichtung: q})
	return st, antworten, func(method, path, bearer, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bearer)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: not JSON (%d): %s", method, path, rec.Code, rec.Body.String())
		}
		return rec.Code, out
	}
}

var teilenLink = regexp.MustCompile(`^https://kontakt\.example\.org/t/([A-Za-z0-9_-]{43})$`)

// owner 2026-11-28: a session for any paired person, its link, 30 minutes; the app sets what the
// page shows, reads whether it was opened, and ends it — its own only.
func TestTeilenSession(t *testing.T) {
	st, _, call := teilenHandler(t)
	code, out := call("POST", "/napi/teilen", "a1", `{"vorname":"Anna"}`)
	m := teilenLink.FindStringSubmatch(out["link"].(string))
	bis, err := time.Parse(time.RFC3339, out["gueltig_bis"].(string))
	if code != 201 || out["ok"] != true || m == nil || err != nil || time.Until(bis) < 29*time.Minute || out["id"] == "" {
		t.Fatalf("POST: %d %v", code, out)
	}
	id, token := out["id"].(string), m[1]
	if a, _, _ := st.Zum(token, false); a.Vorname != "Anna" {
		t.Errorf("the page's name: %q", a.Vorname)
	}
	if code, out := call("GET", "/napi/teilen/"+id, "a1", ""); code != 200 || out["geoeffnet"] != false || out["eingetragen"] != false || out["gueltig_bis"] == nil {
		t.Errorf("GET: %d %v", code, out)
	}
	if code, out := call("PUT", "/napi/teilen/"+id, "a1", `{"vorname":"Anna","karte":{"mobil":"0151 2345678"},"wlan":{"ssid":"Gast","kennwort":"pw"}}`); code != 200 || out["ok"] != true {
		t.Errorf("PUT: %d %v", code, out)
	}
	a, _, _ := st.Zum(token, true)
	if a.Karte == nil || a.Karte.Mobil != "0151 2345678" || a.Karte.Name != "" || a.WLAN.SSID != "Gast" || a.Eintragen {
		t.Errorf("released: %+v", a)
	}
	if _, out := call("GET", "/napi/teilen/"+id, "a1", ""); out["geoeffnet"] != true {
		t.Errorf("opened: %v", out)
	}
	if code, out := call("PUT", "/napi/teilen/"+id, "a1", `{"karte":{"geburtstag":"morgen"}}`); code != 400 || out["ok"] != false || out["error"] == "" {
		t.Errorf("a birthday that is no day: %d %v", code, out)
	}
	if code, _ := call("DELETE", "/napi/teilen/"+id, "a1", ""); code != 200 {
		t.Errorf("DELETE: %d", code)
	}
	if a, _, _ := st.Zum(token, false); !a.Beendet || a.Karte != nil || a.WLAN != nil {
		t.Errorf("ended: %+v", a)
	}
	for _, method := range []string{"GET", "PUT"} {
		if code, out := call(method, "/napi/teilen/"+id, "a1", `{}`); code != 410 || out["error"] != "Dieses Teilen ist schon vorbei. Starte ein neues." {
			t.Errorf("%s after the end: %d %v", method, code, out)
		}
	}
	if code, _ := call("POST", "/napi/teilen", "m1", ""); code != 201 { // no body: the login's name
		t.Errorf("POST without a body: %d", code)
	}
}

// Another person's session answers as one that never was; a device without a person is refused.
func TestTeilenOnlyItsPerson(t *testing.T) {
	_, _, call := teilenHandler(t)
	_, out := call("POST", "/napi/teilen", "m1", "")
	id := out["id"].(string)
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		if code, out := call(method, "/napi/teilen/"+id, "a1", `{"wlan":{"ssid":"x"}}`); code != 404 || out["error"] != "Dieses Teilen gibt es nicht (mehr)." {
			t.Errorf("%s by anna: %d %v", method, code, out)
		}
	}
	if code, _ := call("GET", "/napi/teilen/"+id, "m1", ""); code != 200 {
		t.Errorf("michael's own: %d", code)
	}
	if code, _ := call("POST", "/napi/teilen", "x1", ""); code != 403 {
		t.Errorf("no person: %d", code)
	}
}

// eintragen is an admin's: it makes one link of Kontaktdaten eintragen lassen for the page, kept
// across PUTs; GET says once the guest answered it.
func TestTeilenEintragenOnlyForAnAdmin(t *testing.T) {
	st, antworten, call := teilenHandler(t)
	_, out := call("POST", "/napi/teilen", "a1", "")
	if code, out := call("PUT", "/napi/teilen/"+out["id"].(string), "a1", `{"eintragen":true}`); code != 403 || out["ok"] != false || !strings.Contains(out["error"].(string), "Admin") {
		t.Errorf("anna: %d %v", code, out)
	}
	_, out = call("POST", "/napi/teilen", "m1", `{"vorname":"Michael"}`)
	id, token := out["id"].(string), teilenLink.FindStringSubmatch(out["link"].(string))[1]
	call("PUT", "/napi/teilen/"+id, "m1", `{"eintragen":true}`)
	a, _, _ := st.Zum(token, false)
	call("PUT", "/napi/teilen/"+id, "m1", `{"eintragen":true,"wlan":{"ssid":"Gast"}}`)
	b, _, _ := st.Zum(token, false)
	if !a.Eintragen || a.Einladung.Token == "" || b.Einladung != a.Einladung {
		t.Fatalf("the form's link: %+v then %+v", a.Einladung, b.Einladung)
	}
	f, err := antworten.Zum(a.Einladung.Token)
	if err != nil || !f.Neu || f.Wer != "michael" || f.Von != "Michael" {
		t.Fatalf("the link: %+v %v", f, err)
	}
	antworten.Beantworten(a.Einladung.Token, kontaktantwort.Antwort{Aenderungen: []kontaktantwort.Aenderung{{Art: "vorname", Nachher: "Erika"}}})
	if _, out := call("GET", "/napi/teilen/"+id, "m1", ""); out["eingetragen"] != true {
		t.Errorf("after the answer: %v", out)
	}
	if len(antworten.ZuPruefen("michael")) != 1 {
		t.Errorf("the answer waits for michael's review: %v", antworten.ZuPruefen("michael"))
	}
}

// At most five open: the sixth is said in words.
func TestTeilenFiveOpen(t *testing.T) {
	_, _, call := teilenHandler(t)
	for range teilen.OffenJePerson {
		call("POST", "/napi/teilen", "a1", "")
	}
	if code, out := call("POST", "/napi/teilen", "a1", ""); code != 429 || out["error"] != "Du teilst gerade schon fünfmal. Beende eins davon, dann geht es wieder." {
		t.Errorf("a sixth: %d %v", code, out)
	}
}

// The Wi-Fi password never reaches the log, not even from a request Homes could not read.
func TestTeilenPasswordNotLogged(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	_, _, call := teilenHandler(t)
	_, out := call("POST", "/napi/teilen", "m1", "")
	id := out["id"].(string)
	call("PUT", "/napi/teilen/"+id, "m1", `{"wlan":{"ssid":"Gast-Netz","kennwort":"Sup3rGeheim"},"karte":{"mobil":"0170 999"}}`)
	call("PUT", "/napi/teilen/"+id, "m1", `{"wlan":{"ssid":"Gast-Netz","kennwort":"Sup3rGeheim"`)
	call("PUT", "/napi/teilen/"+id, "m1", `{"wlan":{"ssid":"Gast-Netz","kennwort":"`+strings.Repeat("Sup3rGeheim", 20)+`"}}`)
	call("DELETE", "/napi/teilen/"+id, "m1", "")
	for _, secret := range []string{"Sup3rGeheim", "Gast-Netz", "0170 999"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("%q in the log:\n%s", secret, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "released karte=true wlan=true") {
		t.Errorf("the log says nothing (the check could not fail):\n%s", logs.String())
	}
}

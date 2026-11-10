package pages

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/uebernahme"
)

// archivPaket signs p as the person's archive does (archiv apps/archiv/uebernahme.go).
func archivPaket(secret string, p uebernahme.Paket) url.Values {
	b, _ := json.Marshal(p)
	d := base64.RawURLEncoding.EncodeToString(b)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("archiv-uebernahme-1." + d))
	return url.Values{"d": {d}, "s": {base64.RawURLEncoding.EncodeToString(m.Sum(nil))}}
}

// „Ins Adressbuch übernehmen“ (archiv#27 step 2): the archive's post shows
// each line and where it goes, writes nothing; Übernehmen from Homes' own
// page writes — two household contacts into both cards by UID, the phone
// contact's only to the phone — and says so. Another login, a forged or
// old packet, a confirmation from another site: refused, nothing written.
func TestUebernehmen(t *testing.T) {
	cfg := netzConfig(t)
	jetzt := time.Date(2026, 11, 10, 9, 40, 0, 0, time.UTC)
	cfg.Now, cfg.ArchivGeheim = func() time.Time { return jetzt }, map[string]string{"anna": "anna-geheim", "ben": "ben-geheim"}
	cfg.Archive, cfg.ArchivURL = map[string]bool{"anna": true}, "https://archiv.box.test"
	cfg.Antworten = &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)}
	p := uebernahme.Paket{Person: "anna", Bis: jetzt.Add(time.Hour), Zeilen: []uebernahme.Zeile{
		{X: uebernahme.Ende{Name: "Ben", Haushalt: "b"}, Y: uebernahme.Ende{Name: "Anna", Haushalt: "a"}, Art: "partner"},
		{X: uebernahme.Ende{Name: "Kim Privat", Handy: "k7"}, Y: uebernahme.Ende{Name: "Anna"}, Art: "kind"}}}
	form := archivPaket("anna-geheim", p)
	karten := func() string {
		cards, _, _ := cfg.Contacts.Cards(t.Context())
		var all string
		for _, c := range cards {
			all += c.Text()
		}
		return all
	}

	post := func(user, site string, v url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", UebernehmenPath, strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Remote-User", user)
		req.Header.Set("Sec-Fetch-Site", site)
		rec := httptest.NewRecorder()
		Uebernehmen(cfg).ServeHTTP(rec, req)
		return rec
	}
	rec := post("anna", "same-site", form) // the archive's tap: archiv.<domain> → www.<domain>
	body := rec.Body.String()
	mustContain(t, body, "<h1>Ins Adressbuch übernehmen</h1>", "Ben ist Partner/in von Anna<span class=k>→ Haushalts-Adressbuch, sieht die Familie</span>",
		"Kim Privat ist Kind von Anna<span class=k>→ nur dein Handy</span>", `<button name=ja value=1 data-laeuft="Schreibt ins Adressbuch …">Übernehmen</button>`,
		`href="https://archiv.box.test/tresor/umfeld">Zurück zu deinem Archiv`)
	if rec.Code != 200 || strings.Contains(karten(), "RELATED") {
		t.Fatalf("%d, written before the confirmation:\n%s", rec.Code, karten())
	}

	ja := url.Values{"d": form["d"], "s": form["s"], "ja": {"1"}}
	if rec := post("anna", "same-site", ja); rec.Code != http.StatusForbidden || strings.Contains(karten(), "RELATED") {
		t.Fatalf("confirmed from another site: %d", rec.Code)
	}
	for name, c := range map[string]struct {
		user string
		v    url.Values
	}{
		"ben":        {"ben", ja},
		"forged":     {"anna", archivPaket("ben-geheim", p)},
		"tampered":   {"anna", url.Values{"d": {form.Get("d") + "x"}, "s": form["s"], "ja": {"1"}}},
		"nobody":     {"", form},
		"no archive": {"carla", ja},
	} {
		if rec := post(c.user, "same-origin", c.v); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Das lässt sich nicht übernehmen") || strings.Contains(rec.Body.String(), "name=ja") {
			t.Errorf("%s: %d\n%s", name, rec.Code, rec.Body)
		}
	}
	if strings.Contains(karten(), "RELATED") {
		t.Fatal("a refused one wrote")
	}

	body = post("anna", "same-origin", ja).Body.String()
	mustContain(t, body, "Übernommen. Was auf dein Handy geht, kommt dort an, sobald sich die Homes-App das nächste Mal meldet.")
	mustNotContain(t, body, "name=ja")
	all := karten()
	if !strings.Contains(all, "RELATED;TYPE=spouse:urn:uuid:b\r\n") || !strings.Contains(all, "RELATED;TYPE=spouse:urn:uuid:a\r\n") || strings.Contains(all, "Kim") {
		t.Fatalf("the household cards:\n%s", all)
	}
	if b, _ := os.ReadFile(cfg.Antworten.Path); !strings.Contains(string(b), "k7") || !strings.Contains(string(b), "Anna") {
		t.Fatalf("not for anna's phone: %s", b)
	}

	jetzt = jetzt.Add(2 * time.Hour)
	if rec := post("anna", "same-origin", ja); rec.Code != http.StatusBadRequest {
		t.Fatalf("an old packet: %d", rec.Code)
	}
	req := httptest.NewRequest("GET", UebernehmenPath, nil)
	rec = httptest.NewRecorder()
	Uebernehmen(cfg).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET: %d", rec.Code)
	}
}

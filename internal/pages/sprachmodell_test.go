package pages

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/modellease"
	"github.com/mdopp/homes/internal/modellease/leasefake"
	"github.com/mdopp/homes/internal/sprachmodell"
)

const modellKey = "modell-key-geheim-4711"

func modellSeite(t *testing.T) (StartConfig, *leasefake.Server) {
	t.Helper()
	at := time.Date(2026, 11, 17, 20, 0, 0, 0, time.UTC)
	f := leasefake.New(modellKey)
	f.Now = func() time.Time { return at }
	t.Cleanup(f.Close)
	return StartConfig{Loc: time.UTC, Now: f.Now,
		Sprachmodell: &sprachmodell.Schalter{Lease: &modellease.Client{Base: f.Base(), Key: modellKey}, Now: f.Now}}, f
}

func getModell(cfg StartConfig) string {
	req := httptest.NewRequest("GET", SprachmodellPath, nil)
	req.Header.Set("Remote-User", "anna")
	rec := httptest.NewRecorder()
	Sprachmodell(cfg).ServeHTTP(rec, req)
	return rec.Body.String()
}

func takes(f *leasefake.Server) int {
	n := 0
	for _, r := range f.Seen() {
		if r.Method != http.MethodGet {
			n++
		}
	}
	return n
}

// The card in its three states (owner 2026-11-17): the household, another
// model for a person until a time with what it costs and the way back,
// and a lease that does not answer.
func TestSprachmodellCard(t *testing.T) {
	cfg, f := modellSeite(t)
	page := getModell(cfg)
	for _, want := range []string{"<h1>Sprachmodell</h1>", "Haushalt · Qwen3.6 35B A3B", "Sprachsteuerung läuft", "Gemma 4 12B", ">1 Std.<", ">4 Std.<",
		"Auf Gemma 4 12B umschalten? Die Sprachsteuerung pausiert bis 21:00.", `action=/sprachmodell`, `value="gemma-4-12b@4"`, "pausiert die Sprachsteuerung"} {
		if !strings.Contains(page, want) {
			t.Errorf("household: lacks %q", want)
		}
	}
	if strings.Contains(page, "Zurück zum Haushalt") || strings.Contains(page, ">Qwen3.6 35B A3B<") {
		t.Errorf("household: offers the way back, or the household as a choice")
	}
	f.Hold("homes-ben", "gemma-4-12b", time.Date(2026, 11, 17, 21, 30, 0, 0, time.UTC))
	page = getModell(cfg)
	for _, want := range []string{"Gemma 4 12B · für ben", `class="k warn">Sprachsteuerung pausiert bis 21:30`, "Zurück zum Haushalt", `value="haushalt"`, "Gemma 4 E4B"} {
		if !strings.Contains(page, want) {
			t.Errorf("ben's: lacks %q", want)
		}
	}
	f.Down = true
	page = getModell(cfg)
	if !strings.Contains(page, "Das Sprachmodell antwortet gerade nicht.") || strings.Contains(page, "<form method=post") {
		t.Errorf("down: %s", page)
	}
}

// Who may switch is who may switch a lamp (Schalten): a signed-in person,
// any of them, from Homes' own page. Without a person, or from another
// site, it is refused and the lease hears nothing.
func TestSprachmodellIsALampsRight(t *testing.T) {
	cfg, f := modellSeite(t)
	h := Sprachmodell(cfg)
	form := url.Values{"tun": {"gemma-4-12b@1"}}
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"no person":  post(t, h, SprachmodellPath, form, "", nil),
		"cross-site": post(t, h, SprachmodellPath, form, "anna", map[string]string{"Sec-Fetch-Site": "cross-site"}),
		"no origin":  post(t, h, SprachmodellPath, form, "anna", map[string]string{"Sec-Fetch-Site": ""}),
	} {
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", name, rec.Code)
		}
	}
	if n := takes(f); n != 0 {
		t.Fatalf("a refused switch reached the lease %d times", n)
	}
	// a person who is no admin switches, as they switch a lamp; it is theirs
	if rec := post(t, h, SprachmodellPath, form, "ben", map[string]string{"Remote-Groups": "family"}); rec.Code != http.StatusSeeOther {
		t.Fatalf("ben: %d %s", rec.Code, rec.Body)
	}
	if l := f.Lease(); l == nil || l.Holder != "homes-ben" || l.Alias != "gemma-4-12b" {
		t.Fatalf("ben's switch: %+v", l)
	}
	if rec := post(t, h, SprachmodellPath, url.Values{"tun": {"haushalt"}}, "anna", nil); rec.Code != http.StatusSeeOther || f.Lease() != nil {
		t.Errorf("Zurück zum Haushalt: %d, %+v", rec.Code, f.Lease())
	}
	// what the card does not offer, said on the page
	if rec := post(t, h, SprachmodellPath, url.Values{"tun": {"gemma-4-12b@3"}}, "anna", map[string]string{"X-Homes-Skript": "1"}); rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), "nicht umschalten") {
		t.Errorf("3 h: %d %s", rec.Code, rec.Body)
	}
}

// The model key is never in a page, a refusal or a log line, whatever
// state the lease is in.
func TestSprachmodellKeyStaysOut(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	cfg, f := modellSeite(t)
	h := Sprachmodell(cfg)
	var out []string
	out = append(out, getModell(cfg))
	out = append(out, post(t, h, SprachmodellPath, url.Values{"tun": {"gemma-4-12b@1"}}, "anna", nil).Body.String())
	out = append(out, getModell(cfg))
	f.Hold("bote", "qwen3.8-27b", time.Date(2026, 11, 17, 22, 0, 0, 0, time.UTC))
	out = append(out, getModell(cfg), post(t, h, SprachmodellPath, url.Values{"tun": {"gemma-4-12b@1"}}, "anna", map[string]string{"X-Homes-Skript": "1"}).Body.String())
	f.Down = true
	out = append(out, getModell(cfg), post(t, h, SprachmodellPath, url.Values{"tun": {"haushalt"}}, "anna", nil).Body.String())
	f.Close() // and not there at all
	out = append(out, getModell(cfg), post(t, h, SprachmodellPath, url.Values{"tun": {"gemma-4-12b@4"}}, "anna", nil).Body.String())
	if logs.Len() == 0 {
		t.Fatal("nothing logged: the probe would not see a key there")
	}
	sawKey := false
	for _, r := range f.Seen() {
		sawKey = sawKey || r.Auth == "Bearer "+modellKey
	}
	if !sawKey {
		t.Fatal("the lease never got the key: the probe proves nothing")
	}
	for i, s := range append(out, logs.String()) {
		if strings.Contains(s, modellKey) || strings.Contains(s, "geheim") {
			t.Errorf("output %d carries the key:\n%s", i, s)
		}
	}
}

package napi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/modellease"
	"github.com/mdopp/homes/internal/modellease/leasefake"
	"github.com/mdopp/homes/internal/sprachmodell"
)

const modellKey = "modell-key-geheim-0815"

func modellHouse(t *testing.T) (http.Handler, *leasefake.Server) {
	t.Helper()
	at := time.Date(2026, 11, 17, 20, 0, 0, 0, time.UTC)
	f := leasefake.New(modellKey)
	f.Now = func() time.Time { return at }
	t.Cleanup(f.Close)
	people := map[string]string{"anna1": "anna", "ben1": "ben"}
	return Handler(Config{Validate: StaticValidator([]string{"anna1", "ben1"}), Now: f.Now,
		Whoami:       func(b string) (string, bool) { p, ok := people[b]; return p, ok },
		Sprachmodell: &sprachmodell.Schalter{Lease: &modellease.Client{Base: f.Base(), Key: modellKey}, Now: f.Now}}), f
}

func sendModell(h http.Handler, method, bearer, body string) (*httptest.ResponseRecorder, map[string]any) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/napi/sprachmodell", rd)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

// The app's card: the page's words, switched as the paired phone's person
// — the right /napi/ha/call has. No phone, no switch, and the lease hears
// nothing; the key is in no answer and no log line.
func TestSprachmodellForTheApp(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	h, f := modellHouse(t)
	var bodies []string
	rec, out := sendModell(h, "GET", "anna1", "")
	karte, _ := out["karte"].(map[string]any)
	if rec.Code != 200 || karte["zustand"] != "haushalt" || karte["zeile"] != "Haushalt · Qwen3.6 35B A3B" || len(karte["wahl"].([]any)) != 3 {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body)
	}
	bodies = append(bodies, rec.Body.String())
	for _, bearer := range []string{"", "fremd"} {
		if rec, _ := sendModell(h, "POST", bearer, `{"preset":"gemma-4-12b","stunden":1}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("bearer %q: %d, want 401", bearer, rec.Code)
		}
	}
	for _, r := range f.Seen() {
		if r.Method != http.MethodGet {
			t.Fatalf("a refused switch reached the lease: %s %s", r.Method, r.Body)
		}
	}
	rec, out = sendModell(h, "POST", "ben1", `{"preset":"gemma-4-12b","stunden":4}`)
	karte, _ = out["karte"].(map[string]any)
	if rec.Code != 200 || karte["zeile"] != "Gemma 4 12B · für ben" || karte["kosten"] != "Sprachsteuerung pausiert bis 00:00" || karte["zurueck"] != "Zurück zum Haushalt" {
		t.Fatalf("ben switches: %d %s", rec.Code, rec.Body)
	}
	if l := f.Lease(); l == nil || l.Holder != "homes-ben" {
		t.Fatalf("lease: %+v", l)
	}
	bodies = append(bodies, rec.Body.String())
	rec, out = sendModell(h, "POST", "anna1", `{"preset":"gemma-4-12b","stunden":2}`)
	if rec.Code != http.StatusBadRequest || out["ok"] != false || !strings.Contains(out["error"].(string), "nicht umschalten") {
		t.Errorf("2 h: %d %s", rec.Code, rec.Body)
	}
	if rec, _ = sendModell(h, "POST", "anna1", `{"zurueck":true}`); rec.Code != 200 || f.Lease() != nil {
		t.Errorf("zurück: %d %s %+v", rec.Code, rec.Body, f.Lease())
	}
	f.Hold("bote", "qwen3.8-27b", time.Date(2026, 11, 17, 21, 0, 0, 0, time.UTC))
	rec, out = sendModell(h, "POST", "anna1", `{"preset":"gemma-4-12b","stunden":1}`)
	if rec.Code != http.StatusConflict || !strings.Contains(out["error"].(string), "bote") {
		t.Errorf("over bote's: %d %s", rec.Code, rec.Body)
	}
	bodies = append(bodies, rec.Body.String())
	f.Down = true
	rec, out = sendModell(h, "POST", "anna1", `{"zurueck":true}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(out["error"].(string), "antwortet gerade nicht") {
		t.Errorf("down: %d %s", rec.Code, rec.Body)
	}
	rec2, _ := sendModell(h, "GET", "anna1", "")
	bodies = append(bodies, rec.Body.String(), rec2.Body.String())
	for i, s := range append(bodies, logs.String()) {
		if strings.Contains(s, modellKey) || strings.Contains(s, "geheim") {
			t.Errorf("output %d carries the key: %s", i, s)
		}
	}
	if logs.Len() == 0 {
		t.Error("nothing logged: the probe would not see a key there")
	}
}

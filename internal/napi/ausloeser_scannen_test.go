package napi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mdopp/homes/internal/ha/hafake"
	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/tagaktion"
)

// ADR 0004 §4: a tag at the scanner starts a stack, for whoever touched it
// or for the household, and reports nothing to HA.

type scanLaib struct {
	mu     sync.Mutex
	orders []map[string]any
	answer string // "": a started stack; else the tool's error text
}

func (s *scanLaib) serve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Params struct {
			Name string         `json:"name"`
			Args map[string]any `json:"arguments"`
		} `json:"params"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	defer s.mu.Unlock()
	text, isErr := `{"message":"Stapel angenommen"}`, false
	if req.Params.Name != "laib_scan" {
		text, isErr = "unknown tool", true
	} else if s.answer != "" {
		text, isErr = s.answer, true
	} else {
		s.orders = append(s.orders, req.Params.Args)
	}
	json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": isErr}})
}

func scanTags(t *testing.T) (*hafake.HA, *scanLaib, *tagaktion.Store, http.Handler) {
	t.Helper()
	sl := &scanLaib{}
	srv := httptest.NewServer(http.HandlerFunc(sl.serve))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	store, err := tagaktion.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	fake, h := houseWith(t, func(c *Config) {
		c.Laib = &laib.Client{URL: srv.URL, Token: "ops"}
		c.Tags = &tagaktion.Dienst{HA: c.HA, Laib: c.Laib, Store: store, Dir: dir, PublicURL: "https://www.example.org"}
		c.Whoami = func(b string) (string, bool) {
			p, ok := map[string]string{"dev1": "anna", "dev2": "ben", "dev3": "Kein Login!"}[b]
			return p, ok
		}
		c.Validate = StaticValidator([]string{"dev1", "dev2", "dev3"})
	})
	fake.AddTag("homes-5ca11e000001", "Scanner")
	return fake, sl, store, h
}

func TestATagOffersTheScanAndKeepsWhatItDoes(t *testing.T) {
	_, _, store, h := scanTags(t)
	code, body := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"Scanner","tag":"homes-5ca11e000001","aktion":"scanner-ich"}`)
	if code != http.StatusOK || reply(t, body)["aktion"] != "scanner-ich" || store.Von("homes-5ca11e000001") != "scanner-ich" {
		t.Fatalf("%d %s", code, body)
	}
	_, body = raw(t, h, "GET", "/napi/ausloeser/tags", "dev2", "")
	if !strings.Contains(body, `"aktion":"scanner-ich","automationen":[],"name":"Scanner"`) ||
		!strings.Contains(body, `"aktionen":[{"id":"","titel":"Home Assistant melden"},{"id":"scanner-fragen","titel":"Scanner – fragen, für wen"},`+
			`{"id":"scanner-ich","titel":"Scanner – direkt für mich"},{"id":"scanner-haushalt","titel":"Scanner – direkt für den Haushalt"}]`) {
		t.Fatalf("tags: %s", body)
	}
	// a rename without "aktion" keeps it; "" takes it away
	raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"Scanner Flur","tag":"homes-5ca11e000001"}`)
	if store.Von("homes-5ca11e000001") != "scanner-ich" {
		t.Fatal("a rename dropped the action")
	}
	raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"Scanner Flur","tag":"homes-5ca11e000001","aktion":""}`)
	if store.Von("homes-5ca11e000001") != "" {
		t.Fatal("the action stayed")
	}
	if code, body := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"X","aktion":"licht-an"}`); code != http.StatusBadRequest || !strings.Contains(body, "nicht tun") {
		t.Fatalf("an unknown action: %d %s", code, body)
	}
}

// „Für mich“ is the toucher's: anna's stack for anna, ben's for ben, the
// same tag; never the household's, never another person's.
func TestScanForMeIsTheTouchersOwn(t *testing.T) {
	fake, sl, store, h := scanTags(t)
	store.Setze("homes-5ca11e000001", tagaktion.ScannerIch)
	for _, who := range []struct{ dev, person string }{{"dev1", "anna"}, {"dev2", "ben"}} {
		code, body := raw(t, h, "POST", "/napi/ausloeser", who.dev, `{"art":"nfc","tag":"homes-5ca11e000001"}`)
		m := reply(t, body)
		if code != http.StatusOK || m["text"] != "Der Scanner startet – nur für dich." || m["aktion"] != "scanner-ich" || m["name"] != "Scanner" {
			t.Fatalf("%s: %d %s", who.person, code, body)
		}
	}
	if len(sl.orders) != 2 {
		t.Fatalf("orders: %v", sl.orders)
	}
	for i, p := range []string{"anna", "ben"} {
		o := sl.orders[i]
		if o["action"] != "start" || o["person"] != p || o[laib.ScanFuerFeld] != "person" {
			t.Errorf("order %d: %v", i, o)
		}
	}
	if len(fake.Events()) != 0 {
		t.Fatalf("a scan tag reported to HA: %v", fake.Events())
	}
}

func TestScanForTheHouseholdSendsNoFuer(t *testing.T) {
	_, sl, store, h := scanTags(t)
	store.Setze("homes-5ca11e000001", tagaktion.ScannerHaushalt)
	code, body := raw(t, h, "POST", "/napi/ausloeser", "dev2", `{"art":"nfc","tag":"homes-5ca11e000001"}`)
	if code != http.StatusOK || reply(t, body)["text"] != "Der Scanner startet – für den Haushalt." {
		t.Fatalf("%d %s", code, body)
	}
	if o := sl.orders[0]; o["person"] != "ben" || o[laib.ScanFuerFeld] != nil {
		t.Fatalf("order: %v", o)
	}
}

func TestScanForMeWantsALogin(t *testing.T) {
	_, sl, store, h := scanTags(t)
	store.Setze("homes-5ca11e000001", tagaktion.ScannerIch)
	code, body := raw(t, h, "POST", "/napi/ausloeser", "dev3", `{"art":"nfc","tag":"homes-5ca11e000001"}`)
	if code != http.StatusForbidden || !strings.Contains(reply(t, body)["error"].(string), "Login") || len(sl.orders) != 0 {
		t.Fatalf("%d %s %v", code, body, sl.orders)
	}
}

func TestScanTagWhenTheScannerIsMissingOrSilent(t *testing.T) {
	_, sl, store, h := scanTags(t)
	store.Setze("homes-5ca11e000001", tagaktion.ScannerHaushalt)
	sl.answer = "scan: nicht eingerichtet"
	code, body := raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"nfc","tag":"homes-5ca11e000001"}`)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "kein Scanner eingerichtet") {
		t.Fatalf("missing: %d %s", code, body)
	}
	sl.answer = "scan: boom"
	code, body = raw(t, h, "POST", "/napi/ausloeser", "dev1", `{"art":"nfc","tag":"homes-5ca11e000001"}`)
	if code != http.StatusBadGateway || !strings.Contains(body, "antwortet gerade nicht") || strings.Contains(body, "boom") {
		t.Fatalf("silent: %d %s", code, body)
	}
}

func TestRemovingATagTakesItsAction(t *testing.T) {
	_, _, store, h := scanTags(t)
	store.Setze("homes-5ca11e000001", tagaktion.ScannerIch)
	if code, _ := raw(t, h, "DELETE", "/napi/ausloeser/tag/homes-5ca11e000001", "dev1", ""); code != http.StatusOK || store.Von("homes-5ca11e000001") != "" {
		t.Fatalf("%d, action %q", code, store.Von("homes-5ca11e000001"))
	}
}

// Without the store (or Laib), only HA's is offered and an action is refused.
func TestNoScanActionWithoutTheStore(t *testing.T) {
	_, h := triggers(t)
	_, body := raw(t, h, "GET", "/napi/ausloeser/tags", "dev1", "")
	if !strings.Contains(body, `"aktionen":[{"id":"","titel":"Home Assistant melden"}]`) {
		t.Fatalf("%s", body)
	}
	if code, _ := raw(t, h, "POST", "/napi/ausloeser/tag", "dev1", `{"name":"X","aktion":"scannen-ich"}`); code != http.StatusBadRequest {
		t.Fatalf("refused: %d", code)
	}
}

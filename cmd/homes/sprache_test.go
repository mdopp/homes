package main

import (
	"encoding/json"
	"net/http/httptest"
	"slices"
	"testing"
)

// TestNapiSpeaksThePhonesLanguage (owner 2026-11-18): the app sends the
// phone's language as Accept-Language and /napi/kann answers in it — the
// titles and the traces —, German for one Homes does not speak; every
// answer says it varies by that header.
func TestNapiSpeaksThePhonesLanguage(t *testing.T) {
	h := routes(kannSite(t))
	kann := func(accept string) (titel, spur string) {
		t.Helper()
		req := httptest.NewRequest("GET", "/napi/kann", nil)
		req.Header.Set("Authorization", "Bearer dev1")
		if accept != "" {
			req.Header.Set("Accept-Language", accept)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || !slices.Contains(rec.Header().Values("Vary"), "Accept-Language") {
			t.Fatalf("%q → %d, Vary %q", accept, rec.Code, rec.Header().Values("Vary"))
		}
		var k struct {
			Module []struct{ Name, Titel, Spur string }
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &k); err != nil {
			t.Fatal(err)
		}
		for _, m := range k.Module {
			if m.Name == "geraete" {
				titel = m.Titel
			}
			if m.Name == "sprechen" {
				spur = m.Spur
			}
		}
		return titel, spur
	}
	for accept, want := range map[string][2]string{
		"":                     {"Geräte", "auf dieser Box nicht eingerichtet"},
		"fr":                   {"Appareils", "pas installé sur cette box"},
		"en-US,en;q=0.9":       {"Devices", "not set up on this box"},
		"ja":                   {"Geräte", "auf dieser Box nicht eingerichtet"},
		"de;q=0.5,es;q=0.9,en": {"Devices", "not set up on this box"},
	} {
		if titel, spur := kann(accept); titel != want[0] || spur != want[1] {
			t.Errorf("Accept-Language %q: %q, %q, want %q", accept, titel, spur, want)
		}
	}
	// a page, and a refusal, say so too: the browser keeps one copy per language
	for _, path := range []string{"/start", "/napi/kann"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if !slices.Contains(rec.Header().Values("Vary"), "Accept-Language") {
			t.Errorf("%s → %d without Vary: Accept-Language", path, rec.Code)
		}
	}
}

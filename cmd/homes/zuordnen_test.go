package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/pages"
)

// laib DATA.md gap 6: Dokumente posts what it could not give anyone and takes the admins' answers back,
// from the box with the Haushalt bearer, never through Caddy; an answer goes back only for the scan as shown.
func TestZuordnenBetweenDokumenteAndTheAdmins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zuordnen.json")
	z := openZuordnen(path, "hh")
	post := func(body string, hdr map[string]string) (int, map[string][]antwort) {
		r := httptest.NewRequest("POST", zuordnenPfad, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer hh")
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		z.ServeHTTP(rec, r)
		var out map[string][]antwort
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	liste := `{"offen":[{"id":7,"zeichen":"z7","absender":"Praxis","datum":"2026-11-03","seiten":2,"seit":"10.11."}],"personen":[{"login":"anna","name":"Anna"}]}`
	if code, _ := post(liste, map[string]string{"Authorization": "Bearer nein"}); code != http.StatusUnauthorized {
		t.Fatalf("a wrong bearer: %d", code)
	}
	if code, _ := post(liste, map[string]string{"X-Homes-Proxy": "geheim"}); code != http.StatusForbidden {
		t.Fatalf("through Caddy: %d", code)
	}
	if code, out := post(liste, nil); code != 200 || len(out["antworten"]) != 0 {
		t.Fatalf("%d %v", code, out)
	}
	if z.Antworte(7, "anders", "anna", "mdopp") == nil || z.Antworte(7, "z7", "fremd", "mdopp") == nil {
		t.Fatal("an answer for what was not shown, or to someone not listed, is refused")
	}
	if err := z.Antworte(7, "z7", "anna", "mdopp"); err != nil {
		t.Fatal(err)
	}
	offen, _ := openZuordnen(path, "hh").Stand() // kept on disk
	if len(offen) != 1 || offen[0].An != "anna" {
		t.Fatalf("%+v", offen)
	}
	if _, out := post(liste, nil); len(out["antworten"]) != 1 || out["antworten"][0].An != "anna" || out["antworten"][0].Zeichen != "z7" {
		t.Fatalf("the answer back: %v", out)
	}
	// Dokumente gave it: the scan is no longer posted, and its answer goes
	if _, out := post(`{"offen":[],"personen":[]}`, nil); len(out["antworten"]) != 0 {
		t.Fatalf("%v", out)
	}
	var _ pages.Zuordnen = z
}

package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/notes"
)

// owner 2026-11-27: the app's „Öffnen“ lists the Rezepte after Notizen,
// with how many there are; the page is Homes' /rezepte.
func TestTheAppOpensTheRecipes(t *testing.T) {
	s := kannSite(t)
	s.publicURL = "https://homes.example"
	s.hh.Notes.CreateWith("Kürbissuppe", "1 Hokkaido", notes.Kopf{Art: "rezept"})
	s.hh.Notes.CreateWith("Einkauf", "Milch", notes.Kopf{})
	s.napi.Einrichtung = einrichtung(s, nil, nil)
	s.napi.Whoami = func(b string) (string, bool) { return "anna", b == "dev1" }
	req := httptest.NewRequest("GET", "/napi/einrichtung", nil)
	req.Header.Set("Authorization", "Bearer dev1")
	rec := httptest.NewRecorder()
	routes(s).ServeHTTP(rec, req)
	var a einrichten.Antwort
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil || rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	for i, m := range a.Module {
		if m.Name == "rezepte" {
			if i == 0 || a.Module[i-1].Name != "notizen" || !m.Oeffnen || m.Anzahl != 1 || m.Stand != "1 Rezept" || m.Web != "https://homes.example/rezepte" || !m.NurWeb {
				t.Fatalf("%+v (after %s)", m, a.Module[i-1].Name)
			}
			return
		}
	}
	t.Fatalf("no rezepte: %s", rec.Body)
}

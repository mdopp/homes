package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/notes"
	"github.com/mdopp/homes/internal/paperless"
	"github.com/mdopp/homes/internal/paperless/paperlessfake"
	"github.com/mdopp/homes/internal/plugin"
	"github.com/mdopp/homes/internal/tagaktion"
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

// homes-android#290: the start screen's areas say where they stand — the
// tags and their last run, what Paperless filed this week — and what is new.
func TestTheStartScreensAreas(t *testing.T) {
	s, fake, _ := fullSite(t)
	docs := paperlessfake.New("pl")
	t.Cleanup(docs.Close)
	docs.Add(paperlessfake.Doc{ID: 1, Title: "Neu", Added: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)})
	docs.Add(paperlessfake.Doc{ID: 2, Title: "Alt", Added: time.Now().AddDate(0, -1, 0).UTC().Format(time.RFC3339)})
	s.hh.Documents = &paperless.Client{URL: docs.URL, Token: "pl"}
	fake.AddTag("homes-b0e70e000001", "Bürotür")
	s.napi.Tags = &tagaktion.Dienst{HA: s.napi.HA}
	got := bereiche(s)(t.Context(), "anna")
	if got["dokumente"] != "1 diese Woche" || got["ausloeser"] != "1 Tag" || got["archiv"] != "" {
		t.Fatalf("%v", got)
	}
	// the person's archive says its own line; another person has none
	arch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"vertrag":1,"name":"Dein Archiv","link":"https://archiv.example","zeile":"2 Fragen offen"}`))
	}))
	t.Cleanup(arch.Close)
	s.plugins = plugin.Open([]plugin.Instance{{Plugin: "archiv", Person: "anna", URL: arch.URL, Key: "k"}}, "")
	if got := bereiche(s)(t.Context(), "anna"); got["archiv"] != "2 Fragen offen" {
		t.Fatalf("archiv: %v", got)
	}
	if got := bereiche(s)(t.Context(), "ben"); got["archiv"] != "" {
		t.Fatalf("ben: %v", got)
	}
	docs.Status = 500
	if got := bereiche(s)(t.Context(), "anna"); got["dokumente"] != "" {
		t.Fatalf("Paperless silent: %v", got)
	}
}

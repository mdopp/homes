package main

import (
	"encoding/json"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/mdopp/homes/internal/einrichten"
)

// owner 2026-11-28: no Energie widget any more; the app's tile opens the
// energy page, whose address /napi/einrichtung gives, and reads its line
// from /napi/portal/energy, which stays.
func TestEnergieIsATileNotAWidget(t *testing.T) {
	s := kannSite(t)
	s.publicURL = "https://homes.example"
	s.napi.Einrichtung = einrichtung(s, nil, nil)
	s.napi.Whoami = func(b string) (string, bool) { return "anna", b == "dev1" }
	h := routes(s)
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer dev1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	var k struct {
		App struct{ Widgets []string } `json:"app"`
	}
	json.Unmarshal(get("/napi/kann").Body.Bytes(), &k)
	if slices.Contains(k.App.Widgets, "energie") || !slices.Contains(k.App.Widgets, "geraet") {
		t.Fatalf("widgets: %v", k.App.Widgets)
	}
	var a einrichten.Antwort
	json.Unmarshal(get("/napi/einrichtung").Body.Bytes(), &a)
	i := slices.IndexFunc(a.Module, func(m einrichten.NapiModul) bool { return m.Name == "energie" })
	if i < 0 || a.Module[i].Web != "https://homes.example/energie" || !a.Module[i].NurWeb {
		t.Fatalf("energie in /napi/einrichtung: %+v", a.Module)
	}
	if rec := get("/napi/portal/energy"); rec.Code == 404 {
		t.Fatalf("/napi/portal/energy is gone: %s", rec.Body)
	}
}

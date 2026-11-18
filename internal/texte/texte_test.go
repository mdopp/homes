package texte

import (
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
)

// TestEveryIDHasGerman: each id the package declares is in German.
func TestEveryIDHasGerman(t *testing.T) {
	src, err := os.ReadFile("katalog.go")
	if err != nil {
		t.Fatal(err)
	}
	ids := regexp.MustCompile(`ID = "([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(ids) < 10 {
		t.Fatalf("found %d ids", len(ids))
	}
	for _, m := range ids {
		if de[ID(m[1])] == "" {
			t.Errorf("%s has no German", m[1])
		}
	}
	if Titel("standort") != "Standortverlauf" || In("fr", SpurenTitel) != "Gibt es hier noch nicht" || T("gibts.nicht") != "gibts.nicht" {
		t.Errorf("Titel %q, fr falls back %q, unknown %q", Titel("standort"), In("fr", SpurenTitel), T("gibts.nicht"))
	}
}

// TestEachTranslationIsWhole: a translated word has German and all three
// other languages, and In finds them; a language Homes lacks is German.
func TestEachTranslationIsWhole(t *testing.T) {
	for id, by := range uebersetzt {
		if de[id] == "" || len(by) != 3 || by["en"] == "" || by["es"] == "" || by["fr"] == "" {
			t.Errorf("%s: German %q, %v", id, de[id], by)
		}
	}
	if In("en", NavAbmelden) != "Sign out" || In("fr", NavAbmelden) != "Se déconnecter" || In("it", NavAbmelden) != "Abmelden" {
		t.Errorf("In: %q %q %q", In("en", NavAbmelden), In("fr", NavAbmelden), In("it", NavAbmelden))
	}
}

// TestWaehle (owner 2026-11-18): the client's Accept-Language picks the
// language, by its weights; one Homes does not speak is German.
func TestWaehle(t *testing.T) {
	for accept, want := range map[string]Sprache{
		"en-US,en;q=0.9":            "en",
		"fr":                        "fr",
		"fr-CA":                     "fr",
		"ja":                        "de",
		"":                          "de",
		"*":                         "de",
		"ja,fr;q=0.5,en;q=0.8":      "en", // the highest weight, not the first named
		"de;q=0.2, es":              "es", // no q is 1
		"es;q=0.7,fr;q=0.7":         "es", // equal weights: the first
		"en;q=0,fr;q=0.1":           "fr", // q=0: not wanted at all
		"en;q=0":                    "de",
		"EN-gb ; q=0.9 , ja":        "en",
		"en;q=x,fr;q=0.3":           "fr", // a weight that is none: that entry is left out
		"de-DE,de;q=0.9,en;q=0.8":   "de",
		"pt-BR,es-419;q=0.9,en;q=1": "en",
	} {
		if got := Waehle(accept); got != want {
			t.Errorf("Waehle(%q) = %q, want %q", accept, got, want)
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Language", "es-ES,es;q=0.9")
	if Von(r) != "es" || Von(r).T(NavMehr) != "Más" || Von(r).Titel("kalender") != "Calendario" || Sprache("es").T(SpurenTitel) != de[SpurenTitel] {
		t.Errorf("Von %q: %q %q", Von(r), Von(r).T(NavMehr), Von(r).Titel("kalender"))
	}
}

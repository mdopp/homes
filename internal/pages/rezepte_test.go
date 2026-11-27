package pages

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/einkauf"
	"github.com/mdopp/homes/internal/notes"
)

// owner 2026-11-27: the household's recipes, newest first, searched, each
// its note's page, the way into Einkauf's choice. The recipes are the
// household's: everyone sees the same list (one viewer per kind below).

func rezeptNotes(t *testing.T) *notes.Dir {
	t.Helper()
	dir := t.TempDir()
	d, err := notes.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range []struct{ title, text string }{{"Lasagne", "Nudelplatten, Hack"}, {"Kürbissuppe", "1 Hokkaido, Kokosmilch"}} {
		name, err := d.CreateWith(r.title, r.text, notes.Kopf{Art: "rezept"})
		if err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-time.Duration(2-i) * time.Hour) // Kürbissuppe the newest
		os.Chtimes(filepath.Join(dir, name), at, at)
	}
	d.CreateWith("Hokkaido kaufen", "nur eine Notiz", notes.Kopf{})
	return d
}

func rezepteGet(t *testing.T, cfg StartConfig, target, user, groups string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	req.Header.Set("Remote-User", user)
	req.Header.Set("Remote-Groups", groups)
	rec := httptest.NewRecorder()
	Rezepte(cfg).ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestRezepteListsTheHouseholdsRecipes(t *testing.T) {
	cfg := StartConfig{Notes: rezeptNotes(t), Einkauf: &einkauf.Liste{}}
	for user, groups := range map[string]string{"michael": "admins", "anna": "family", "petra": "admins,family"} {
		code, body := rezepteGet(t, cfg, RezeptePath, user, groups)
		if code != 200 {
			t.Fatalf("%s: %d", user, code)
		}
		mustContain(t, body, "<h1>Rezepte</h1>", ">Kürbissuppe</a>", ">Lasagne</a>", `href="/notizen/`, `data-laeuft="Sucht …">Suchen</button>`,
			`<form method=get action=/einkauf/rezepte><button data-laeuft="Öffnet …">Für den Einkauf auswählen</button></form>`)
		mustNotContain(t, body, "Hokkaido kaufen")
		before(t, body, "Kürbissuppe", "Lasagne")
	}
}

func TestRezepteSearchEmptyAndFailing(t *testing.T) {
	cfg := StartConfig{Notes: rezeptNotes(t)}
	_, body := rezepteGet(t, cfg, RezeptePath+"?q=hokkaido", "anna", "family")
	mustContain(t, body, ">Kürbissuppe</a>", `value="hokkaido"`)
	mustNotContain(t, body, ">Lasagne</a>", "Hokkaido kaufen", "geändert 01.01.0001", "Für den Einkauf") // no Einkauf on this box
	_, body = rezepteGet(t, cfg, RezeptePath+"?q=pizza", "anna", "family")
	mustContain(t, body, "Kein Rezept zu „pizza“.", `<a href=/rezepte>Alle Rezepte</a>`)
	empty, _ := notes.Open(t.TempDir())
	_, body = rezepteGet(t, StartConfig{Notes: empty}, RezeptePath, "anna", "family")
	mustContain(t, body, "Noch kein Rezept. Teile eine Rezeptseite mit der Homes-App oder schick den Link an Homes.")
	gone := t.TempDir()
	d, _ := notes.Open(gone)
	os.RemoveAll(gone)
	_, body = rezepteGet(t, StartConfig{Notes: d}, RezeptePath, "anna", "family")
	mustContain(t, body, "Die Rezepte lassen sich gerade nicht lesen.")
	_, body = rezepteGet(t, StartConfig{}, RezeptePath, "anna", "family")
	mustContain(t, body, "noch nicht eingerichtet")
}

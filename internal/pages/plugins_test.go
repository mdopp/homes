package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/plugin"
)

// A plugin's entry (homes ADR 0003): its name, icon and link under Mehr and
// in the wide window's list, for the person whose instance it is, once it
// answered; nobody else, and nobody before it answered.
func TestPluginEintragOnlyForItsPerson(t *testing.T) {
	answer := fragen362
	anna, _ := archiv362(t, false, &answer)
	cfg := StartConfig{Plugins: plugin.Open([]plugin.Instance{anna}, "")}
	for _, c := range []struct {
		person string
		sees   bool
	}{{"anna", false}, {"ben", false}} { // not answered yet: no entry
		_, body, _ := familyPageAs(t, Mehr, cfg, "GET", "/mehr", c.person, "family")
		mustNotContain(t, body, "archiv.box.test")
	}
	cfg.Plugins.Lesen(context.Background(), "anna")
	_, body, _ := familyPageAs(t, Mehr, cfg, "GET", "/mehr", "anna", "family")
	mustContain(t, body, `href="https://archiv.box.test/tresor"`, ">Archiv<")
	_, body, _ = familyPageAs(t, Start, cfg, "GET", "/start", "anna", "family")
	mustContain(t, body, "href=https://archiv.box.test/tresor>")
	_, body, _ = familyPageAs(t, Mehr, cfg, "GET", "/mehr", "ben", "family")
	mustNotContain(t, body, "archiv.box.test", ">Archiv<")
}

// The probe (homes#375 acceptance): a box without the archive registered
// shows no trace of it — not on Start, Braucht dich, Deine Aufgaben, Mehr
// or in the app — even with what Homes kept of it from before, for its
// own person. The same pages with the archive registered do show it, so
// the probe can fail.
func TestOhneArchivKeineSpur(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plugins.json")
	answer := fragen362
	anna, _ := archiv362(t, false, &answer)
	pages := func(cfg StartConfig) string {
		var all strings.Builder
		for _, p := range []struct {
			h    func(StartConfig) http.HandlerFunc
			path string
		}{{Start, "/start"}, {Start, StartBoxPath}, {DeineAufgaben, aufgabenPath}, {Mehr, mehrPath}, {Mehr, MehrAufgabenPath}} {
			_, body, _ := familyPageAs(t, p.h, cfg, "GET", p.path, "anna", "family,admins")
			all.WriteString(body)
		}
		cfg.Einrichtung = &einrichten.Quellen{Module: modulListe(t), Admins: einrichten.OpenAdmins(t.TempDir())}
		for _, z := range AppListe(cfg)(context.Background(), "anna") {
			all.WriteString(z.Name + " " + z.Titel + " " + z.Link + "\n")
		}
		return all.String()
	}
	spuren := []string{"Archiv", "archiv", "/tresor", "3 Fragen offen"}

	mit := StartConfig{Laib: laib362(t, true), Plugins: plugin.Open([]plugin.Instance{anna}, file)}
	mit.Plugins.Lesen(context.Background(), "anna")
	if got := pages(mit); !strings.Contains(got, "Im Archiv: 3 Fragen offen") || !strings.Contains(got, "archiv.box.test") {
		t.Fatalf("with the archive registered its tasks and entry are not there: the probe could not fail")
	}
	if b, err := os.ReadFile(file); err != nil || !strings.Contains(string(b), "3 Fragen offen") {
		t.Fatalf("Homes kept nothing of the archive: %v %s", err, b)
	}

	ohne := StartConfig{Laib: laib362(t, true), Plugins: plugin.Open(plugin.FromEnv(nil), file)} // laib remove archiv-anna: no HOMES_PLUGIN_ARCHIV
	mustNotContain(t, pages(ohne), spuren...)
	if b, _ := os.ReadFile(file); strings.Contains(string(b), "Fragen") {
		t.Errorf("the archive's kept tasks stayed in Homes after it was removed: %s", b)
	}
	mustNotContain(t, pages(StartConfig{Laib: laib362(t, true)}), spuren...) // and a Homes that never had one
}

// A plugin that answers outside the contract, slowly or not at all is a
// quiet line once it was there, and Homes' pages come all the same.
func TestPluginStillNieKaputt(t *testing.T) {
	answer := fragen362
	anna, _ := archiv362(t, false, &answer)
	cfg := StartConfig{Plugins: plugin.Open([]plugin.Instance{anna}, "")}
	cfg.Plugins.Lesen(context.Background(), "anna")
	for _, a := range []string{`{"vertrag":2,"name":"Archiv","link":"https://archiv.box.test"}`, `{"vertrag":1,"name":"Archiv","link":"javascript:alert(1)"}`, "<html>"} {
		answer = a
		code, body, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "anna", "family")
		if code != http.StatusOK {
			t.Fatalf("%s: %d", a, code)
		}
		mustContain(t, body, "<p class=k>Archiv antwortet gerade nicht.</p>", "Im Archiv: 3 Fragen offen")
		mustNotContain(t, body, "javascript:")
	}
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	cfg.Plugins = plugin.Open([]plugin.Instance{{Plugin: "archiv", Person: "anna", URL: down.URL}}, "")
	code, body, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "anna", "family")
	if code != http.StatusOK || strings.Contains(body, "Archiv") {
		t.Errorf("a plugin that never answered: %d, a trace of it: %v", code, strings.Contains(body, "Archiv"))
	}
}

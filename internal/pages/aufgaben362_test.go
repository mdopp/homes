package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/plugin"
)

// laib362 is a Laib after laib#1195: the tasks with the keys the box waits
// for, two decisions waiting. old: a Laib before it, no schluessel.
func laib362(t *testing.T, old bool) *laib.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Name string         `json:"name"`
				Args map[string]any `json:"arguments"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		text := "[]"
		switch req.Params.Name {
		case "laib_aufgabe":
			text = `{"offen":6,"faellig":0,"aufgaben":[
 {"id":20,"titel":"Zweite Platte einbauen","warum":"Anleitung: https://laib.box.test/hilfe/platte.","fuer":"admins"},
 {"id":21,"titel":"Post-Passwort eintragen","fuer":"admins","weg":{"schluessel":"WHO_MAIL","link":"https://laib.box.test/schluessel?name=WHO_MAIL"}},
 {"id":22,"titel":"Später erst","fuer":"admins","erinnern":"2099-01-01"},
 {"id":23,"titel":"Für alle","fuer":"alle"},
 {"id":24,"titel":"Bens Sache","fuer":"person:ben","weg":{"seite":"/netzlaufwerk"}},
 {"id":25,"titel":"Discogs-Schlüssel","fuer":"person:ben","weg":{"schluessel":"DISCOGS","link":"https://laib.box.test/schluessel?name=DISCOGS"}}]`
			if !old {
				text += `,"schluessel":[{"name":"WHO_MAIL","dienst":"whoami","label":"Whoami: Post","warum":"Damit whoami Post liest.","link":"https://laib.box.test/schluessel?name=WHO_MAIL","aufgabe":21},
 {"name":"ARCHIV_IMAP","dienst":"archiv-anna","label":"Archiv: Postfach","link":"https://laib.box.test/schluessel?name=ARCHIV_IMAP"}]`
			}
			text += "}"
		case "laib_inbox":
			text = `[{"id":1,"status":"open"},{"id":2,"status":"open"}]`
		}
		json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"content": []map[string]string{{"text": text}}}})
	}))
	t.Cleanup(srv.Close)
	return &laib.Client{URL: srv.URL, Token: "t"}
}

// archiv362 is anna's archive as a plugin (homes ADR 0003): its contract
// at /homes/v1 for her key and her name alone, answer its JSON; old: an
// archive before the contract (404). It counts who asked.
func archiv362(t *testing.T, old bool, answer *string) (plugin.Instance, *int) {
	t.Helper()
	asked := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked++
		if old || r.URL.Path != plugin.Path || r.Header.Get(plugin.Header) != "anna-geheim" || r.URL.Query().Get("person") != "anna" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(*answer))
	}))
	t.Cleanup(srv.Close)
	return plugin.Instance{Plugin: "archiv", Person: "anna", URL: srv.URL, Key: "anna-geheim"}, asked
}

// fragen362 is the archive's answer with its vault open: three questions as one row.
const fragen362 = `{"vertrag":1,"name":"Archiv","icon":"archiv","link":"https://archiv.box.test/tresor","aufgaben":[` +
	`{"id":"fragen","titel":"Im Archiv: 3 Fragen offen","warum":"Wer zu wem gehört: dein Archiv fragt nach.","link":"https://archiv.box.test/tresor"}]}`

// cfg362 is a box where anna's and clara's archives are registered;
// clara's does not answer.
func cfg362(t *testing.T, old bool) StartConfig {
	answer := fragen362
	anna, _ := archiv362(t, old, &answer)
	return StartConfig{Laib: laib362(t, old), LaibURL: "https://laib.box.test", PublicURL: "https://www.box.test",
		Plugins: plugin.Open([]plugin.Instance{anna, {Plugin: "archiv", Person: "clara", URL: "http://127.0.0.1:1", Key: "clara-geheim"}}, "")}
}

// homes#362: Deine Aufgaben is the one list, per viewer, in the owner's
// order — decisions, keys, questions, setup. The owner (an admin, with an
// archive) sees all of it; an admin who is not the owner the box's but not
// anna's archive; another person only theirs and the household's. A key
// leads to its form on Laib's page and back; a task that waits for a key
// is one row with it; Laib's addresses are links; a deferred one waits.
func TestDeineAufgabenPerViewer(t *testing.T) {
	cfg := cfg362(t, false)
	_, owner, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "anna", "family,admins")
	offen := section(t, owner, "karte-offen")
	back := "&amp;zurueck=https%3A%2F%2Fwww.box.test%2Faufgaben"
	mustContain(t, offen, `<a href="https://laib.box.test/inbox">2 Entscheidungen warten</a>`,
		`<a href="https://laib.box.test/schluessel?name=WHO_MAIL`+back+`">Whoami: Post</a>`, "Damit whoami Post liest.", ">Schlüssel eintragen</a>",
		`<a href="https://laib.box.test/schluessel?name=ARCHIV_IMAP`+back+`">Archiv: Postfach</a>`, "Ohne ihn wartet archiv-anna.",
		`<a href="https://archiv.box.test/tresor">Im Archiv: 3 Fragen offen</a>`, ">Archiv öffnen</a>",
		`<li id=aufgabe-20><b>Zweite Platte einbauen</b><span class=text>Anleitung: <a href="https://laib.box.test/hilfe/platte">https://laib.box.test/hilfe/platte</a>.</span>`,
		`<li id=aufgabe-23><b>Für alle</b>`)
	mustNotContain(t, owner, "Post-Passwort eintragen", "Später erst", "Bens Sache", "Discogs-Schlüssel", "aufräumen")
	before(t, offen, "Entscheidungen", "Whoami")
	before(t, offen, "Whoami", "Im Archiv")
	before(t, offen, "Im Archiv", "Zweite Platte")
	mustContain(t, owner, `<a href="/aufgaben?nur=box">Box</a>`, "<h1>Deine Aufgaben <span class=anzahl")

	_, admin, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "clara", "family,admins")
	mustContain(t, admin, "2 Entscheidungen warten", "Whoami: Post", "Zweite Platte einbauen", "Für alle")
	mustNotContain(t, admin, "3 Fragen offen", "Im Archiv", "Bens Sache") // her own archive does not answer: no row, no error

	_, ben, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "ben", "family")
	mustContain(t, section(t, ben, "karte-offen"), `<a href="/netzlaufwerk">Bens Sache</a>`, `Für alle`,
		`<a href="https://laib.box.test/schluessel?name=DISCOGS`+back+`">Discogs-Schlüssel</a>`)
	mustNotContain(t, ben, "Entscheidung", "Whoami", "Archiv: Postfach", "Im Archiv", "Zweite Platte", "?nur=box")
	before(t, ben, "Discogs-Schlüssel", "Bens Sache") // a key before setup

	// Start: the first three and how many more
	_, start, _ := familyPageAs(t, Start, cfg, "GET", StartBoxPath, "anna", "family,admins")
	c := section(t, start, "karte-braucht-dich")
	mustContain(t, c, "2 Entscheidungen warten", "Whoami: Post", "Archiv: Postfach", `<a href="/aufgaben">3 weitere Aufgaben</a>`)
	mustNotContain(t, c, "Zweite Platte")
}

// The Box filter is the admins' Box-Aufgaben with Erledigt and Später; a
// task that waits for a key leads to its form; a person without admins
// gets the list itself.
func TestDeineAufgabenBoxFilter(t *testing.T) {
	cfg := cfg362(t, false)
	_, box, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", boxAufgabenPfad, "anna", "admins")
	mustContain(t, box, `<a href="/aufgaben?nur=box" aria-current=page>Box</a>`, "<li id=aufgabe-20>", "<li id=aufgabe-22>", "Erledigt</button>",
		`<a href="https://laib.box.test/schluessel?name=WHO_MAIL&amp;zurueck=https%3A%2F%2Fwww.box.test%2Faufgaben%3Fnur%3Dbox">Post-Passwort eintragen</a>`,
		`<a href="https://laib.box.test/schluessel?name=ARCHIV_IMAP&amp;zurueck=`)
	_, ben, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", boxAufgabenPfad, "ben", "family")
	mustNotContain(t, ben, "Zweite Platte", "Erledigt</button>")
	mustContain(t, ben, "Bens Sache")
}

// A Laib before laib#1195 (the box's v2.24.0) and an archive before the
// plugin contract: no key rows, no tasks, no quiet line, no error.
func TestDeineAufgabenWithAnOlderLaib(t *testing.T) {
	cfg := cfg362(t, true)
	_, body, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "anna", "family,admins")
	mustContain(t, body, "2 Entscheidungen warten", "Zweite Platte einbauen")
	mustNotContain(t, body, "Archiv: Postfach", "Im Archiv", "Archiv", saysAufgabenNicht, "Whoami", "antwortet gerade nicht")
	_, start, _ := familyPageAs(t, Start, cfg, "GET", StartBoxPath, "anna", "family,admins")
	mustNotContain(t, start, "Archiv: Postfach")
}

// /napi/einrichtung carries the same rows (homes#362): each with its whole
// address, decisions, keys and questions before the modules; an admin's
// rows for whom Homes saw among the admins, another person's own only.
func TestAppListeIsThePageList(t *testing.T) {
	cfg := cfg362(t, false)
	cfg.Einrichtung = &einrichten.Quellen{Module: modulListe(t), Admins: einrichten.OpenAdmins(t.TempDir())}
	liste := AppListe(cfg)
	names := func(zs []einrichten.Zeile) (out []string) {
		for _, z := range zs {
			out = append(out, z.Name)
		}
		return out
	}
	if got := names(liste(context.Background(), "anna")); strings.Contains(strings.Join(got, " "), "entscheiden") {
		t.Fatalf("anna not seen as an admin yet: %v", got)
	}
	familyPageAs(t, Mehr, cfg, "GET", mehrPath, "anna", "family,admins") // a page seen with Authelia's admins
	zs := liste(context.Background(), "anna")
	if got := strings.Join(names(zs), " "); got != "entscheiden schluessel-WHO_MAIL schluessel-ARCHIV_IMAP plugin-archiv-fragen aufgabe-20 aufgabe-23" {
		t.Fatalf("the owner's rows: %s", got)
	}
	if z := zs[0]; z.Link != "https://laib.box.test/inbox" || !z.Vorn || z.Titel != "2 Entscheidungen warten" {
		t.Errorf("%+v", z)
	}
	if z := zs[5]; z.Link != "https://www.box.test/aufgaben#aufgabe-23" || z.Vorn {
		t.Errorf("%+v", z)
	}
	// dringend (homes-android#290): a decision and a missing key; a question, a task not
	for i, want := range []bool{true, true, true, false, false, false} {
		if zs[i].Dringend != want {
			t.Errorf("%s: dringend %v", zs[i].Name, zs[i].Dringend)
		}
	}
	if got := strings.Join(names(liste(context.Background(), "ben")), " "); got != "aufgabe-25 aufgabe-23 aufgabe-24" {
		t.Errorf("ben's rows: %s", got)
	}
	familyPageAs(t, Mehr, cfg, "GET", mehrPath, "anna", "family") // no longer an admin
	if got := names(liste(context.Background(), "anna")); strings.Contains(strings.Join(got, " "), "entscheiden") {
		t.Errorf("anna's admin rows stayed: %v", got)
	}
}

// The orphans have a way in (homes#362): Scannen and, for an admin while
// there are any, Scans zuordnen on Dokumente; the answers on Kontakte.
func TestDokumenteLeadsToScannenAndZuordnen(t *testing.T) {
	body := scanPage(t, &scanLaib{status: map[string]any{"zustand": "bereit", "scanner": true}}, "family")
	mustContain(t, body, `<a class=knopf href=#scannen>Scannen</a>`, `id=scannen`)
	before(t, body, "href=#scannen", "id=scannen")
	mustNotContain(t, scanPage(t, &scanLaib{unset: true}, "family"), "href=#scannen")

	_, cfg := zuordnenCfg()
	cfg.DocumentsMail = "dokumente@box.test"
	_, admin, _ := familyPage(t, Dokumente, cfg, "GET", DokumentePath, "family,admins")
	mustContain(t, admin, `<a class=knopf href=/dokumente/zuordnen>Scans zuordnen (1)</a>`)
	_, fam, _ := familyPage(t, Dokumente, cfg, "GET", DokumentePath, "family")
	mustNotContain(t, fam, "zuordnen")
}

// archiv ADR 0001 §4, homes ADR 0003: the archive's tasks are its owner's
// only. Handed over while her vault was open, they stay with their text
// while it is locked („Stand: gestern“), on Deine Aufgaben, Start and in
// the app; a key among the keys, the questions among the questions. ben
// (family) and clara (an admin) never see them, and Homes does not even
// ask anna's archive for them. The archive silent: the same rows and one
// quiet line.
func TestPluginAufgabenOnlyForItsPerson(t *testing.T) {
	gestern := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	jetzt := gestern
	answer := `{"vertrag":1,"name":"Archiv","icon":"archiv","link":"https://archiv.box.test/tresor","aufgaben":[` +
		`{"id":"schluessel","titel":"Im Archiv fehlt ein Schlüssel","warum":"Ohne ihn kommt nicht alles in dein Archiv.","link":"https://archiv.box.test/tresor/quellen","art":"schluessel"},` +
		`{"id":"frage-1a2b","titel":"Wie steht Ben zu Clara?","warum":"Vorschlag, weil beide denselben Nachnamen haben."},` +
		`{"id":"pflege","titel":"4 Dinge zum Aufräumen im Archiv","link":"https://archiv.box.test/tresor/pflege"}]}`
	anna, asked := archiv362(t, false, &answer)
	cfg := cfg362(t, false)
	cfg.Plugins = plugin.Open([]plugin.Instance{anna}, filepath.Join(t.TempDir(), "plugins.json"))
	cfg.Plugins.Now = func() time.Time { return jetzt }
	cfg.Now, cfg.Loc = func() time.Time { return gestern.Add(14 * time.Hour) }, time.UTC
	cfg.Plugins.Lesen(context.Background(), "anna")                                                  // handed over yesterday, the vault open
	answer = `{"vertrag":1,"name":"Archiv","icon":"archiv","link":"https://archiv.box.test/tresor"}` // locked now: no tasks to say
	jetzt = gestern.Add(14 * time.Hour)

	_, page, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "anna", "family")
	offen := section(t, page, "karte-offen")
	mustContain(t, offen, `<a href="https://archiv.box.test/tresor/quellen">Im Archiv fehlt ein Schlüssel</a>`,
		`<a href="https://archiv.box.test/tresor">Wie steht Ben zu Clara?</a>`, "Stand: gestern · Vorschlag, weil",
		`<a href="https://archiv.box.test/tresor/pflege">4 Dinge zum Aufräumen im Archiv</a>`)
	mustNotContain(t, page, "antwortet gerade nicht")
	before(t, offen, "fehlt ein Schlüssel", "Wie steht Ben")
	before(t, offen, "Wie steht Ben", "Für alle")
	_, start, _ := familyPageAs(t, Start, cfg, "GET", StartBoxPath, "anna", "family")
	mustContain(t, section(t, start, "karte-braucht-dich"), "Im Archiv fehlt ein Schlüssel", "Wie steht Ben zu Clara?")
	if *asked != 3 {
		t.Fatalf("anna's archive asked %d times for one hand-over and anna's two pages", *asked)
	}
	_, mehr, _ := familyPageAs(t, Mehr, cfg, "GET", mehrPath, "anna", "family")
	mustContain(t, mehr, `href="https://archiv.box.test/tresor"`, ">Archiv<")

	for _, v := range [][2]string{{"ben", "family"}, {"clara", "family,admins"}} {
		_, page, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, v[0], v[1])
		_, start, _ := familyPageAs(t, Start, cfg, "GET", StartBoxPath, v[0], v[1])
		_, mehr, _ := familyPageAs(t, Mehr, cfg, "GET", mehrPath, v[0], v[1])
		mustNotContain(t, page+start+mehr, "Im Archiv", "Aufräumen", "/tresor", "Wie steht", ">Archiv<")
	}
	cfg.Einrichtung = &einrichten.Quellen{Module: modulListe(t), Admins: einrichten.OpenAdmins(t.TempDir())}
	for _, z := range AppListe(cfg)(context.Background(), "ben") {
		if strings.HasPrefix(z.Name, "plugin-") {
			t.Errorf("ben's app has anna's %+v", z)
		}
	}
	if *asked != 3 {
		t.Errorf("anna's archive asked for another viewer: %d", *asked)
	}

	answer = "kaputt" // the archive breaks: what it handed over stays, one quiet line, nothing breaks
	_, page, _ = familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "anna", "family")
	mustContain(t, page, "<p class=k>Archiv antwortet gerade nicht.</p>", "Wie steht Ben zu Clara?", "Stand: gestern")
	_, mehr, _ = familyPageAs(t, Mehr, cfg, "GET", mehrPath, "anna", "family")
	mustContain(t, mehr, "Archiv antwortet gerade nicht.")
}

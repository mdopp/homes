package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/napi"
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

// archiv362 is anna's archive after archiv#60: its counts at
// /intern/aufgaben for her secret alone, answer its JSON; old: an archive
// before it (404). It counts who asked.
func archiv362(t *testing.T, old bool, answer string) (napi.ArchivIntern, *int) {
	t.Helper()
	asked := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked++
		if old || r.URL.Path != "/intern/aufgaben" || r.Header.Get("X-Archiv-Homes") != "anna-geheim" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return napi.ArchivIntern{URL: srv.URL, Secret: "anna-geheim"}, asked
}

// cfg362 is a box where anna's and clara's archives are there; clara's
// does not answer.
func cfg362(t *testing.T, old bool) StartConfig {
	anna, _ := archiv362(t, old, `{"offen":true,"fragen":3,"aufraeumen":0,"schluessel_fehlt":false}`)
	return StartConfig{Laib: laib362(t, old), LaibURL: "https://laib.box.test", PublicURL: "https://www.box.test",
		Archive: map[string]bool{"anna": true, "clara": true}, ArchivURL: "https://archiv.box.test",
		ArchivIntern: map[string]napi.ArchivIntern{"anna": anna, "clara": {URL: "http://127.0.0.1:1", Secret: "clara-geheim"}}}
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
		`<a href="https://archiv.box.test/tresor">Im Archiv: 3 Fragen offen</a>`, ">Im Archiv öffnen</a>",
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

// A Laib before laib#1195 (the box's v2.24.0) and an archive before
// archiv#60: no key rows, no counts, no error on the page or on Start.
func TestDeineAufgabenWithAnOlderLaib(t *testing.T) {
	cfg := cfg362(t, true)
	_, body, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "anna", "family,admins")
	mustContain(t, body, "2 Entscheidungen warten", "Zweite Platte einbauen")
	mustNotContain(t, body, "Archiv: Postfach", "Im Archiv", saysAufgabenNicht, "Whoami")
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
	if got := strings.Join(names(zs), " "); got != "entscheiden schluessel-WHO_MAIL schluessel-ARCHIV_IMAP archiv-fragen aufgabe-20 aufgabe-23" {
		t.Fatalf("the owner's rows: %s", got)
	}
	if z := zs[0]; z.Link != "https://laib.box.test/inbox" || !z.Vorn || z.Titel != "2 Entscheidungen warten" {
		t.Errorf("%+v", z)
	}
	if z := zs[5]; z.Link != "https://www.box.test/aufgaben#aufgabe-23" || z.Vorn {
		t.Errorf("%+v", z)
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

// archiv#60, owner 2026-11-12: the archive's counts are its owner's only.
// anna sees hers — a missing key among the keys, the questions and the
// clean-up among the questions, with the day they were counted while her
// vault is closed — on Deine Aufgaben, Start and in the app; ben (family)
// and clara (an admin) never, and Homes does not even ask anna's archive
// for them.
func TestArchivZahlenOnlyForItsOwner(t *testing.T) {
	cfg := cfg362(t, false)
	gestern := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	cfg.Now, cfg.Loc = func() time.Time { return gestern.Add(14 * time.Hour) }, time.UTC
	anna, asked := archiv362(t, false, `{"offen":false,"am":"`+gestern.Format(time.RFC3339)+`","fragen":1,"aufraeumen":4,"schluessel_fehlt":true}`)
	cfg.ArchivIntern["anna"] = anna
	_, page, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, "anna", "family")
	offen := section(t, page, "karte-offen")
	mustContain(t, offen, `<a href="https://archiv.box.test/tresor/quellen">Im Archiv fehlt ein Schlüssel</a>`,
		`<a href="https://archiv.box.test/tresor">Im Archiv: 1 Frage offen</a>`, "Stand: gestern · Wer zu wem gehört",
		`<a href="https://archiv.box.test/tresor/pflege">4 Dinge zum Aufräumen im Archiv</a>`)
	before(t, offen, "fehlt ein Schlüssel", "1 Frage offen")
	before(t, offen, "1 Frage offen", "Für alle")
	_, start, _ := familyPageAs(t, Start, cfg, "GET", StartBoxPath, "anna", "family")
	mustContain(t, section(t, start, "karte-braucht-dich"), "Im Archiv fehlt ein Schlüssel", "Im Archiv: 1 Frage offen")
	if *asked != 2 {
		t.Fatalf("anna's archive asked %d times for anna's two pages", *asked)
	}

	for _, v := range [][2]string{{"ben", "family"}, {"clara", "family,admins"}} {
		_, page, _ := familyPageAs(t, DeineAufgaben, cfg, "GET", aufgabenPath, v[0], v[1])
		_, start, _ := familyPageAs(t, Start, cfg, "GET", StartBoxPath, v[0], v[1])
		mustNotContain(t, page+start, "Im Archiv", "Aufräumen", "/tresor")
	}
	cfg.Einrichtung = &einrichten.Quellen{Module: modulListe(t), Admins: einrichten.OpenAdmins(t.TempDir())}
	for _, z := range AppListe(cfg)(context.Background(), "ben") {
		if strings.HasPrefix(z.Name, "archiv-") {
			t.Errorf("ben's app has anna's %+v", z)
		}
	}
	if *asked != 2 {
		t.Errorf("anna's archive asked for another viewer: %d", *asked)
	}
}

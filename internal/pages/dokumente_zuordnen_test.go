package pages

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeZuordnen is the list Dokumente left (laib DATA.md gap 6) and the answers given.
type fakeZuordnen struct {
	offen    []Scan
	personen []Person
	gegeben  []string
}

func (f *fakeZuordnen) Stand() ([]Scan, []Person) { return f.offen, f.personen }

func (f *fakeZuordnen) Antworte(id int, zeichen, an, von string) error {
	for i, s := range f.offen {
		if s.ID == id && s.Zeichen == zeichen {
			f.offen[i].An = an
			f.gegeben = append(f.gegeben, an+" von "+von)
			return nil
		}
	}
	return errors.New("gone")
}

func zuordnenCfg() (*fakeZuordnen, StartConfig) {
	z := &fakeZuordnen{offen: []Scan{{ID: 7, Zeichen: "z7", Absender: "Praxis Dr. Muster", Datum: "2026-11-03", Seiten: 2, Seit: "10.11. 09:14"}},
		personen: []Person{{"anna", "Anna"}, {"ben", "Ben"}}}
	return z, StartConfig{Zuordnen: z}
}

// Only the admins see whose a scan is, and only what they need to decide: the sender, the date, the pages.
func TestWemGehoertDieserScanIsTheAdmins(t *testing.T) {
	_, cfg := zuordnenCfg()
	if code, body, _ := familyPage(t, ZuordnenSeite, cfg, "GET", ZuordnenPath, "family"); code != http.StatusNotFound || strings.Contains(body, "Praxis") {
		t.Fatalf("a family member: %d\n%s", code, body)
	}
	code, body, _ := familyPage(t, ZuordnenSeite, cfg, "GET", ZuordnenPath, "admins,family")
	if code != http.StatusOK {
		t.Fatalf("%d", code)
	}
	mustContain(t, body, "<h1>Wem gehört dieser Scan?</h1>", "Von Praxis Dr. Muster", "2026-11-03 · 2 Seite(n)",
		`<button name=an value="anna"`, `<button name=an value="ben"`, `<button name=an value="haushalt"`, `name=zeichen value="z7"`, "sieht sie niemand in der Familie")
	as := einrichtung(withGroups("admins"), cfg)
	if len(as) == 0 || as[0].titel != "Wem gehört dieser Scan?" || as[0].href != ZuordnenPath || as[0].art != artFrage || as[0].stand != "1 offen" {
		t.Fatalf("the admins' task: %+v", as)
	}
	if as := einrichtung(withGroups("family"), cfg); len(as) != 0 {
		t.Fatalf("not a family member's task: %+v", as)
	}
}

func withGroups(g string) *http.Request {
	r := httptest.NewRequest("GET", "/aufgaben", nil)
	r.Header.Set("Remote-User", "anna")
	r.Header.Set("Remote-Groups", g)
	return r
}

// An answer is kept for what the admin saw and said on the page; a changed list is said, nothing given.
func TestWemGehoertDieserScanAnswered(t *testing.T) {
	z, cfg := zuordnenCfg()
	post := func(form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", ZuordnenPath, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Remote-User", "mdopp")
		r.Header.Set("Remote-Groups", "admins")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		rec := httptest.NewRecorder()
		ZuordnenSeite(cfg).ServeHTTP(rec, r)
		return rec
	}
	if rec := post(url.Values{"id": {"7"}, "zeichen": {"alt"}, "an": {"anna"}}); rec.Header().Get("Location") != ZuordnenPath+"?nicht=1" || len(z.gegeben) != 0 {
		t.Fatalf("a mark the admin did not see: %v %v", rec.Header(), z.gegeben)
	}
	if rec := post(url.Values{"id": {"7"}, "zeichen": {"z7"}, "an": {"ben"}}); rec.Header().Get("Location") != ZuordnenPath+"?gemerkt=1" {
		t.Fatalf("%v", rec.Header())
	}
	if len(z.gegeben) != 1 || z.gegeben[0] != "ben von mdopp" {
		t.Fatalf("%v", z.gegeben)
	}
	_, body, _ := familyPage(t, ZuordnenSeite, cfg, "GET", ZuordnenPath+"?gemerkt=1", "admins")
	mustContain(t, body, "Gemerkt. Dokumente gibt es beim nächsten Durchgang weiter", "Geht an Ben, sobald Dokumente das nächste Mal läuft")
	if as := einrichtung(withGroups("admins"), cfg); len(as) != 0 {
		t.Fatalf("answered: no task left: %+v", as)
	}
}

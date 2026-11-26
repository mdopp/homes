package pages

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/bote"
	"github.com/mdopp/homes/internal/bote/botefake"
	"github.com/mdopp/homes/internal/ha/hafake"
	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/laib/laibfake"
	"github.com/mdopp/homes/internal/tagaktion"
)

// ADR 0004 §5, §8: a tag's own page behind the login, and the admins'
// answer to a wish. Viewers: michael (the owner, an admin), petra (an
// admin who is not the owner), anna (who asks), ben (another person).

const tagBuero = "homes-b0e70e000001"

type tagRig struct {
	ha   *hafake.HA
	laib *laibfake.Laib
	bote *botefake.Server
	cfg  StartConfig
}

func newTagRig(t *testing.T) *tagRig {
	t.Helper()
	fake, hc := house(t)
	lf := laibfake.New()
	srv := lf.Server()
	t.Cleanup(srv.Close)
	b := botefake.New("k")
	t.Cleanup(b.Close)
	dir := t.TempDir()
	store, _ := tagaktion.Open(dir)
	d := &tagaktion.Dienst{HA: hc, Laib: &laib.Client{URL: srv.URL, Token: "x"}, Store: store, Bote: &bote.Client{URL: b.Base(), Key: "k"},
		PublicURL: "https://www.box.test", Dir: dir, Admins: func() []string { return []string{"michael", "petra"} }}
	fake.AddTag(tagBuero, "Bürotür")
	fake.AddAutomation("1700000000001", "Gute Nacht", map[string]any{"id": "1700000000001", "alias": "Gute Nacht", "triggers": []any{}})
	fake.AddAutomation("1700000000002", "Flurlicht", map[string]any{"id": "1700000000002", "triggers": []any{map[string]any{"trigger": "tag", "tag_id": tagBuero}}})
	return &tagRig{fake, lf, b, StartConfig{HA: hc, Tags: d, PublicURL: "https://www.box.test"}}
}

var tagViewers = map[string]map[string]string{
	"michael": {"Remote-Groups": "admins", "Remote-Name": "Michael"},
	"petra":   {"Remote-Groups": "admins,family"},
	"anna":    {"Remote-Groups": "family", "Remote-Name": "Anna"},
	"ben":     {"Remote-Groups": "family"},
}

func tagGet(t *testing.T, h http.HandlerFunc, target, user string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	req.Header.Set("Remote-User", user)
	for k, v := range tagViewers[user] {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func tagPostAs(t *testing.T, h http.HandlerFunc, target string, form url.Values, user string) *httptest.ResponseRecorder {
	t.Helper()
	return post(t, h, target, form, user, tagViewers[user])
}

// Idle: the name, what it does, one button; a GET touches nothing.
func TestTheTagPageShowsAndAGetDoesNothing(t *testing.T) {
	r := newTagRig(t)
	rec := tagGet(t, Tag(r.cfg), TagPath+tagBuero, "anna")
	body := rec.Body.String()
	mustContain(t, body, "<h1>Bürotür</h1>", "Meldet sich bei Home Assistant", `<input type=hidden name=tun value=ausloesen><button data-laeuft="Löst aus …">Auslösen</button>`,
		"<h2>Automationen</h2>", "<span>Flurlicht</span>", `<option value="1700000000001">Gute Nacht</option>`, `name=satz`)
	mustNotContain(t, body, `<option value="1700000000002">`)
	if rec.Code != 200 || len(r.ha.Events()) != 0 {
		t.Fatalf("%d, events %v", rec.Code, r.ha.Events())
	}
	if rec := tagGet(t, Tag(r.cfg), TagPath+"homes-000000000000", "anna"); !strings.Contains(rec.Body.String(), "Unbekannter Tag") {
		t.Fatalf("unnamed: %s", rec.Body.String())
	}
	if rec := tagGet(t, Tag(r.cfg), TagPath+"a/b", "anna"); rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
}

// Done and failed: the button acts as the signed-in person, the outcome on
// the page; a refusal is a sentence (the script shows it, the page stays).
func TestTheTagPagesButtonActsAsThePerson(t *testing.T) {
	r := newTagRig(t)
	rec := tagPostAs(t, Tag(r.cfg), TagPath+tagBuero, url.Values{"tun": {"ausloesen"}}, "ben")
	mustContain(t, rec.Body.String(), `<div class=gut role=status><span class=punkt></span><p>Ausgelöst: Home Assistant hat es bekommen.</p>`)
	if ev := r.ha.Events(); len(ev) != 1 || ev[0].Data["person"] != "ben" || ev[0].Data["tag_id"] != tagBuero {
		t.Fatalf("%+v", ev)
	}
	r.ha.Down(true)
	rec = tagPostAs(t, Tag(r.cfg), TagPath+tagBuero, url.Values{"tun": {"ausloesen"}}, "ben")
	if rec.Code != http.StatusBadGateway || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(rec.Body.String(), "Home Assistant antwortet gerade nicht") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := post(t, Tag(r.cfg), TagPath+tagBuero, url.Values{"tun": {"ausloesen"}}, "ben", map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusForbidden {
		t.Fatalf("from elsewhere: %d", rec.Code)
	}
}

func TestTheScannerTagsPage(t *testing.T) {
	r := newTagRig(t)
	r.cfg.Tags.Store.Setze("homes-5ca11e000001", tagaktion.ScannerFragen)
	if rec := tagGet(t, Tag(r.cfg), TagPath+"homes-5ca11e000001", "anna"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != DokumentePath+"#scannen" {
		t.Fatalf("fragen: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	r.cfg.Tags.Store.Setze("homes-5ca11e000001", tagaktion.ScannerIch)
	mustContain(t, tagGet(t, Tag(r.cfg), TagPath+"homes-5ca11e000001", "anna").Body.String(), "Scanner – direkt für mich.", `data-laeuft="Startet …">Scannen – für mich</button>`)
	rec := tagPostAs(t, Tag(r.cfg), TagPath+"homes-5ca11e000001", url.Values{"tun": {"ausloesen"}}, "anna")
	mustContain(t, rec.Body.String(), "Der Scanner startet – nur für dich.")
	if o := r.laib.Scanauftraege(); len(o) != 1 || o[0]["person"] != "anna" {
		t.Fatalf("%v", o)
	}
}

// An admin: the sentence first, then written; the owner and another admin alike.
func TestAnAdminLinksOnThePage(t *testing.T) {
	for _, admin := range []string{"michael", "petra"} {
		r := newTagRig(t)
		form := url.Values{"tun": {"verbinden"}, "automation": {"1700000000001"}}
		body := tagPostAs(t, Tag(r.cfg), TagPath+tagBuero, form, admin).Body.String()
		mustContain(t, body, "<h2>So wird es</h2>", "Wenn jemand ‚Bürotür‘ berührt, startet ‚Gute Nacht‘.", `name=bestaetigt value=1`, `data-laeuft="Verbindet …">Ja, verbinden</button>`)
		mustNotContain(t, body, "Ein Admin gibt das frei")
		if len(r.ha.AutomationConfig("1700000000001")["triggers"].([]any)) != 0 {
			t.Fatal("the preview wrote")
		}
		form.Set("bestaetigt", "1")
		body = tagPostAs(t, Tag(r.cfg), TagPath+tagBuero, form, admin).Body.String()
		mustContain(t, body, "Verbunden: ‚Bürotür‘ startet jetzt ‚Gute Nacht‘.", "<span>Gute Nacht</span>")
		if len(r.ha.AutomationConfig("1700000000001")["triggers"].([]any)) != 1 {
			t.Fatalf("%s: not written", admin)
		}
	}
}

// Another person: waiting on the page (fetched again), the admins told and
// tasked; the freigabe page per viewer; the first admin's answer; the outcome.
func TestAPersonsWishWaitsAndTheFirstAdminAnswers(t *testing.T) {
	r := newTagRig(t)
	form := url.Values{"tun": {"verbinden"}, "automation": {"1700000000001"}, "bestaetigt": {"1"}}
	rec := tagPostAs(t, Tag(r.cfg), TagPath+tagBuero, form, "anna")
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(loc, TagPath+tagBuero+"?freigabe=") {
		t.Fatalf("%d %q", rec.Code, loc)
	}
	id := strings.TrimPrefix(loc, TagPath+tagBuero+"?freigabe=")
	mustContain(t, tagGet(t, Tag(r.cfg), loc, "anna").Body.String(), "<div hidden data-auffrischen=10></div>", "Wartet auf Freigabe: Anna möchte ‚Bürotür‘ mit ‚Gute Nacht‘ verbinden.")
	mustNotContain(t, tagGet(t, Tag(r.cfg), loc, "ben").Body.String(), "Wartet auf Freigabe")
	if len(r.laib.Gesagte()) != 2 || !strings.Contains(r.laib.Gesagte()[0], "Freigeben: https://www.box.test/tag/freigabe/"+id) {
		t.Fatalf("told: %v", r.laib.Gesagte())
	}
	h := TagFreigabe(r.cfg)
	for user, want := range map[string][]string{
		"michael": {"<h1>Freigabe</h1>", `<button name=entscheiden value=ja data-laeuft="Gibt frei …">Freigeben</button>`, "Ablehnen"},
		"petra":   {`value=ja`, `value=nein`},
		"anna":    {"Wartet auf Freigabe durch einen Admin.", "<div hidden data-auffrischen=10>"},
	} {
		rec := tagGet(t, h, TagFreigabePath+id, user)
		mustContain(t, rec.Body.String(), want...)
		if user == "anna" {
			mustNotContain(t, rec.Body.String(), "value=ja")
		}
	}
	if rec := tagGet(t, h, TagFreigabePath+id, "ben"); rec.Code != http.StatusNotFound {
		t.Fatalf("ben sees anna's wish: %d", rec.Code)
	}
	if rec := tagPostAs(t, h, TagFreigabePath+id, url.Values{"entscheiden": {"ja"}}, "anna"); rec.Code != http.StatusForbidden {
		t.Fatalf("anna decides: %d", rec.Code)
	}
	rec = tagPostAs(t, h, TagFreigabePath+id, url.Values{"entscheiden": {"ja"}}, "petra")
	mustContain(t, rec.Body.String(), "Verbunden: ‚Bürotür‘ startet jetzt ‚Gute Nacht‘.")
	if rec := tagPostAs(t, h, TagFreigabePath+id, url.Values{"entscheiden": {"nein"}}, "michael"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "schon entschieden") {
		t.Fatalf("second answer: %d %s", rec.Code, rec.Body.String())
	}
	mustContain(t, tagGet(t, Tag(r.cfg), loc, "anna").Body.String(), "Verbunden: ‚Bürotür‘ startet jetzt ‚Gute Nacht‘.")
	mustNotContain(t, tagGet(t, Tag(r.cfg), loc, "anna").Body.String(), "<div hidden data-auffrischen=10>")
}

// Deine Aufgaben: the wish is a task for each admin, the owner and
// another, until answered; not for the person who asked, nor anyone else.
func TestTheWishIsAnAdminsTask(t *testing.T) {
	r := newTagRig(t)
	got, _ := r.cfg.Tags.Aendern(t.Context(), tagaktion.Wunsch{Art: tagaktion.Verbinden, Tag: tagBuero, Automation: "1700000000001"}, tagaktion.Wer{Login: "anna"}, true)
	for user, sees := range map[string]bool{"michael": true, "petra": true, "anna": false, "ben": false} {
		body := tagGet(t, DeineAufgaben(r.cfg), "/aufgaben", user).Body.String()
		if has := strings.Contains(body, TagFreigabePath+got.Freigabe); has != sees {
			t.Errorf("%s sees the task: %v", user, has)
		}
	}
}

// The new automation by a sentence: the Bote's answer on the page for an
// admin; its failure in words.
func TestANewAutomationOnThePage(t *testing.T) {
	r := newTagRig(t)
	form := url.Values{"tun": {"neu"}, "satz": {"Licht im Flur an"}, "bestaetigt": {"1"}}
	mustContain(t, tagPostAs(t, Tag(r.cfg), TagPath+tagBuero, form, "michael").Body.String(), "<h2>Antwort</h2>", "Erledigt: das Licht im Flur ist an.", "Sprechen-Feld")
	r.bote.Status = http.StatusBadGateway
	if rec := tagPostAs(t, Tag(r.cfg), TagPath+tagBuero, form, "michael"); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "antwortet gerade nicht") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAssetLinks(t *testing.T) {
	get := func(shas string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		AssetLinks(shas)(rec, httptest.NewRequest("GET", AssetLinksPath, nil))
		return rec
	}
	if rec := get(""); rec.Code != http.StatusNotFound {
		t.Fatalf("without: %d", rec.Code)
	}
	if rec := get("nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("only a bad one: %d", rec.Code)
	}
	a := strings.Repeat("ab", 32)
	b := strings.Repeat("0F:", 31) + "0F"
	rec := get(a + ", " + b + ",nope")
	want := `[{"relation":["delegate_permission/common.handle_all_urls"],"target":{"namespace":"android_app","package_name":"cloud.dopp.homes",` +
		`"sha256_cert_fingerprints":["` + strings.TrimSuffix(strings.Repeat("AB:", 32), ":") + `","` + b + `"]}}]`
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" || rec.Body.String() != want {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

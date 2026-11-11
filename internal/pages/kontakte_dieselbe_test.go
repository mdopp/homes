package pages

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/immich"
	"github.com/mdopp/homes/internal/immich/immichfake"
)

// dieselbeConfig: anna's key; Immich has six named persons, Anna (40
// photos) the first, each with three unnamed faces near it (u0 closest to
// Anna, 1 photo): 18 pairs; the hourly run done. The household has Xaver,
// whom Immich does not know, and karten.
func dieselbeConfig(t *testing.T, karten ...string) (StartConfig, *immichfake.Server) {
	t.Helper()
	_, c := addressBook(t, append([]string{"x|Xaver||"}, karten...)...)
	fake := immichfake.New("key")
	t.Cleanup(fake.Close)
	for i, name := range []string{"Anna", "Ben", "Carla", "Dora", "Emil", "Frida"} {
		id := "p-" + strings.ToLower(name)
		fake.Person(id, name, "1", false, faceJPEG(t, color.RGBA{200, uint8(30 * i), 40, 255}))
		fake.Lage(id, 40-i, float64(100*i), 0, false)
		for j := range 3 {
			u := fmt.Sprint("u", 3*i+j)
			fake.Person(u, "", "1", false, faceJPEG(t, color.RGBA{40, uint8(10 * j), 40, 255}))
			fake.Lage(u, 3*i+j+1, float64(100*i+j+1), 0, false)
		}
	}
	g := &gesichter.Gesichter{Immich: &immich.Client{URL: fake.URL, Key: "key"}, Kontakte: c, Web: "https://photos.box.test", Dir: t.TempDir()}
	g.Auffrischen(t.Context())
	g.Vorschlagen(t.Context())
	return StartConfig{Contacts: c, Gesichter: g}, fake
}

// dieselbeAlle is the task's page with every question at once („Alle zeigen“).
const dieselbeAlle = DieselbePath + "?alle=1"

// antworte posts an answer as user from Homes' own page to Dieselbe.
func antworte(cfg StartConfig, user string, form url.Values) *httptest.ResponseRecorder {
	return antworteBei(Dieselbe(cfg), dieselbeAlle, user, form)
}

// antworteBei posts an answer as user from Homes' own page to h at pfad.
func antworteBei(h http.Handler, pfad, user string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", pfad, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Remote-User", user)
	req.Header.Set("Remote-Email", user+"@box.test")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Only the key's owner: with no contact of these persons, the task's page
// shows the pairs (one per named person) with the two faces, names, photo counts and
// three buttons, no segment of its own; anyone else goes to Kontakte, and
// a post of theirs merges nothing.
func TestDieselbeNurEigentuemer(t *testing.T) {
	cfg, fake := dieselbeConfig(t)
	mustNotContain(t, as(Kontakte, cfg, KontaktePath, "anna", "anna@box.test").Body.String(), DieselbePath+">", "class=anzahl")
	rec := as(Dieselbe, cfg, dieselbeAlle, "anna", "anna@box.test")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, body)
	}
	mustContain(t, body, "<a href=/kontakte>Kontakte</a> › <span>Dieselbe Person?</span></nav>", "<h1>Dieselbe Person?</h1>", "<b>Anna</b><span>40 Fotos</span>", "<i>ohne Namen</i><span>1 Foto</span>", `src="data:image/jpeg;base64,`,
		`name=a value="p-anna"`, `name=b value="u0"`, `value=ja data-laeuft="Führt zusammen …">Ja, dieselbe Person</button>`, `>Nein</button>`, `>Überspringen</button>`)
	if strings.Count(body, "<section class=paar>") != 6 { // one per named person: only its closest (owner 2026-11-09)
		t.Fatalf("not six: %d", strings.Count(body, "<section class=paar>"))
	}
	if rec := as(Dieselbe, cfg, DieselbePath, "ben", "ben@box.test"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != KontaktePath {
		t.Fatalf("ben: %d", rec.Code)
	}
	if rec := asAdmin(Dieselbe, cfg, DieselbePath); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != KontaktePath {
		t.Fatalf("admin: %d", rec.Code)
	}
	if rec := antworte(cfg, "ben", url.Values{"a": {"p-anna"}, "b": {"u0"}, "antwort": {"ja"}}); rec.Code != http.StatusSeeOther || len(fake.Merges()) != 0 {
		t.Fatalf("ben merged: %d %v", rec.Code, fake.Merges())
	}
}

// owner 2026-11-08 („alles auf dem Kontakt, damit weniger Listen“): a pair
// of a contact's person is asked on the contact: its row counts it, the
// task's address opens it, Ja there merges and is said there; once none
// is left the task's address shows what belongs to no contact.
func TestDieselbeAufDemKontakt(t *testing.T) {
	cfg, fake := dieselbeConfig(t, "a|Anna||")
	list := as(Kontakte, cfg, KontaktePath, "anna", "anna@box.test").Body.String()
	mustContain(t, list, `<a href="/kontakte/a">Anna <span class=anzahl aria-label="1 offen">1</span></a>`, `<a href="/kontakte/x">Xaver</a>`)
	if rec := as(Dieselbe, cfg, dieselbeAlle, "anna", "anna@box.test"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<form method=post action="/kontakte/dieselbe?alle=1"><input type=hidden name=a value="p-anna"><input type=hidden name=b value="u0">`) {
		t.Fatalf("the task answers the pairs in place: %d", rec.Code) // owner 2026-11-09: no click-through
	}
	body := as(Kontakt, cfg, "/kontakte/a", "anna", "anna@box.test").Body.String()
	mustContain(t, body, "<h2>Dieselbe Person?</h2>", `<form method=post action="/kontakte/a"><input type=hidden name=a value="p-anna"><input type=hidden name=b value="u0">`)
	if n := strings.Count(body, "<section class=paar>"); n != 1 {
		t.Fatalf("%d pairs on Anna", n)
	}
	mustNotContain(t, as(Kontakt, cfg, "/kontakte/a", "ben", "ben@box.test").Body.String(), "Dieselbe Person?")
	rec := antworteBei(Kontakt(cfg), "/kontakte/a", "anna", url.Values{"a": {"p-anna"}, "b": {"u0"}, "antwort": {"ja"}})
	mustContain(t, rec.Body.String(), "<h1>Anna</h1>", "Zusammengeführt: Die Fotos gehören jetzt zu Anna.")
	if fmt.Sprint(fake.Merges()) != "[[p-anna u0]]" {
		t.Fatalf("%v", fake.Merges())
	}
	rest := as(Dieselbe, cfg, dieselbeAlle, "anna", "anna@box.test")
	if rest.Code != http.StatusOK || strings.Contains(rest.Body.String(), `value="p-anna"`) || !strings.Contains(rest.Body.String(), `value="p-ben"`) {
		t.Fatalf("what is left: %d", rest.Code)
	}
}

// Ja merges in Immich and says into whom; Nein is said and kept; a pair
// gone is said; an answer from elsewhere or no answer is refused; a
// missing person.merge is named on the page.
func TestDieselbeAntworten(t *testing.T) {
	cfg, fake := dieselbeConfig(t)
	as(Kontakte, cfg, KontaktePath, "anna", "anna@box.test") // Homes learns whose key it is
	rec := antworte(cfg, "anna", url.Values{"a": {"p-anna"}, "b": {"u0"}, "antwort": {"ja"}})
	mustContain(t, rec.Body.String(), "Zusammengeführt: Die Fotos gehören jetzt zu Anna.")
	if fmt.Sprint(fake.Merges()) != "[[p-anna u0]]" {
		t.Fatalf("%v", fake.Merges())
	}
	mustContain(t, antworte(cfg, "anna", url.Values{"a": {"u0"}, "b": {"p-anna"}, "antwort": {"nein"}}).Body.String(), "Diesen Vorschlag gibt es nicht mehr.")
	mustContain(t, antworte(cfg, "anna", url.Values{"a": {"p-ben"}, "b": {"u3"}, "antwort": {"nein"}}).Body.String(), "Gemerkt: Nach diesen beiden fragt Homes nicht wieder.")
	keys := map[string]bool{}
	for _, p := range cfg.Gesichter.Paare() {
		keys[p.Key()] = true
	}
	if keys["p-ben|u3"] || keys["p-ben|u4"] || !keys["p-carla|u6"] { // a Nein ends Ben's (owner 2026-11-09)
		t.Errorf("the pairs after Nein: %v", keys)
	}
	if rec := antworte(cfg, "anna", url.Values{"a": {"p-anna"}, "b": {"u2"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("no answer: %d", rec.Code)
	}
	req := httptest.NewRequest("POST", DieselbePath, strings.NewReader("a=p-anna&b=u2&antwort=ja"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("Remote-Email", "anna@box.test")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec = httptest.NewRecorder()
	if Dieselbe(cfg).ServeHTTP(rec, req); rec.Code != http.StatusForbidden {
		t.Fatalf("from elsewhere: %d", rec.Code)
	}
	fake.Ohne = []string{"person.merge"}
	cfg.Gesichter.Vorschlagen(t.Context())
	p := cfg.Gesichter.Paare()[0]
	mustContain(t, antworte(cfg, "anna", url.Values{"a": {p.A.ID}, "b": {p.B.ID}, "antwort": {"ja"}}).Body.String(), "fehlt diese Berechtigung: person.merge")
	if len(fake.Merges()) != 1 {
		t.Fatal("merged without the permission")
	}
}

// Each face opens all of its person's photos in Immich in a new tab, and
// under it a grey line each for its places and years and who else is in
// its photos, read in the background (owner 2026-11-07).
func TestDieselbeKontext(t *testing.T) {
	cfg, fake := dieselbeConfig(t)
	fake.Bibliothek = []immichfake.Foto{
		{Wann: "2019-07-01T10:00:00.000Z", Stadt: "Münster", Wer: []string{"p-anna", "p-ben"}},
		{Wann: "2012-07-01T10:00:00.000Z", Land: "Italien", Wer: []string{"p-anna", "u0"}},
	}
	cfg.Gesichter.Umfelder(t.Context())
	body := as(Dieselbe, cfg, dieselbeAlle, "anna", "anna@box.test").Body.String()
	mustContain(t, body, `<a href="https://photos.box.test/people/p-anna" target=_blank rel=noopener aria-label="Alle Fotos von Anna in Immich"><img`,
		`<a href="https://photos.box.test/people/u0" target=_blank rel=noopener aria-label="Alle Fotos dieses Gesichts in Immich">`,
		`<small>Orte: Italien, Münster · 2012–2019</small><small>Auch dabei: Ben</small>`, `<small>Orte: Italien · 2012</small><small>Auch dabei: Anna</small>`)
}

// Under each face up to three more, cut from other photos, each opening
// its whole photo in Immich (owner 2026-11-11); Immich's own face is one
// colour here, so the first cut one is shown big in its place.
func TestDieselbeWeitere(t *testing.T) {
	cfg, fake := dieselbeConfig(t)
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := range 300 {
		for x := range 400 {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, nil)
	for _, id := range []string{"f1", "f2", "f3", "f4"} {
		fake.Bibliothek = append(fake.Bibliothek, immichfake.Foto{ID: id, Wann: "2020-01-01T10:00:00.000Z", Wer: []string{"p-anna"}, Breite: 4000, Hoehe: 3000,
			Rahmen: map[string][4]int{"p-anna": {1000, 1000, 1600, 1700}}, Bild: b.Bytes()})
	}
	cfg.Gesichter.Umfelder(t.Context())
	body := as(Dieselbe, cfg, dieselbeAlle, "anna", "anna@box.test").Body.String()
	mustContain(t, body, `<span class=weitere><a href="https://photos.box.test/photos/f2" target=_blank rel=noopener aria-label="Das ganze Foto in Immich"><img src="data:image/jpeg;base64,`,
		`<a href="https://photos.box.test/photos/f3" target=_blank`)
	mustNotContain(t, body, "photos/f1", "photos/f4")
}

// One question at a time (owner 2026-11-11, „rigoros einfach“): the task's
// address shows one card under one heading, which one of how many it is,
// „Nächste Frage“ and „Alle zeigen“; an answer stays at its place, so the
// next question follows; after a pair's Nein its unnamed face comes
// first. The page is the key owner's alone, at every address.
func TestDieselbeEineFrage(t *testing.T) {
	cfg, _ := dieselbeConfig(t)
	alle := as(Dieselbe, cfg, dieselbeAlle, "anna", "anna@box.test").Body.String()
	n := strings.Count(alle, "<section class=")
	if n < 3 || strings.Contains(alle, "Nächste Frage") {
		t.Fatalf("alle: %d cards\n%s", n, alle)
	}
	body := as(Dieselbe, cfg, DieselbePath, "anna", "anna@box.test").Body.String()
	mustContain(t, body, "<h1>Dieselbe Person?</h1>", ">Überspringen</button>", fmt.Sprintf(`<p class=k>Frage 1 von %d · <a href="/kontakte/dieselbe?ab=1">Nächste Frage</a> · <a href="/kontakte/dieselbe?alle=1">Alle zeigen</a></p>`, n),
		`<form method=post action="/kontakte/dieselbe?ab=0">`)
	if strings.Count(body, "<section class=") != 1 || strings.Count(body, "<h1>") != 1 || strings.Contains(body, "<h2>") {
		t.Fatalf("not one card under one heading:\n%s", body)
	}
	// one tap first (owner 2026-11-11): every „Dieselbe Person?“ and „Ist das …?“ before any „Wer ist das?“
	tippen, getippt := len(cfg.Gesichter.Paare())+len(cfg.Gesichter.Fragen()), false
	if tippen == 0 || tippen == n {
		t.Fatalf("the fixture needs both kinds: %d of %d", tippen, n)
	}
	for ab := range n {
		seite := as(Dieselbe, cfg, fmt.Sprintf("%s?ab=%d", DieselbePath, ab), "anna", "anna@box.test").Body.String()
		wer := strings.Contains(seite, "<h1>Wer ist das?</h1>")
		if ab < tippen && wer || ab >= tippen && !wer || getippt && !wer {
			t.Fatalf("question %d of %d (%d one tap): typing out of order:\n%s", ab+1, n, tippen, seite)
		}
		getippt = getippt || wer
	}
	// the last one is a face to name; the one after it is the first again
	letzte := as(Dieselbe, cfg, fmt.Sprintf("%s?ab=%d", DieselbePath, n-1), "anna", "anna@box.test").Body.String()
	mustContain(t, letzte, "<h1>Wer ist das?</h1>", fmt.Sprintf("Frage %d von %d", n, n), `<a href="/kontakte/dieselbe?ab=0">Nächste Frage</a>`, ">Kenne ich nicht</button>")
	if strings.Count(letzte, "<section class=") != 1 {
		t.Fatalf("the last: not one card:\n%s", letzte)
	}
	for _, ab := range []string{"-3", "999", "x"} { // an address that fits nothing is the first question
		mustContain(t, as(Dieselbe, cfg, DieselbePath+"?ab="+ab, "anna", "anna@box.test").Body.String(), "Frage 1 von ")
	}
	// an answer at ?ab=… comes back there: the next question, one fewer
	erste := cfg.Gesichter.Wer()[0].ID
	rec := antworteBei(Dieselbe(cfg), fmt.Sprintf("%s?ab=%d", DieselbePath, tippen), "anna", url.Values{"wer": {erste}, "antwort": {"nein"}})
	mustContain(t, rec.Body.String(), "role=status", fmt.Sprintf("Frage %d von ", tippen+1), fmt.Sprintf(`action="/kontakte/dieselbe?ab=%d"`, tippen))
	if strings.Count(rec.Body.String(), "<section class=") != 1 || strings.Contains(rec.Body.String(), `name=wer value="`+erste+`"`) {
		t.Fatalf("after an answer: not one card:\n%s", rec.Body)
	}
	// a pair's Nein: its unnamed face to name comes first, whatever was asked
	p := cfg.Gesichter.Paare()[0]
	rec = antworteBei(Dieselbe(cfg), DieselbePath+"?ab=0", "anna", url.Values{"a": {p.A.ID}, "b": {p.B.ID}, "antwort": {"nein"}})
	mustContain(t, rec.Body.String(), "<h1>Wer ist das dann?</h1>", `name=wer value="`+p.B.ID+`"`, `action="/kontakte/dieselbe?ab=0"`)
	// another person and an admin without the key see none of it, at any address
	for _, pfad := range []string{DieselbePath, DieselbePath + "?ab=1", dieselbeAlle} {
		if rec := as(Dieselbe, cfg, pfad, "ben", "ben@box.test"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != KontaktePath {
			t.Fatalf("ben at %s: %d", pfad, rec.Code)
		}
		if rec := asAdmin(Dieselbe, cfg, pfad); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != KontaktePath {
			t.Fatalf("an admin at %s: %d", pfad, rec.Code)
		}
	}
	if rec := antworteBei(Dieselbe(cfg), DieselbePath+"?ab=0", "ben", url.Values{"wer": {"u1"}, "antwort": {"nein"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("ben answers: %d", rec.Code)
	}
}

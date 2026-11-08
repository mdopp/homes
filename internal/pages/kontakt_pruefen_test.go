package pages

import (
	"bytes"
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/caldav/davfake"
	"github.com/mdopp/homes/internal/kontaktantwort"
)

const pruefenKarte = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:h1\r\nFN:Anna Beispiel\r\nN:Beispiel;Anna;;;\r\nTEL;TYPE=CELL:+49 151 23456789\r\n" +
	"EMAIL:anna@example.org\r\nORG:Bäckerei\r\nX-IBAN:DE00\r\nEND:VCARD\r\n"

type noDNS struct{}

func (noDNS) LookupMX(context.Context, string) ([]*net.MX, error) {
	return []*net.MX{{Host: "mx.example."}}, nil
}
func (noDNS) LookupHost(context.Context, string) ([]string, error) { return []string{"192.0.2.1"}, nil }

// pruefenCfg is a house with one shared card, a Handy-Kontakt of anna's
// and the store; the cards' text is dav's.
func pruefenCfg(t *testing.T) (StartConfig, *davfake.Server) {
	t.Helper()
	dav, c := calendar(t)
	dav.AddAddressBook("/homes/haushalt-kontakte/", "Haushalt")
	dav.Put("/homes/haushalt-kontakte/h1.vcf", pruefenKarte)
	dav.AddAddressBook("/anna/handy-kontakte/", "Handy-Kontakte")
	dav.Put("/anna/handy-kontakte/k1.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:3789r1-2B4D\r\nFN:Oma <b>Hilde</b>\r\nTEL;TYPE=HOME:0711 445566\r\nEND:VCARD\r\n")
	now := startNow(t)
	return StartConfig{Contacts: c, Antworten: &kontaktantwort.Store{Path: filepath.Join(t.TempDir(), kontaktantwort.Datei)}, DNS: noDNS{},
		KontaktURL: "https://kontakt.example.org", Loc: berlin(t), Now: func() time.Time { return now }}, dav
}

// frag sends one request as Homes' own page would (anna, same-origin) or, user "", as anyone.
func frag(h http.Handler, method, target, user, groups string, form url.Values) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	if user != "" {
		req.Header.Set("Remote-User", user)
		req.Header.Set("Remote-Groups", groups)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var linkRE = regexp.MustCompile(`https://kontakt\.example\.org/([A-Za-z0-9_-]{43})`)

// startLink makes the link for buch/uid as anna and returns its token.
func startLink(t *testing.T, cfg StartConfig, buch, uid, groups string) string {
	t.Helper()
	rec := frag(Pruefen(cfg), "POST", PruefenPath, "anna", groups, url.Values{"buch": {buch}, "uid": {uid}})
	m := linkRE.FindStringSubmatch(rec.Body.String())
	if rec.Code != 200 || m == nil {
		t.Fatalf("no link made (%d):\n%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") || !strings.HasPrefix(rec.Body.String(), "Hallo ") || !strings.Contains(rec.Body.String(), "nur einmal") {
		t.Fatalf("not the message for the share sheet (%s):\n%s", ct, rec.Body.String())
	}
	return m[1]
}

// antworteAuf decides anna's answer id on the contact's page at pfad.
func antworteAuf(cfg StartConfig, pfad, id string, form url.Values) *httptest.ResponseRecorder {
	form.Set("id", id)
	h := Kontakt(cfg)
	if strings.HasPrefix(pfad, MeineKontaktePath) {
		h = MeineKontakte(cfg)
	}
	return frag(h, "POST", pfad, "anna", "family", form)
}

func formOf(vals map[string][]string) url.Values { return url.Values(vals) }

// owner 2026-11-07: the link shows the contact's own fields and nothing
// else of the house; one answer, then dead; the review before → after;
// only what was taken is written, over the card's other lines.
func TestKontaktPruefenFromTheLinkToTheAddressBook(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	token := startLink(t, cfg, "haushalt", "h1", "family,admins")
	pub := KontaktServer(cfg)

	rec := frag(pub, "GET", "/"+token, "", "", nil)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("form %d", rec.Code)
	}
	mustContain(t, body, "Stimmen deine Kontaktdaten?", `name=vorname value="Anna"`, `name=nachname value="Beispiel"`,
		`value="+49 151 23456789"`, `<option value=mobil selected>`, `value="anna@example.org"`, `name=firma value="Bäckerei"`,
		`action="/`+token+`"`, "Alles stimmt so", "Änderungen senden")
	mustNotContain(t, body, "DE00", "Oma", "/kontakte", "/start", "Abmelden", "<nav")
	for k, want := range map[string]string{"Referrer-Policy": "no-referrer", "Cache-Control": "no-store", "X-Frame-Options": "DENY"} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s: %q, want %q", k, got, want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP %q", csp)
	}

	answer := formOf(map[string][]string{"vorname": {"Anna"}, "nachname": {"Muster"}, "tel": {"+49 151 99999999", "0711 1234567"}, "tel-art": {"mobil", "geschaeftlich"},
		"mail": {"anna@example.org", ""}, "adr-strasse": {"Neuer Weg 2"}, "adr-plz": {"12345"}, "adr-ort": {"Stadt"}, "adr-land": {"Deutschland"},
		"geb-tag": {"12"}, "geb-monat": {"4"}, "geb-jahr": {""}, "firma": {"Bäckerei"}, "position": {"Inhaberin"}, "aktion": {"senden"}})
	if rec := frag(pub, "POST", "/"+token, "", "", answer); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Danke!") {
		t.Fatalf("answer %d:\n%s", rec.Code, rec.Body.String())
	}
	if card := dav.Objects()["/homes/haushalt-kontakte/h1.vcf"]; card != pruefenKarte {
		t.Fatalf("the answer was written before the review:\n%s", card)
	}
	if rec := frag(pub, "GET", "/"+token, "", "", nil); rec.Code != 404 || !strings.Contains(rec.Body.String(), "Link ungültig oder abgelaufen") {
		t.Errorf("an answered link: %d", rec.Code)
	}
	if rec := frag(pub, "POST", "/"+token, "", "", answer); rec.Code != 404 {
		t.Errorf("a second answer: %d", rec.Code)
	}

	open := cfg.Antworten.ZuPruefen("anna")
	if len(open) != 1 {
		t.Fatalf("%d to review", len(open))
	}
	r := httptest.NewRequest("GET", "/aufgaben", nil)
	r.Header.Set("Remote-User", "anna")
	if as := einrichtung(r, cfg); len(as) == 0 || as[0].titel != "1 Kontakt-Antwort prüfen" || as[0].href != AntwortenPath {
		t.Errorf("tasks %+v", as)
	}
	id := open[0].ID
	// owner 2026-11-08: no list of answers; the task opens the contact, its row says one waits, the review is on it
	if rec := frag(Antworten(cfg), "GET", AntwortenPath, "anna", "family", nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/kontakte/h1" {
		t.Errorf("the task's address: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	mustContain(t, frag(Kontakte(cfg), "GET", KontaktePath, "anna", "family", nil).Body.String(), `<a href=/kontakte aria-current=page>Haushalt</a>`,
		`<a href="/kontakte/h1">Anna Beispiel <span class=anzahl aria-label="1 offen">1</span></a>`)
	body = frag(Kontakt(cfg), "GET", "/kontakte/h1", "anna", "family", nil).Body.String()
	mustContain(t, body, `<a href=/kontakte>Kontakte</a> › <span>Anna Beispiel</span></nav>`, "<h1>Anna Beispiel</h1>", "<h2>Antwort auf deinen Link</h2>", "Nachname",
		`<span class=vorher>Beispiel</span> → <span class=nachher>Muster</span>`,
		`<span class=vorher>+49 151 23456789 (mobil)</span> → <span class=nachher>+49 151 99999999 (mobil)</span>`,
		`0711 1234567 (geschäftlich)</span> <span class=k>(neu)`, "Neuer Weg 2, 12345 Stadt, Deutschland", "12.04.", "Inhaberin",
		`<form method=post action="/kontakte/h1"><input type=hidden name=id value="`+id+`">`, "Ausgewählte übernehmen", "Alle übernehmen", "Verwerfen")
	before(t, body, "+49 151 23456789</a>", "<h2>Antwort auf deinen Link</h2>") // the contact first, then what waits
	mustNotContain(t, frag(Kontakt(cfg), "GET", "/kontakte/h1", "ben", "family", nil).Body.String(), "Antwort auf deinen Link")
	if rec := frag(Kontakt(cfg), "POST", "/kontakte/h1", "ben", "family", url.Values{"id": {id}, "aktion": {"alle"}}); !strings.Contains(rec.Body.String(), "Diese Antwort gibt es nicht") {
		t.Errorf("ben decides anna's review: %d", rec.Code)
	}

	// take the family name (0) and the new position (last) only, on the contact's page
	got, _ := cfg.Antworten.Eine(id, "anna")
	last := len(got.Aenderungen) - 1
	rec = antworteAuf(cfg, "/kontakte/h1", id, url.Values{"aktion": {"auswahl"}, "nimm": {"0", strconv.Itoa(last)}})
	mustContain(t, rec.Body.String(), "2 Änderungen ins Adressbuch geschrieben", "<h1>Anna Muster</h1>", "nicht übernommen") // the card as written, each change's outcome
	mustNotContain(t, frag(Kontakte(cfg), "GET", KontaktePath, "anna", "family", nil).Body.String(), "class=anzahl")         // none waits
	card := dav.Objects()["/homes/haushalt-kontakte/h1.vcf"]
	mustContain(t, card, "FN:Anna Muster", "N:Muster;Anna;;;", "TITLE:Inhaberin", "TEL;TYPE=CELL:+49 151 23456789", "X-IBAN:DE00")
	mustNotContain(t, card, "99999999", "Neuer Weg", "BDAY")
	if len(cfg.Antworten.ZuPruefen("anna")) != 0 {
		t.Error("a decided answer is still open")
	}
	for _, secret := range []string{"Beispiel", "Muster", "99999999", "anna@example.org", "Neuer Weg", "Inhaberin", token} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("the log holds %q:\n%s", secret, logs.String())
		}
	}
}

// The one page for every link that is none: never made, malformed, over;
// the same words, the same code. And nothing else of Homes on that listener.
func TestThePublicListenerServesTheFormOnly(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	token := startLink(t, cfg, "privat", "3789r1-2B4D", "family")
	pub := KontaktServer(cfg)
	if rec := frag(pub, "GET", "/"+token, "", "", nil); !strings.Contains(rec.Body.String(), `value="Oma &lt;b&gt;Hilde&lt;/b&gt;"`) && !strings.Contains(rec.Body.String(), "&lt;b&gt;") {
		t.Errorf("the name is not escaped:\n%s", rec.Body.String())
	} else if strings.Contains(rec.Body.String(), "<b>Hilde") {
		t.Error("raw HTML from a card on the public page")
	}
	strip := regexp.MustCompile(`nonce="[^"]*"`)
	var first string
	for _, p := range []string{"/" + strings.Repeat("A", 43), "/x", "/" + token[:42], "/kontakte", "/start", "/napi/kann", "/" + token + "/x", "/" + token + "?a=1", "/healthz"} {
		rec := frag(pub, "GET", p, "anna", "admins", nil)
		b := strip.ReplaceAllString(rec.Body.String(), "")
		if rec.Code != 404 || !strings.Contains(b, "Link ungültig oder abgelaufen") {
			t.Errorf("%s: %d\n%s", p, rec.Code, b)
		}
		if first == "" {
			first = b
		} else if b != first {
			t.Errorf("%s answers differently from a link never made", p)
		}
	}
	if rec := frag(pub, "GET", "/robots.txt", "", "", nil); !strings.Contains(rec.Body.String(), "Disallow: /") {
		t.Errorf("robots.txt: %q", rec.Body.String())
	}
}

func TestThePublicPageRefusesWhatIsNotItsOwnForm(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	token := startLink(t, cfg, "privat", "3789r1-2B4D", "family")
	pub := KontaktServer(cfg)
	req := httptest.NewRequest("POST", "/"+token, strings.NewReader("vorname=X&aktion=senden"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	pub.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Errorf("a cross-site post: %d", rec.Code)
	}
	big := "vorname=" + strings.Repeat("x", 40<<10)
	req = httptest.NewRequest("POST", "/"+token, strings.NewReader(big))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	pub.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("a 40 KB body: %d", rec.Code)
	}
	rec = frag(pub, "POST", "/"+token, "", "", url.Values{"vorname": {"Hilde"}, "geb-tag": {"31"}, "geb-monat": {"2"}, "aktion": {"senden"}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "kein Tag im Kalender") {
		t.Errorf("31.2.: %d\n%s", rec.Code, rec.Body.String())
	}
	if _, err := cfg.Antworten.Zum(token); err != nil {
		t.Errorf("a refused form used the link up: %v", err)
	}
}

func TestThePublicPageIsRateLimited(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	pub := KontaktServer(cfg)
	codes := map[int]int{}
	for range fragePro + 3 {
		req := httptest.NewRequest("GET", "/"+strings.Repeat("B", 43), nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		rec := httptest.NewRecorder()
		pub.ServeHTTP(rec, req)
		codes[rec.Code]++
	}
	if codes[404] != fragePro || codes[http.StatusTooManyRequests] != 3 {
		t.Errorf("codes %v", codes)
	}
	req := httptest.NewRequest("GET", "/"+strings.Repeat("B", 43), nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	rec := httptest.NewRecorder()
	pub.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Errorf("another address is limited too: %d", rec.Code)
	}
}

// Haushalts-Kontakte are an admin's; a person's own Handy-Kontakte theirs,
// and anyone has Meine Kontakte for it, without the Immich key. A refusal
// is one plain sentence. A Handy-Kontakt's accepted changes wait for the
// phone; discarded, none.
func TestWhoMayStartAndWhereAHandyKontaktGoes(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	if rec := frag(Pruefen(cfg), "POST", PruefenPath, "anna", "family", url.Values{"buch": {"haushalt"}, "uid": {"h1"}}); rec.Code != 403 ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(rec.Body.String(), "nur ein Admin") {
		t.Errorf("a family member starts a house contact: %d %s", rec.Code, rec.Body.String())
	}
	if rec := frag(Pruefen(cfg), "POST", PruefenPath, "anna", "family", url.Values{"buch": {"privat"}, "uid": {"weg"}}); rec.Code != 404 {
		t.Errorf("a contact not there: %d", rec.Code)
	}
	req := httptest.NewRequest("POST", PruefenPath, strings.NewReader("buch=privat&uid=3789r1-2B4D"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	Pruefen(cfg).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Errorf("a link made from another site: %d", rec.Code)
	}
	if rec := frag(Pruefen(cfg), "GET", PruefenPath, "anna", "family", nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != KontaktePath {
		t.Errorf("the old start page: %d", rec.Code)
	}
	list := frag(MeineKontakte(cfg), "GET", MeineKontaktePath, "anna", "family", nil).Body.String()
	mustContain(t, list, `<a href=/kontakte/privat aria-current=page>Meine</a>`, `<a href="/kontakte/privat/3789r1-2B4D">Oma &lt;b&gt;Hilde&lt;/b&gt;</a>`,
		`<form method=post action=/kontakte/pruefen class=teilen data-teilen><input type=hidden name=buch value=privat><input type=hidden name=uid value="3789r1-2B4D"><button aria-label="Link teilen: Oma &lt;b&gt;Hilde&lt;/b&gt; prüfen lassen"`)
	seite := frag(MeineKontakte(cfg), "GET", "/kontakte/privat/3789r1-2B4D", "anna", "family", nil).Body.String()
	mustContain(t, seite, `<a href=/kontakte>Kontakte</a> › <a href="/kontakte/privat">Meine</a> › <span>Oma &lt;b&gt;Hilde&lt;/b&gt;</span></nav>`, ">Link teilen</button>", "0711 445566")
	mustNotContain(t, seite, "Fotos") // not the Immich key's owner
	token := startLink(t, cfg, "privat", "3789r1-2B4D", "family")
	frag(KontaktServer(cfg), "POST", "/"+token, "", "", url.Values{"vorname": {"Hilde"}, "nachname": {"Oma"}, "tel": {"0711 445577"}, "tel-art": {"privat"}, "aktion": {"senden"}})
	f := cfg.Antworten.ZuPruefen("anna")[0]
	if rec := frag(Antworten(cfg), "GET", AntwortenPath, "anna", "family", nil); rec.Header().Get("Location") != "/kontakte/privat/3789r1-2B4D" {
		t.Errorf("the task opens %q", rec.Header().Get("Location"))
	}
	rec = antworteAuf(cfg, "/kontakte/privat/3789r1-2B4D", f.ID, url.Values{"aktion": {"alle"}})
	mustContain(t, rec.Body.String(), "gehen an dein Handy")
	if card := dav.Objects()["/anna/handy-kontakte/k1.vcf"]; strings.Contains(card, "445577") {
		t.Error("Homes wrote a Handy-Kontakt itself")
	}
	if fh := cfg.Antworten.FuersHandy("anna"); len(fh) != 1 || fh[0].UID != "3789r1-2B4D" {
		t.Errorf("for the phone: %+v", fh)
	}
}

// owner 2026-11-08: „Link teilen“ stands right under the contact's name,
// for an admin on a house contact, with the path back; the list has the
// search at its top and an icon per row, the same form; no family member
// sees one on a house contact.
func TestKontakteActionsAtTheTop(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	body := frag(Kontakt(cfg), "GET", "/kontakte/h1", "anna", "family,admins", nil).Body.String()
	mustContain(t, body, `<a href=/kontakte>Kontakte</a> › <span>Anna Beispiel</span></nav>`, `data-laeuft="Erstellt den Link …" data-kopiert="Link kopiert" data-nochmal="Jetzt teilen">Link teilen</button>`)
	before(t, body, "<h1>Anna Beispiel</h1><div class=oben>", `>Link teilen</button>`)
	before(t, body, `>Link teilen</button>`, "+49 151 23456789")
	mustNotContain(t, body, "Link erstellen")
	list := frag(Kontakte(cfg), "GET", KontaktePath, "anna", "family,admins", nil).Body.String()
	before(t, list, "id=kontakte-suche", ">Anna Beispiel<")
	before(t, list, ">Anna Beispiel<", `aria-label="Link teilen: Anna Beispiel prüfen lassen"`)
	mustNotContain(t, frag(Kontakte(cfg), "GET", KontaktePath, "anna", "family", nil).Body.String(), "class=teilen data-teilen>")
	mustNotContain(t, frag(Kontakt(cfg), "GET", "/kontakte/h1", "anna", "family", nil).Body.String(), "class=teilen data-teilen>")
}

// An answer whose contact has gone meanwhile is not lost: the task still
// opens its page, with the name and the review, and ends it there.
func TestAntwortOhneKontakt(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	token := startLink(t, cfg, "haushalt", "h1", "family,admins")
	frag(KontaktServer(cfg), "POST", "/"+token, "", "", url.Values{"vorname": {"Anna"}, "nachname": {"Muster"}, "aktion": {"senden"}})
	dav.Put("/homes/haushalt-kontakte/h1.vcf", strings.Replace(pruefenKarte, "UID:h1", "UID:h9", 1)) // h1 is gone
	if rec := frag(Antworten(cfg), "GET", AntwortenPath, "anna", "family", nil); rec.Header().Get("Location") != "/kontakte/h1" {
		t.Fatalf("the task: %q", rec.Header().Get("Location"))
	}
	body := frag(Kontakt(cfg), "GET", "/kontakte/h1", "anna", "family", nil).Body.String()
	mustContain(t, body, "<h1>Anna Beispiel</h1>", "<h2>Antwort auf deinen Link</h2>", ">Verwerfen</button>")
	f := cfg.Antworten.ZuPruefen("anna")[0]
	mustContain(t, antworteAuf(cfg, "/kontakte/h1", f.ID, url.Values{"aktion": {"verwerfen"}}).Body.String(), "Verworfen: nichts wurde geändert.")
	if rec := frag(Kontakt(cfg), "GET", "/kontakte/h1", "anna", "family", nil); rec.Code != http.StatusNotFound || len(cfg.Antworten.ZuPruefen("anna")) != 0 {
		t.Errorf("after: %d", rec.Code)
	}
}

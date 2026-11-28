package pages

import (
	"bytes"
	"log"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/mdopp/homes/internal/teilen"
)

// The public contact pages, held to what a stranger can send (owner 2026-11-28: „da die Form quasi
// öffentlich ist: Injektionen-Schutz ist gegeben, richtig?“). One test per class: each finding's
// fix, and each class that was fine, so it stays fine.

const boese = `"><script>alert(1)</script><img src=x onerror=alert(2)>`

// unsichtbar is a rune that shows nothing (bidi, zero-width), written out so this file holds none.
func unsichtbar(r rune) string { return string(r) }

// HTML: every value a guest sends comes back escaped — on the public form shown again, on the
// review (text and attribute), and the message in its quote; Firma and Position, which have no
// pattern, are where it gets through to the review.
func TestPublicFormEscapesEveryValue(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	token := einladenLink(t, cfg)
	pub := KontaktServer(cfg)
	rec := frag(pub, "POST", "/"+token, "", "", url.Values{"vorname": {boese}, "nachname": {boese}, "tel": {boese}, "mail": {boese}, "adr-strasse": {boese},
		"adr-nr": {boese}, "adr-plz": {boese}, "adr-ort": {boese}, "adr-land": {boese}, "firma": {boese}, "position": {boese}, "nachricht": {boese}, "aktion": {"senden"}})
	body := rec.Body.String()
	mustNotContain(t, body, "<script>alert", "<img src=x")
	mustContain(t, body, `value="&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;`, `<textarea name=nachricht rows=4 maxlength=1000>&#34;&gt;&lt;script&gt;alert(1)`)
	if len(cfg.Antworten.ZuPruefen("anna")) != 0 {
		t.Fatal("a refused form was kept")
	}

	id := neuBeantwortet(t, cfg, url.Values{"vorname": {"Erika"}, "firma": {boese}, "position": {boese}, "nachricht": {boese + "\n</blockquote><script>x()</script>"}})
	body = frag(Antworten(cfg), "GET", AntwortenPath+"/"+id, "anna", "family,admins", nil).Body.String()
	mustNotContain(t, body, "<script>alert", "<img src=x", "</blockquote><script>")
	mustContain(t, body, `<span class=nachher>&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;`, `&lt;/blockquote&gt;&lt;script&gt;x()&lt;/script&gt;`)
}

// Unicode: a bidi override or a zero-width space in a name never reaches the admin's review or
// the address book; the name the review shows is the name without them.
func TestPublicFormDropsInvisibleCharacters(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	vor := "Er" + unsichtbar(0x200b) + "ika" + unsichtbar(0x202e)
	nach := unsichtbar(0x2066) + "Mus" + unsichtbar(0xad) + "ter" + unsichtbar(0x2069)
	id := neuBeantwortet(t, cfg, url.Values{"vorname": {vor}, "nachname": {nach}, "firma": {"A" + unsichtbar(0x200f) + "CME" + unsichtbar(0x2028) + "GmbH"}})
	body := frag(Antworten(cfg), "GET", AntwortenPath+"/"+id, "anna", "family,admins", nil).Body.String()
	mustContain(t, body, "<h1>Neuer Kontakt: Erika Muster</h1>", "<span class=nachher>ACME GmbH</span>")
	frag(Antworten(cfg), "POST", AntwortenPath+"/"+id, "anna", "family,admins", url.Values{"id": {id}, "aktion": {"anlegen"}, "nimm": {"2"}})
	for p, v := range dav.Objects() {
		for _, r := range []rune{0x200b, 0x202e, 0x2066, 0x2069, 0xad, 0x200f, 0x2028} {
			if strings.ContainsRune(v, r) {
				t.Errorf("%s holds U+%04X:\n%s", p, r, v)
			}
		}
	}
}

// vCard: CR, LF, ; , and \ in any field stay inside their property in what Radicale is given — no
// field adds a TEL, a PHOTO or a URL, or ends the card.
func TestPublicAnswerCannotAddCardProperties(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	quer := "x\r\nTEL:+1 555 0100\r\nPHOTO;VALUE=uri:https://evil.example/p.jpg\r\nURL:https://evil.example\r\nEND:VCARD\r\nBEGIN:VCARD\r\nFN:Mallory"
	id := neuBeantwortet(t, cfg, url.Values{"vorname": {"Erika"}, "nachname": {"Muster"}, "tel": {"0170 1234567"}, "firma": {"A;B,C\\" + quer}, "position": {quer},
		"nachricht": {"Hallo\n" + quer}, "adr-strasse": {"Weg 1"}, "adr-plz": {"12345"}, "adr-ort": {"Stadt"}})
	f, _ := cfg.Antworten.Eine(id, "anna")
	alle := url.Values{"id": {id}, "aktion": {"anlegen"}}
	for i := range f.Aenderungen {
		alle.Add("nimm", strconv.Itoa(i))
	}
	mustContain(t, frag(Antworten(cfg), "POST", AntwortenPath+"/"+id, "anna", "family,admins", alle).Body.String(), "Angelegt:")
	card := ""
	for _, v := range dav.Objects() {
		if strings.Contains(v, "FN:Erika Muster") {
			card = v
		}
	}
	if card == "" {
		t.Fatal("no card written")
	}
	props := map[string]int{}
	for _, l := range strings.Split(strings.ReplaceAll(card, "\r\n ", ""), "\r\n") {
		name, _, _ := strings.Cut(l, ":")
		name, _, _ = strings.Cut(name, ";")
		props[name]++
	}
	if props["BEGIN"] != 1 || props["END"] != 1 || props["TEL"] != 1 || props["PHOTO"] != 0 || props["URL"] != 0 || props["FN"] != 1 {
		t.Errorf("the card's properties %v:\n%s", props, card)
	}
	mustContain(t, card, `ORG:A\;B\,C\\x TEL:+1 555 0100 PHOTO\;VALUE=uri:https://evil.example/p.jpg`, `NOTE:Hallo\nx\nTEL:+1 555 0100\nPHOTO`)
}

// CSRF: the public form takes no answer another site posts, the review no decision; nothing is kept or written.
func TestPublicFormAndReviewRefuseOtherSites(t *testing.T) {
	cfg, dav := pruefenCfg(t)
	token := einladenLink(t, cfg)
	pub := KontaktServer(cfg)
	post := func(h http.Handler, path, user string, form url.Values, site, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "https://kontakt.example.org"+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if user != "" {
			req.Header.Set("Remote-User", user)
			req.Header.Set("Remote-Groups", "family,admins")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	answer := url.Values{"vorname": {"Erika"}, "aktion": {"senden"}}
	for _, h := range [][2]string{{"cross-site", ""}, {"same-site", ""}, {"", "https://evil.example"}} {
		if rec := post(pub, "/"+token, "", answer, h[0], h[1]); rec.Code != 404 || len(cfg.Antworten.ZuPruefen("anna")) != 0 {
			t.Errorf("the public form from %v: %d", h, rec.Code)
		}
	}
	id := neuBeantwortet(t, cfg, answer)
	n := len(dav.Objects())
	for _, h := range [][2]string{{"cross-site", ""}, {"", "https://evil.example"}, {"", ""}} {
		if rec := post(Antworten(cfg), AntwortenPath+"/"+id, "anna", url.Values{"id": {id}, "aktion": {"anlegen"}}, h[0], h[1]); rec.Code != 403 {
			t.Errorf("the review from %v: %d", h, rec.Code)
		}
	}
	if len(dav.Objects()) != n || len(cfg.Antworten.ZuPruefen("anna")) != 1 {
		t.Error("a decision from elsewhere was taken")
	}
}

// Headers and brakes: both public pages keep their token to themselves (no-store, no-referrer, a
// script nonce, no frames), and a new person's link is braked as any other: answers from one
// address stop at frageProPost, and a body over the limit is refused.
func TestPublicPagesHeadersAndBrakes(t *testing.T) {
	cfg, _ := pruefenCfg(t)
	token := einladenLink(t, cfg)
	cfg.Teilen = &teilen.Store{}
	_, tt, _, _ := cfg.Teilen.Neu("anna", "Anna")
	pub := KontaktServer(cfg)
	for _, p := range []string{"/" + token, "/t/" + tt} {
		req := httptest.NewRequest("GET", "https://kontakt.example.org"+p, nil)
		rec := httptest.NewRecorder()
		pub.ServeHTTP(rec, req)
		h := rec.Header()
		if rec.Code != 200 || h.Get("Cache-Control") != "no-store" || h.Get("Referrer-Policy") != "no-referrer" || h.Get("X-Frame-Options") != "DENY" ||
			!strings.Contains(h.Get("Content-Security-Policy"), "script-src 'nonce-") || !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Errorf("%s: %d %v", p, rec.Code, h)
		}
	}
	big := httptest.NewRequest("POST", "/"+token, strings.NewReader("vorname="+strings.Repeat("a", frageBytes+1)))
	big.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	pub.ServeHTTP(rec, big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("a body over the limit: %d", rec.Code)
	}
	codes := map[int]int{}
	for range frageProPost + 2 {
		codes[fragVon(pub, "POST", "/"+token, "198.51.100.20")]++
	}
	if codes[http.StatusTooManyRequests] != 2 {
		t.Errorf("a new person's link's answers from one address: %v", codes)
	}
}

// Kontakt austauschen: a sharer's values cannot break the page, the live stream or the .vcf; the
// file name cannot end the header; the Wi-Fi password and the values reach no log line.
func TestTeilenValuesCannotBreakOut(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	cfg, st, _, id, token := teilenCfg(t)
	quer := "\r\nURL:https://evil.example\r\nEND:VCARD\r\n"
	k := &teilen.Karte{Name: "Eve " + boese + unsichtbar(0x202e), Mobil: "+49 170 1" + quer, Telefon: "0711 2", Mail: "a@b.example" + quer, Adresse: "Weg 1;2,3\\\n12345 Ort",
		Firma: strings.Repeat("Ä", 120)}
	if err := st.Setzen(id, "michael", "Mi\"chael\r\nSet-Cookie: x=1", teilen.Freigabe{Karte: k, WLAN: &teilen.WLAN{SSID: boese, Kennwort: "geh\"eim<>&"}}, teilen.Einladung{}); err != nil {
		t.Fatal(err)
	}
	body := teilenGet(cfg, "/t/"+token).Body.String()
	mustNotContain(t, body, "<script>alert", "<img src=x", unsichtbar(0x202e))
	mustContain(t, body, `data-kopie="geh&#34;eim&lt;&gt;&amp;"`, "&lt;script&gt;alert(1)")

	rec := teilenGet(cfg, "/t/"+token+"/karte.vcf")
	cd := rec.Header().Get("Content-Disposition")
	if _, ps, err := mime.ParseMediaType(cd); err != nil || strings.ContainsAny(cd, "\r\n") || ps["filename"] != "MichaelSet-Cookie: x=1.vcf" {
		t.Errorf("Content-Disposition %q: %v %v", cd, ps, err)
	}
	vcf := rec.Body.String()
	props := map[string]int{}
	for _, l := range strings.Split(strings.TrimSuffix(vcf, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Errorf("a line of %d octets: %q", len(l), l)
		}
		if strings.HasPrefix(l, " ") {
			continue
		}
		name, _, _ := strings.Cut(l, ":")
		name, _, _ = strings.Cut(name, ";")
		props[name]++
	}
	if props["BEGIN"] != 1 || props["END"] != 1 || props["URL"] != 0 || props["TEL"] != 2 || props["EMAIL"] != 1 || strings.Contains(vcf, "\r\nURL") {
		t.Errorf("the .vcf's properties %v:\n%s", props, vcf)
	}
	mustContain(t, strings.ReplaceAll(vcf, "\r\n ", ""), `ADR:;;Weg 1\;2\,3\\\, 12345 Ort;;;;`, `EMAIL;TYPE=INTERNET:a@b.example\nURL:https://evil.example\nEND:VCARD`)
	for _, secret := range []string{"geh\"eim", "Eve", "a@b.example", token} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("the log holds %q", secret)
		}
	}
}

// The live stream: whatever a value holds, each event is one data line of JSON (a newline in a
// value cannot end the frame or start a field) and its HTML parts are escaped, since the page's
// script sets them with innerHTML.
func TestTeilenLiveFrameHoldsAnyValue(t *testing.T) {
	cfg, st, _, id, token := teilenCfg(t)
	st.Setzen(id, "michael", "", teilen.Freigabe{Karte: &teilen.Karte{Name: "x\n\ndata: {\"karte\":\"<script>\"}\n\nevent: y", Firma: boese + unsichtbar(0x2028)}}, teilen.Einladung{})
	p := teilenStand(httptest.NewRequest("GET", "/", nil), cfg, token, mustAnsicht(t, st, token))
	mustNotContain(t, p.Karte, "<script>", "<img src=x")
	mustContain(t, p.Karte, "&lt;script&gt;")
	frame := teilenFrame(p)
	if strings.Count(frame, "\n") != 2 || !strings.HasPrefix(frame, "data: {") || !strings.HasSuffix(frame, "}\n\n") {
		t.Errorf("the frame %q", frame)
	}
}

func mustAnsicht(t *testing.T, st *teilen.Store, token string) teilen.Ansicht {
	t.Helper()
	a, _, err := st.Zum(token, false)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// Kontakte's links: a card's number is a tel: of its digits and an address a mailto: whose ? and #
// are encoded, so a value a contact typed adds no cc, subject or body.
func TestKontaktLinksCarryOnlyTheValue(t *testing.T) {
	if got := mailHref("anna?cc=eve@evil.example&body=x#y@example.org"); got != "mailto:anna%3Fcc=eve@evil.example&body=x%23y@example.org" {
		t.Errorf("mailHref: %s", got)
	}
	if got := mailHref("anna@example.org"); got != "mailto:anna@example.org" {
		t.Errorf("mailHref: %s", got)
	}
	if got := telHref("+49 (0171) 123-45;ext=9?x"); got != "tel:+490171123459" {
		t.Errorf("telHref: %s", got)
	}
}

package pages

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/zugang"
)

// teaserCfg is the public listener with both hosts: the teaser on
// box.example.org, Kontakt prüfen lassen on kontakt.example.org, Laib's
// tell recorded, the clock held.
func teaserCfg(t *testing.T) (StartConfig, func() []string) {
	t.Helper()
	cfg, _ := pruefenCfg(t)
	var told []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Name string         `json:"name"`
				Args map[string]any `json:"arguments"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Params.Name == "laib_tell" {
			mu.Lock()
			told = append(told, req.Params.Args["person"].(string)+": "+req.Params.Args["text"].(string))
			mu.Unlock()
		}
		json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"content": []map[string]string{{"text": `{"sent":true}`}}}})
	}))
	t.Cleanup(srv.Close)
	cfg.Laib = &laib.Client{URL: srv.URL, Token: "t"}
	cfg.TeaserHost, cfg.TeaserPerson, cfg.PublicURL = "box.example.org", "mama", "https://www.example.org"
	cfg.Zugang = &zugang.Store{Path: filepath.Join(t.TempDir(), zugang.Datei), Now: cfg.Now}
	return cfg, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), told...) }
}

// zugang posts the form from ip as a browser on the teaser host does.
func zugangPost(h http.Handler, ip string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "https://box.example.org/zugang", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Forwarded-For", ip)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func hol(h http.Handler, target string) *httptest.ResponseRecorder {
	return holVon(h, target, "")
}

// holVon is a GET as Caddy passes it on for a client at ip ("" none).
func holVon(h http.Handler, target, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", target, nil)
	if ip != "" {
		req.Header.Set("X-Forwarded-For", ip)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func anfrage(name, kontakt, nachricht string) url.Values {
	return url.Values{"name": {name}, "kontakt": {kontakt}, "nachricht": {nachricht}}
}

// Owner 2026-11-08: the domain itself shows the teaser, the kontakt host its
// pages as before, any other host nothing; the teaser may be indexed, the
// form not; nothing but the page, the form and robots.txt is there.
func TestTeaserDispatchesByHost(t *testing.T) {
	cfg, _ := teaserCfg(t)
	pub := KontaktServer(cfg)
	rec := holVon(pub, "https://box.example.org/", "192.168.178.20")
	if rec.Code != 200 {
		t.Fatalf("the teaser: %d\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	mustContain(t, body, `<!doctype html><html lang="de"><head><meta charset="utf-8">`, "<title>Die Box</title>", "<h1>Die Box</h1>", `<form method="post" action="/zugang">`,
		`name="name"`, `name="kontakt"`, `name="nachricht"`, `name="website"`, `href="https://www.example.org/"`, "Zugang anfragen", "</body></html>")
	mustNotContain(t, body, `<template id="zugang-danke">`, `<template id="zugang-draussen">`, "{{ANMELDEN}}", "claude:", "dopp.cloud", "https://fonts.", "<script")
	h := rec.Header()
	if h.Get("X-Robots-Tag") != "" || !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'; style-src 'unsafe-inline'; img-src") || h.Get("Cache-Control") != "private, no-cache" ||
		!strings.Contains(h.Get("Content-Security-Policy"), "form-action 'self'; frame-ancestors 'none'") || h.Get("X-Frame-Options") != "DENY" {
		t.Errorf("headers %v", h)
	}
	if rec := hol(pub, "https://box.example.org:443/"); rec.Code != 200 {
		t.Errorf("the host with its port: %d", rec.Code)
	}
	if rec := hol(pub, "https://BOX.example.org/robots.txt"); rec.Code != 200 || rec.Body.String() != "User-agent: *\nAllow: /\nDisallow: /zugang\n" {
		t.Errorf("robots.txt: %d %q", rec.Code, rec.Body.String())
	}
	if rec := hol(pub, "https://box.example.org/zugang"); rec.Code != 303 || rec.Header().Get("Location") != "/" {
		t.Errorf("GET /zugang: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	for _, p := range []string{"/start", "/napi/box", "/abc", "/.env"} {
		if rec := hol(pub, "https://box.example.org"+p); rec.Code != 404 || !strings.Contains(rec.Body.String(), "Diese Seite gibt es nicht.") || rec.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
			t.Errorf("%s: %d %q", p, rec.Code, rec.Header().Get("X-Robots-Tag"))
		}
	}
	// the kontakt host answers as before: its robots say no, a token that is none is its page
	if rec := hol(pub, "https://kontakt.example.org/robots.txt"); rec.Body.String() != "User-agent: *\nDisallow: /\n" {
		t.Errorf("kontakt's robots.txt: %q", rec.Body.String())
	}
	if rec := hol(pub, "https://kontakt.example.org/keintoken"); rec.Code != 404 || !strings.Contains(rec.Body.String(), "Link ungültig") {
		t.Errorf("kontakt's page: %d", rec.Code)
	}
	// any other host, nothing
	for _, h := range []string{"https://www.example.org/", "https://example.org/", "https://198.51.100.4/", "https://kontakt.example.org.evil.net/"} {
		if rec := hol(pub, h); rec.Code != 404 || strings.Contains(rec.Body.String(), "<form") {
			t.Errorf("%s: %d", h, rec.Code)
		}
	}
	// without a teaser host (HOMES_TEASER_HOST unset) the listener is kontakt's alone, on any Host
	cfg.TeaserHost = ""
	if rec := hol(KontaktServer(cfg), "https://box.example.org/"); rec.Code != 404 || !strings.Contains(rec.Body.String(), "Link ungültig") {
		t.Errorf("teaser off: %d", rec.Code)
	}
}

// A request: checked field by field with the words kept, a bot's dropped
// with thanks, kept in HOMES_DATA with a hash of the address, told to the
// notify person, thanked in the form's place; the admins see it on Start
// and tick it off on /zugang.
func TestZugangAnfragen(t *testing.T) {
	cfg, told := teaserCfg(t)
	pub := KontaktServer(cfg)
	// validation, each with the words kept and the one line before the button
	for _, c := range []struct {
		form url.Values
		says string
	}{
		{anfrage("", "0711 1", ""), "Sag uns bitte, wie du heißt"},
		{anfrage(strings.Repeat("a", 81), "0711 1", ""), "Sag uns bitte, wie du heißt"},
		{anfrage("Oma", "  ", ""), "Wie erreichen wir dich?"},
		{anfrage("Oma", "0711 1", strings.Repeat("n", 301)), "höchstens 300 Zeichen"},
	} {
		rec := zugangPost(pub, "192.168.178.21", c.form)
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), `<p class="fehler" role="alert">`) || !strings.Contains(rec.Body.String(), c.says) || rec.Header().Get("X-Robots-Tag") != "noindex, nofollow" {
			t.Errorf("%v: %d, no %q", c.form, rec.Code, c.says)
		}
		if n := c.form.Get("name"); n != "" && len(n) < 80 && !strings.Contains(rec.Body.String(), `name="name" value="`+n+`"`) {
			t.Errorf("%v: the name is not kept", c.form)
		}
	}
	if offen, _ := cfg.Zugang.Offen(); len(offen) != 0 {
		t.Fatalf("a refused form was kept: %+v", offen)
	}
	// a POST from elsewhere
	req := httptest.NewRequest("POST", "https://box.example.org/zugang", strings.NewReader(anfrage("Oma", "0711 1", "").Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("X-Forwarded-For", "192.168.178.21")
	rec := httptest.NewRecorder()
	pub.ServeHTTP(rec, req)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "von dieser Seite aus") {
		t.Errorf("cross-site: %d", rec.Code)
	}
	// the honeypot: thanked, dropped
	f := anfrage("Bot", "bot@example.org", "")
	f.Set("website", "https://spam.example")
	if rec := zugangPost(pub, "10.0.0.2", f); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Danke, deine Anfrage ist angekommen.") {
		t.Errorf("honeypot: %d", rec.Code)
	}
	if offen, _ := cfg.Zugang.Offen(); len(offen) != 0 {
		t.Fatalf("the bot's request was kept: %+v", offen)
	}
	// a real one
	rec = zugangPost(pub, "192.168.178.21", anfrage(" Oma  Hilde ", "0711 445566", "Ich bin die Oma.\nRuft an."))
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v\n%s", rec.Code, rec.Header(), rec.Body.String())
	}
	mustContain(t, rec.Body.String(), `<p class="danke" role="status">`, "Danke, deine Anfrage ist angekommen.", "<h1>Die Box</h1>")
	mustNotContain(t, rec.Body.String(), `<form method="post" action="/zugang">`, "<template")
	offen, err := cfg.Zugang.Offen()
	if err != nil || len(offen) != 1 || offen[0].Name != "Oma Hilde" || offen[0].Kontakt != "0711 445566" || offen[0].Nachricht != "Ich bin die Oma.\nRuft an." || !offen[0].Zeit.Equal(cfg.Now()) {
		t.Fatalf("%v %+v", err, offen)
	}
	b, _ := os.ReadFile(cfg.Zugang.Path)
	if strings.Contains(string(b), "192.168.178") || !strings.Contains(string(b), `"adresse"`) {
		t.Errorf("the file:\n%s", b)
	}
	// told to the notify person, in the background
	deadline := time.Now().Add(3 * time.Second)
	for len(told()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if want := "mama: Zugangsanfrage von Oma Hilde (0711 445566): Ich bin die Oma.\nRuft an."; len(told()) != 1 || told()[0] != want {
		t.Errorf("told %q, want %q", told(), want)
	}
	// Braucht dich for an admin: the row; for the family nothing
	req = httptest.NewRequest("GET", "/start", nil)
	req.Header.Set("Remote-User", "anna")
	req.Header.Set("Remote-Groups", "family,admins")
	start := brauchtDich(req, cfg, laibTeil{})
	mustContain(t, start, "Zugangsanfrage von Oma Hilde", "0711 445566", `href="/zugang"`)
	req.Header.Set("Remote-Groups", "family")
	mustNotContain(t, brauchtDich(req, cfg, laibTeil{}), "Zugangsanfrage")
	// the admins' page, and Erledigt
	page := Zugang(cfg)
	if rec := frag(page, "GET", ZugangPath, "anna", "family", nil); rec.Code != 303 {
		t.Errorf("the family on /zugang: %d", rec.Code)
	}
	rec = frag(page, "GET", ZugangPath, "anna", "family,admins", nil)
	mustContain(t, rec.Body.String(), "<h1>Zugangsanfragen</h1>", "<b>Oma Hilde</b> · 0711 445566", "Ich bin die Oma.<br>Ruft an.", "14.10.2026 10:00", `name=id value="`+offen[0].ID+`"`, ">Erledigt</button>")
	rec = frag(page, "POST", ZugangPath, "anna", "family,admins", url.Values{"id": {offen[0].ID}})
	if rec.Code != 303 || rec.Header().Get("Location") != ZugangPath {
		t.Fatalf("Erledigt: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if offen, _ := cfg.Zugang.Offen(); len(offen) != 0 {
		t.Errorf("still open: %+v", offen)
	}
	mustContain(t, frag(page, "GET", ZugangPath, "anna", "family,admins", nil).Body.String(), "Gerade fragt niemand nach Zugang.")
	mustNotContain(t, brauchtDich(req, cfg, laibTeil{}), "Zugangsanfrage")
}

// One accepted request per address an hour, twenty a day for everyone:
// over it the same page with the calm line, 429, and nothing kept or told.
func TestZugangThrottles(t *testing.T) {
	cfg, told := teaserCfg(t)
	now := cfg.Now()
	cfg.Now = func() time.Time { return now }
	cfg.Zugang.Now = cfg.Now
	pub := KontaktServer(cfg)
	if rec := zugangPost(pub, "192.168.178.21", anfrage("Oma", "0711 1", "")); rec.Code != 200 {
		t.Fatalf("the first: %d", rec.Code)
	}
	rec := zugangPost(pub, "192.168.178.21", anfrage("Oma", "0711 1", "noch einmal"))
	if rec.Code != 429 || !strings.Contains(rec.Body.String(), `<p class="fehler" role="alert">Gerade geht es nicht, versuch es später noch einmal.</p>`) || rec.Header().Get("Retry-After") != "3600" {
		t.Errorf("the second from one address: %d %v\n%s", rec.Code, rec.Header(), rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `>noch einmal</textarea>`) {
		t.Error("the words are not kept")
	}
	for i := 2; i <= zugang.ProTag; i++ {
		now = now.Add(time.Second)
		if rec := zugangPost(pub, "10.0.1."+strconv.Itoa(i), anfrage("N", "k", "")); rec.Code != 200 {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	if rec := zugangPost(pub, "fd00::9", anfrage("N", "k", "")); rec.Code != 429 {
		t.Errorf("the 21st of the day: %d", rec.Code)
	}
	offen, _ := cfg.Zugang.Offen()
	if len(offen) != zugang.ProTag {
		t.Errorf("%d kept, want %d", len(offen), zugang.ProTag)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(told()) < zugang.ProTag && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(told()); n != zugang.ProTag {
		t.Errorf("told %d times, want %d", n, zugang.ProTag)
	}
}

// Owner 2026-11-12, after spam: the form is shown and taken only at home,
// as Caddy saw the client (X-Forwarded-For's last entry, which Caddy sets);
// outside the one sentence instead, a POST refused with it, nothing kept or
// told, and no header a client sends makes it home.
func TestZugangNurZuhause(t *testing.T) {
	cfg, told := teaserCfg(t)
	pub := KontaktServer(cfg)
	const draussen = "Zugang fragst du bei uns persönlich an, oder auf dieser Seite, wenn du bei uns zuhause bist."
	for _, ip := range []string{"192.168.178.20", "10.1.2.3", "172.16.0.9", "fd12:3456::1", "fe80::1", "169.254.1.1", "127.0.0.1", "::ffff:192.168.1.5"} {
		body := holVon(pub, "https://box.example.org/", ip).Body.String()
		if !strings.Contains(body, `<form method="post" action="/zugang">`) || strings.Contains(body, draussen) {
			t.Errorf("at home from %s: no form", ip)
		}
	}
	for _, ip := range []string{"", "203.0.113.7", "2001:db8::1", "100.64.0.1", "unknown", "192.168.1.5, 203.0.113.7"} {
		rec := holVon(pub, "https://box.example.org/", ip)
		mustContain(t, rec.Body.String(), "<h1>Die Box</h1>", "Welche Daten wir annehmen", "Keine Cookies, nichts von fremden Servern", `<p class="draussen">`+draussen+"</p>", "Anmelden")
		mustNotContain(t, rec.Body.String(), "<form", `name="kontakt"`, "Anfrage schicken", "Ein Mensch aus der Familie", "<template")
		if rec.Code != 200 {
			t.Errorf("outside from %q: %d", ip, rec.Code)
		}
	}
	// a POST from outside, with what a client might send to look at home
	for _, spoof := range []map[string]string{
		{"X-Forwarded-For": "203.0.113.7"},
		{"X-Forwarded-For": "192.168.1.5, 203.0.113.7"},
		{"X-Forwarded-For": "203.0.113.7", "X-Real-IP": "192.168.1.5", "Forwarded": "for=192.168.1.5"},
	} {
		req := httptest.NewRequest("POST", "https://box.example.org/zugang", strings.NewReader(anfrage("Spam", "spam@example.org", "Hallo").Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		for k, v := range spoof {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		pub.ServeHTTP(rec, req)
		if rec.Code != 403 || !strings.Contains(rec.Body.String(), draussen) || strings.Contains(rec.Body.String(), "<form") || strings.Contains(rec.Body.String(), "Danke") {
			t.Errorf("POST from outside %v: %d\n%s", spoof, rec.Code, rec.Body.String())
		}
	}
	if _, err := os.Stat(cfg.Zugang.Path); err == nil || len(told()) != 0 {
		t.Fatalf("a request from outside was kept (%v) or told %q", err, told())
	}
	// the same request from home is taken
	if rec := zugangPost(pub, "192.168.178.20", anfrage("Oma", "0711 1", "")); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Danke, deine Anfrage ist angekommen.") {
		t.Errorf("from home: %d", rec.Code)
	}
	if offen, _ := cfg.Zugang.Offen(); len(offen) != 1 || offen[0].Name != "Oma" {
		t.Errorf("kept %+v", offen)
	}
}

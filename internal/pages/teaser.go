package pages

import (
	"context"
	_ "embed"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/texte"
	"github.com/mdopp/homes/internal/zugang"
)

// Teaser is the domain itself for a visitor without an account (owner
// 2026-11-08): one static page that says what the box is, „Anmelden“ to
// Homes, and „Zugang anfragen“, a request a person reads — never a
// registration. It shares the public listener with Kontakt prüfen lassen
// (KontaktServer tells the two apart by Host), so it is behind no login and
// trusts no header. The page is teaser.html, a file of its own so a word is
// changed without touching code; Homes wraps it, fills the login link and
// swaps the form for its thanks. A request is kept (zugang.Store), told to
// the box's notify person over Laib, and shown to the admins on Start; only
// from the house's own network (owner 2026-11-12, after spam).

//go:embed teaser.html
var teaserHTML string

// TeaserPath is the request's path; ZugangPath the admins' page behind the login.
const (
	TeaserPath = "/zugang"
	ZugangPath = "/zugang"
)

const teaserCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

// Teaser serves the teaser host: GET / the page, POST /zugang the request,
// robots.txt allows the page and not the form; everything else is 404.
func Teaser(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", teaserCSP)
		switch {
		case r.URL.Path == "/robots.txt":
			h.Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("User-agent: *\nAllow: /\nDisallow: /zugang\n"))
		case r.URL.Path == "/favicon.ico":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			h.Set("Cache-Control", "private, no-cache") // the form at home, the sentence outside
			teaserSeite(w, http.StatusOK, cfg, map[bool]string{true: teaserForm(zugang.Anfrage{}, ""), false: teaserVorlage("zugang-draussen")}[zuhause(r)])
		case r.URL.Path == TeaserPath && r.Method == http.MethodPost:
			h.Set("Cache-Control", "no-store")
			h.Set("X-Robots-Tag", "noindex, nofollow")
			if !zuhause(r) {
				teaserSeite(w, http.StatusForbidden, cfg, teaserVorlage("zugang-draussen"))
				return
			}
			zugangAnfragen(w, r, cfg)
		case r.URL.Path == TeaserPath:
			http.Redirect(w, r, "/", http.StatusSeeOther)
		default:
			h.Set("X-Robots-Tag", "noindex, nofollow")
			teaserSeite(w, http.StatusNotFound, cfg, `<p class="hinweis">`+html.EscapeString(texte.T(texte.ZugangNichtDa))+` <a href="/">Zur Seite</a></p>`)
		}
	}
}

// zugangAnfragen takes one request: checked, throttled by the store, kept,
// told to the notify person (in the background, the visitor waits for no
// Signal), and thanked. A filled honeypot is thanked too and dropped.
func zugangAnfragen(w http.ResponseWriter, r *http.Request, cfg StartConfig) {
	if fremd(r) {
		teaserSeite(w, http.StatusForbidden, cfg, teaserForm(zugang.Anfrage{}, texte.T(texte.ZugangFremd)))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, frageBytes)
	if r.ParseForm() != nil || cfg.Zugang == nil {
		teaserSeite(w, http.StatusBadRequest, cfg, teaserForm(zugang.Anfrage{}, texte.T(texte.ZugangSpaeter)))
		return
	}
	f := r.PostForm
	a, fehler := zugang.Pruefen(f.Get("name"), f.Get("kontakt"), f.Get("nachricht"))
	if fehler != "" {
		says := map[zugang.Fehler]texte.ID{zugang.OhneName: texte.ZugangOhneName, zugang.OhneKontakt: texte.ZugangOhneKontakt, zugang.ZuLang: texte.ZugangZuLang}[fehler]
		teaserSeite(w, http.StatusUnprocessableEntity, cfg, teaserForm(a, texte.T(says)))
		return
	}
	if f.Get("website") != "" { // the field no person sees: a bot filled it
		log.Printf("homes: zugang: a request with the hidden field filled, dropped")
		teaserSeite(w, http.StatusOK, cfg, teaserDanke())
		return
	}
	a, err := cfg.Zugang.Annehmen(a, clientIP(r))
	if err != nil {
		code := http.StatusTooManyRequests
		if err != zugang.ErrZuViele {
			code = http.StatusInternalServerError
			log.Printf("homes: zugang: not kept: %v", err)
		} else {
			w.Header().Set("Retry-After", "3600")
		}
		teaserSeite(w, code, cfg, teaserForm(a, texte.T(texte.ZugangSpaeter)))
		return
	}
	log.Printf("homes: zugang: request %s kept", a.ID)
	if cfg.Laib != nil && cfg.TeaserPerson != "" {
		go zugangMelden(cfg, a)
	}
	teaserSeite(w, http.StatusOK, cfg, teaserDanke())
}

// zugangMelden tells the notify person (box.yml notify.person, HOMES_TEASER_PERSON) over Laib.
func zugangMelden(cfg StartConfig, a zugang.Anfrage) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	text := ""
	if a.Nachricht != "" {
		text = ": " + a.Nachricht
	}
	if err := cfg.Laib.Tell(ctx, cfg.TeaserPerson, fmt.Sprintf(texte.T(texte.ZugangMeldung), a.Name, a.Kontakt, text)); err != nil {
		log.Printf("homes: zugang: telling %s about request %s: %v", cfg.TeaserPerson, a.ID, err)
	}
}

// zuhause is a request from the house's own network as Caddy saw it
// (clientIP: Caddy sets X-Forwarded-For, never a client): private
// (fc00::/7 too), loopback or link-local; via the public address it is not.
func zuhause(r *http.Request) bool {
	ip := net.ParseIP(clientIP(r))
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())
}

// teaserForm is the page's request section with the visitor's words kept
// and, problem set, the one line that says what to change before the button.
func teaserForm(a zugang.Anfrage, problem string) string {
	form := teaserPart(`<section class="zugang"`, "</section>")
	form = strings.Replace(form, `name="name"`, `name="name" value="`+html.EscapeString(a.Name)+`"`, 1)
	form = strings.Replace(form, `name="kontakt"`, `name="kontakt" value="`+html.EscapeString(a.Kontakt)+`"`, 1)
	form = strings.Replace(form, `></textarea>`, `>`+html.EscapeString(a.Nachricht)+`</textarea>`, 1)
	if problem != "" {
		form = strings.Replace(form, `<button type="submit">`, `<p class="fehler" role="alert">`+html.EscapeString(problem)+`</p><button type="submit">`, 1)
	}
	return form
}

// teaserDanke is the section with the thanks in the form's place.
func teaserDanke() string {
	return strings.Replace(teaserPart(`<section class="zugang"`, "</section>"), teaserPart(`<form method="post" action="/zugang">`, "</form>"), teaserVorlage("zugang-danke"), 1)
}

// teaserVorlage is what the file carries as template id: the thanks, the sentence outside.
func teaserVorlage(id string) string {
	return strings.TrimSuffix(strings.TrimPrefix(teaserPart(`<template id="`+id+`">`, "</template>"), `<template id="`+id+`">`), "</template>")
}

// teaserPart is the file from the first from to the end of to, inclusive.
func teaserPart(from, to string) string {
	i := strings.Index(teaserHTML, from)
	j := strings.Index(teaserHTML[i:], to) + len(to)
	return teaserHTML[i : i+j]
}

// teaserSeite writes the page with teil in the request section's place:
// the file's head and main, wrapped, the login link filled, the templates after main left out.
func teaserSeite(w http.ResponseWriter, code int, cfg StartConfig, teil string) {
	head, body, _ := strings.Cut(teaserHTML, "<main>")
	body, _, _ = strings.Cut(strings.Replace("<main>"+body, teaserPart(`<section class="zugang"`, "</section>"), teil, 1), "</main>")
	body = strings.ReplaceAll(body, "{{ANMELDEN}}", html.EscapeString(strings.TrimRight(cfg.PublicURL, "/")+"/"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	w.Write([]byte(`<!doctype html><html lang="de"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">` + head + "</head><body>" + body + "</main></body></html>"))
}

// Zugang is the admins' page behind the login: the open requests, each
// with „Erledigt“; POST ticks one off and comes back here.
func Zugang(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !adminOnly(w, r) {
			return
		}
		if cfg.Zugang == nil {
			missing{http.StatusNotFound, saysNoSuchPage, "Box", "/box", "Zur Box"}.write(w, r, cfg)
			return
		}
		if r.Method == http.MethodPost {
			if !fromHomes(r) {
				http.Error(w, "Das geht nur von Homes' eigener Seite.", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
			r.ParseForm()
			if err := cfg.Zugang.Erledigen(r.PostForm.Get("id")); err != nil {
				http.Error(w, familySentence("zugang", err, texte.T(texte.FrageNicht)), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, ZugangPath, http.StatusSeeOther)
			return
		}
		offen, err := cfg.Zugang.Offen()
		b := adminHead(r, cfg, texte.T(texte.ZugangAnfragen))
		b.WriteString("<h1>" + texte.T(texte.ZugangAnfragen) + "</h1><p class=k>" + html.EscapeString(texte.T(texte.ZugangHinweis)) + "</p>")
		switch {
		case err != nil:
			log.Printf("homes: zugang: %v", err)
			b.WriteString("<p>" + html.EscapeString(texte.T(texte.FrageNicht)) + "</p>")
		case len(offen) == 0:
			b.WriteString("<p>" + html.EscapeString(texte.T(texte.ZugangKeine)) + "</p>")
		}
		_, loc := cfg.clock()
		if len(offen) > 0 {
			b.WriteString("<ul class=zeilen>")
		}
		for _, a := range offen {
			b.WriteString(`<li><div><b>` + html.EscapeString(a.Name) + "</b> · " + html.EscapeString(a.Kontakt) + `<br><span class=k>` + a.Zeit.In(loc).Format("02.01.2006 15:04") + "</span>")
			if a.Nachricht != "" {
				b.WriteString("<br>" + strings.ReplaceAll(html.EscapeString(a.Nachricht), "\n", "<br>"))
			}
			b.WriteString(`</div><form method=post action="` + ZugangPath + `" data-ganz><input type=hidden name=id value="` + html.EscapeString(a.ID) + `"><button data-laeuft="Erledigt …">` + texte.T(texte.ZugangErledigt) + "</button></form></li>")
		}
		if len(offen) > 0 {
			b.WriteString("</ul>")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(b.String()))
	}
}

// zugangRows are Braucht dich's rows for an admin: one per open request,
// leading to the page; a store that cannot be read is nothing to show.
func zugangRows(cfg StartConfig) [][2]string {
	if cfg.Zugang == nil {
		return nil
	}
	offen, err := cfg.Zugang.Offen()
	if err != nil {
		log.Printf("homes: zugang: %v", err)
	}
	var rows [][2]string
	for _, a := range offen {
		rows = append(rows, [2]string{fmt.Sprintf(texte.T(texte.ZugangAnfrage), a.Name), a.Kontakt})
	}
	return rows
}

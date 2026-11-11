package pages

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"slices"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/texte"
)

// „Wer ist das?“ (owner 2026-11-09): on „Dieselbe Person?“, up to five faces Immich sees often and
// knows no name for (gesichter.Wer), each with its photos, places and company, a name field (it suggests
// the household's people and the owner's Handy-Kontakte, firms not) with Speichern (to Immich) and Kenne ich nicht (never asked again).
// The same card, „Wer ist das dann?“, for the unnamed face of a pair just answered Nein (owner 2026-11-11).

// werKarten are the faces to name, posted to action: dann (werDann) under
// „Wer ist das dann?“, then wer under „Wer ist das?“; "" without one.
func werKarten(r *http.Request, cfg StartConfig, dann, wer []gesichter.Seite, action string) string {
	wer = slices.DeleteFunc(slices.Clone(wer), func(s gesichter.Seite) bool {
		return slices.ContainsFunc(dann, func(d gesichter.Seite) bool { return d.ID == s.ID })
	})
	if len(dann)+len(wer) == 0 {
		return ""
	}
	t := func(id texte.ID) string { return html.EscapeString(texte.T(id)) }
	var b strings.Builder
	var all, eigene []caldav.Contact
	if cfg.Contacts != nil { // the page is the key owner's alone (privatEigen): their Handy-Kontakte too (owner 2026-11-10)
		fetch(r, func(ctx context.Context) { all, _, _ = cfg.Contacts.Contacts(ctx) },
			func(ctx context.Context) { eigene, _ = meineKontakte(ctx, cfg, r.Header.Get("Remote-User")) })
	}
	seen := map[string]bool{"": true}
	for _, c := range append(all, eigene...) {
		if !seen[c.Name] && !c.IsFirm() {
			seen[c.Name] = true
			b.WriteString(`<option value="` + html.EscapeString(c.Name) + `">`)
		}
	}
	b.WriteString("</datalist>")
	if len(dann) > 0 {
		b.WriteString("<h2>" + t(texte.WerDann) + "</h2><p class=k>" + t(texte.WerDannHinweis) + "</p>" + werKarte(cfg, dann[0], action))
	}
	if len(wer) > 0 {
		b.WriteString("<h2>" + t(texte.WerIstDas) + "</h2><p class=k>" + t(texte.WerHinweis) + "</p>")
	}
	for _, s := range wer {
		b.WriteString(werKarte(cfg, s, action))
	}
	return "<datalist id=wernamen>" + b.String()
}

// werKarte is one face to name, its name field and two buttons, posted to action.
func werKarte(cfg StartConfig, s gesichter.Seite, action string) string {
	t := func(id texte.ID) string { return html.EscapeString(texte.T(id)) }
	return `<section class="paar wer">` + dieselbeGesicht(cfg, s) + `<form method=post action="` + html.EscapeString(action) +
		`"><input type=hidden name=wer value="` + html.EscapeString(s.ID) + `"><input name=name list=wernamen required maxlength=120 autocomplete=off aria-label="` + t(texte.WerName) + `" placeholder="` + t(texte.WerName) +
		`"><button name=antwort value=ja data-laeuft="` + t(texte.WerSpeichernLaeuft) + `">` + t(texte.WerSpeichern) +
		`</button><button name=antwort value=nein class=leise formnovalidate data-laeuft="` + t(texte.WerUnbekanntLauft) + `">` + t(texte.WerUnbekannt) + `</button></form></section>`
}

// werDann is the unnamed face of the pair just answered Nein, or of a name
// for it just refused, to name right there; none after anything else.
func werDann(r *http.Request, cfg StartConfig, problem string) []gesichter.Seite {
	f, id := r.PostForm, ""
	switch {
	case f.Get("wer") != "" && problem != "":
		id = f.Get("wer")
	case f.Get("antwort") == "nein" && f.Get("a") != "" && f.Get("kontakt") == "" && problem == "":
		id = f.Get("b")
	}
	if s, ok := cfg.Gesichter.Dann(id); ok {
		return []gesichter.Seite{s}
	}
	return nil
}

// werGesagt are the answers about a face as the page says them.
var werGesagt = map[string]texte.ID{"ja": texte.WerGenannt, "nein": texte.WerGemerkt, "stumm": texte.WerStumm}

// werAntwort passes the owner's answer about the face id on: "ja" with
// the name typed, "nein" „Kenne ich nicht“.
func werAntwort(r *http.Request, cfg StartConfig, id string) (said, problem string) {
	antwort, name := r.PostForm.Get("antwort"), trimRunes(oneLine(r.PostForm.Get("name")), 120)
	if antwort != "ja" {
		antwort, name = "nein", ""
	} else if name == "" {
		return "", texte.T(texte.WerOhneName)
	}
	doch, genannt := false, ""
	said, problem = dieselbeAntworten(r.Context(), antwort, func(ctx context.Context) (string, error) {
		var err error
		genannt, doch, err = cfg.Gesichter.Benennen(ctx, id, name)
		return genannt, err
	}, werGesagt)
	if doch && problem == "" { // the named side's own name after a Nein: the same person after all, said so
		said = fmt.Sprintf(texte.T(texte.WerDoch), genannt)
	}
	return said, problem
}

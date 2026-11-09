package pages

import (
	"context"
	"html"
	"net/http"
	"strings"

	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/texte"
)

// „Wer ist das?“ (owner 2026-11-09): on „Dieselbe Person?“, up to five
// faces Immich sees often and knows no name for (gesichter.Wer), each with
// its photos, places and company, a name field with Speichern (the name
// goes to Immich) and Kenne ich nicht (never asked again).

// werKarten is the section, posted to action; "" without a face to name.
func werKarten(cfg StartConfig, wer []gesichter.Seite, action string) string {
	if len(wer) == 0 {
		return ""
	}
	t := func(id texte.ID) string { return html.EscapeString(texte.T(id)) }
	var b strings.Builder
	b.WriteString("<h2>" + t(texte.WerIstDas) + "</h2><p class=k>" + t(texte.WerHinweis) + "</p>")
	for _, s := range wer {
		b.WriteString(`<section class="paar wer">` + dieselbeGesicht(cfg, s) + `<form method=post action="` + html.EscapeString(action) +
			`"><input type=hidden name=wer value="` + html.EscapeString(s.ID) + `"><input name=name required maxlength=120 autocomplete=off aria-label="` + t(texte.WerName) + `" placeholder="` + t(texte.WerName) +
			`"><button name=antwort value=ja data-laeuft="` + t(texte.WerSpeichernLaeuft) + `">` + t(texte.WerSpeichern) +
			`</button><button name=antwort value=nein class=leise formnovalidate data-laeuft="` + t(texte.WerUnbekanntLauft) + `">` + t(texte.WerUnbekannt) + `</button></form></section>`)
	}
	return b.String()
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
	return dieselbeAntworten(r.Context(), antwort, func(ctx context.Context) (string, error) { return cfg.Gesichter.Benennen(ctx, id, name) }, werGesagt)
}

package pages

import (
	"cmp"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/sprachmodell"
	"github.com/mdopp/homes/internal/texte"
)

// SprachmodellPath is the language model's page and where its buttons post.
const SprachmodellPath = "/sprachmodell"

// Sprachmodell is the box's language model as a card (owner 2026-11-17):
// which model answers, for whom, until when and what that costs; a row
// per other preset with a button per duration, each asking first; and
// „Zurück zum Haushalt“. Switching is a lamp's right: a signed-in person,
// from Homes' own page (fromHomes), the switch as that person.
func Sprachmodell(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Sprachmodell == nil {
			modulAus(w, r, cfg, "sprachmodell", r.URL.Path)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodPost {
			modellSchalten(w, r, cfg)
			return
		}
		if !showOnly(w, r, texte.T(texte.SprachmodellTitel)) {
			return
		}
		now, loc := cfg.clock()
		a := cfg.Sprachmodell.Lesen(r.Context()).Sehen("de", now, loc)
		b := familyHead(r, cfg, a.Titel, "Mehr")
		jetzt, auf := modellKarten(a)
		b.WriteString("<h1>" + html.EscapeString(a.Titel) + "</h1>" + liveRefresh("seite", 60) + jetzt.HTML() + auf.HTML())
		writePage(w, http.StatusOK, b)
	}
}

// modellKarten are the page's two cards: the model now with the way
// back, and the presets to switch to with what that costs (the app draws
// the same words as one card).
func modellKarten(a sprachmodell.Ansicht) (cards.Card, cards.Card) {
	jetzt := cards.Card{Title: texte.T(texte.SprachmodellJetzt)}
	row := cards.Row{Text: a.Zeile, Detail: cmp.Or(a.Kosten, a.Detail), Warn: a.Kosten != ""}
	if a.Zurueck != "" {
		row.Action = &cards.Action{Label: a.Zurueck, Do: "haushalt", Back: SprachmodellPath, Path: SprachmodellPath, Laeuft: texte.T(texte.SprachmodellGibt)}
	}
	jetzt.Rows = append(jetzt.Rows, row)
	if len(a.Wahl) == 0 { // down, or someone else's window: what that means is the note
		jetzt.Note = a.Hinweis
	}
	c := cards.Card{Title: texte.T(texte.SprachmodellAuf), Note: a.Hinweis}
	for _, wahl := range a.Wahl {
		row := cards.Row{Text: wahl.Name, Also: []*cards.Action{}}
		for _, d := range wahl.Dauern {
			row.Also = append(row.Also, &cards.Action{Label: d.Label, Do: wahl.Preset + "@" + strconv.Itoa(d.Stunden),
				Back: SprachmodellPath, Path: SprachmodellPath, Laeuft: texte.T(texte.SprachmodellSchaltet), Frage: &cards.Frage{Text: d.Frage, Ja: d.Ja, Nein: d.Nein}})
		}
		c.Rows = append(c.Rows, row)
	}
	return jetzt, c
}

// modellSchalten is a button's POST: tun is „<preset>@<Stunden>“ or
// „haushalt“. Done, the page again (303); refused, the sentence.
func modellSchalten(w http.ResponseWriter, r *http.Request, cfg StartConfig) {
	_, loc := cfg.clock()
	if !fromHomes(r) {
		http.Error(w, texte.T(texte.SprachmodellFremd), http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, texte.T(texte.SprachmodellNicht), http.StatusBadRequest)
		return
	}
	person, tun := r.Header.Get("Remote-User"), r.PostForm.Get("tun")
	var err error
	if tun == "haushalt" {
		err = cfg.Sprachmodell.Zurueck(r.Context(), person)
	} else {
		preset, h, _ := strings.Cut(tun, "@")
		n, _ := strconv.Atoi(h)
		err = cfg.Sprachmodell.Umschalten(r.Context(), person, preset, time.Duration(n)*time.Hour)
	}
	if err == nil {
		http.Redirect(w, r, SprachmodellPath, http.StatusSeeOther)
		return
	}
	say := sprachmodell.Sagen("de", err, loc)
	if r.Header.Get("X-Homes-Skript") != "" { // the page stays; the sentence is shown on it
		http.Error(w, say, http.StatusConflict)
		return
	}
	missing{http.StatusOK, say, texte.T(texte.SprachmodellTitel), SprachmodellPath, "Zurück"}.write(w, r, cfg)
}

package pages

import (
	"context"
	"html"
	"log"
	"net/http"
	"strings"

	"github.com/mdopp/homes/internal/assistent"
	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/einkauf"
	"github.com/mdopp/homes/internal/notes"
)

// RezeptePath is the household's recipes (owner 2026-11-27: „Wenn ich
// Rezepte mit der App teile, werden die mir nicht mehr in der App
// angezeigt“): the notes of kind rezept, newest first, a search over their
// title and text (the notes' own search), each row its note's page, and
// the way into Einkauf's recipe choice. The recipes are the household's,
// as every note is: the page shows them to everyone alike.
const RezeptePath = "/rezepte"

// Rezepte is the page.
func Rezepte(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !showOnly(w, r, "Rezepte") {
			return
		}
		if cfg.Notes == nil {
			notSetNotes.write(w, r, cfg, "Rezepte")
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if rs := []rune(q); len(rs) > maxQuery {
			q = string(rs[:maxQuery])
		}
		var es []notes.Entry
		var err error
		fetch(r, func(ctx context.Context) { es, err = rezepteLesen(ctx, cfg.Notes, q) })
		b := familyHead(r, cfg, "Rezepte", "Notizen")
		b.WriteString(`<h1>Rezepte</h1><p class=k>Die Rezepte des Haushalts, das neueste zuerst.</p>` +
			`<form method=get action=` + RezeptePath + ` role=search><input type=search name=q value="` + html.EscapeString(q) +
			`" placeholder="Rezepte durchsuchen" aria-label="Rezepte durchsuchen"> <button data-laeuft="Sucht …">Suchen</button></form>`)
		c := cards.Card{Title: "Rezepte"}
		_, loc := cfg.clock()
		for _, e := range es {
			wann := "" // a search's hit says no date
			if !e.Changed.IsZero() {
				wann = "geändert " + e.Changed.In(loc).Format("02.01.2006")
			}
			c.AddLink(e.Title, wann, cards.NotePage(e.Name))
		}
		switch {
		case err != nil:
			log.Printf("homes: rezepte: %v", err)
			b.WriteString(`<p>Die Rezepte lassen sich gerade nicht lesen. Versuch es gleich noch einmal.</p>`)
		case len(es) == 0 && q != "":
			b.WriteString(`<p>Kein Rezept zu „` + html.EscapeString(q) + `“. <a href=` + RezeptePath + `>Alle Rezepte</a></p>`)
		case len(es) == 0:
			b.WriteString(`<p>Noch kein Rezept. Teile eine Rezeptseite mit der Homes-App oder schick den Link an ` + html.EscapeString(assistent.Name()) + `.</p>`)
		default:
			b.WriteString(c.HTML())
		}
		if cfg.Einkauf != nil && err == nil && len(es) > 0 {
			b.WriteString(`<form method=get action=` + EinkaufRezeptePath + `><button data-laeuft="Öffnet …">Für den Einkauf auswählen</button></form>`)
		}
		writePage(w, http.StatusOK, b)
	}
}

// rezepteLesen are the recipes, those holding q's words when q is set.
func rezepteLesen(ctx context.Context, d *notes.Dir, q string) ([]notes.Entry, error) {
	if q == "" {
		return einkauf.Rezepte(ctx, d)
	}
	res, err := d.Search(ctx, q, 500)
	var out []notes.Entry
	for _, h := range res.Hits {
		if h.Kopf.Art == "rezept" {
			out = append(out, notes.Entry{Name: h.Name, Title: h.Title, Kopf: h.Kopf})
		}
	}
	return out, err
}

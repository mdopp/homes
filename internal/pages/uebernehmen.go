package pages

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	kontaktnetz "github.com/mdopp/homes/internal/netz"
	"github.com/mdopp/homes/internal/texte"
	"github.com/mdopp/homes/internal/uebernahme"
)

// UebernehmenPath is „Ins Adressbuch übernehmen“ (archiv#27 step 2, ADR
// 0002): the person's archive posts an answer here (d, s; internal/
// uebernahme), Homes shows each line and where it goes, and writes only
// when the person confirms with Übernehmen on this page (ja, from Homes'
// own page). Only the person whose archive signed it: another login, a
// forged or an old packet is refused, said in a sentence.
const UebernehmenPath = KontaktePath + "/uebernehmen"

const uebernehmenStyle = `<style>.uebernahme li{margin:10px 0}.uebernahme .k{display:block;font-size:.85rem}</style>`

// Uebernehmen is the page.
func Uebernehmen(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, KontaktePath, http.StatusSeeOther)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		now, _ := cfg.clock()
		wer := r.Header.Get("Remote-User")
		p, err := uebernahme.Lesen(r.PostFormValue("d"), r.PostFormValue("s"), cfg.ArchivGeheim[wer], wer, now)
		ja := r.PostFormValue("ja") != ""
		if ja && !fromHomes(r) {
			http.Error(w, "Das geht nur von Homes' eigener Seite.", http.StatusForbidden)
			return
		}
		b := kontakteHead(r, cfg, texte.T(texte.Uebernehmen), "", texte.T(texte.Uebernehmen))
		b.WriteString(uebernehmenStyle + "<h1>" + wort(texte.Uebernehmen) + "</h1>")
		code, said, problem := http.StatusOK, "", ""
		switch {
		case err != nil:
			code, problem = http.StatusBadRequest, texte.T(texte.UebernehmenUngueltig)
		case ja:
			said, problem = uebernehmenSchreiben(r.Context(), cfg, wer, p)
		}
		b.WriteString(gesagt(said, problem) + uebernehmenZeilen(p)) // a refused packet: no lines
		if err == nil && said == "" {
			b.WriteString("<p class=k>" + wort(texte.UebernehmenWas) + `</p><form method=post action="` + UebernehmenPath + `"><input type=hidden name=d value="` +
				html.EscapeString(r.PostFormValue("d")) + `"><input type=hidden name=s value="` + html.EscapeString(r.PostFormValue("s")) +
				`"><button name=ja value=1 data-laeuft="` + wort(texte.UebernehmenLaeuft) + `">` + wort(texte.UebernehmenJa) + "</button></form>")
		}
		if cfg.Archive[wer] {
			b.WriteString(`<p class=weiter-in><a href="` + html.EscapeString(cfg.ArchivURL+"/tresor/umfeld") + `">` + wort(texte.UebernehmenZurueck) + "</a></p>")
		}
		writePage(w, code, b)
	}
}

// uebernehmenZeilen are the packet's lines, each where it goes.
func uebernehmenZeilen(p uebernahme.Paket) string {
	var b strings.Builder
	b.WriteString("<ul class=uebernahme>")
	for _, z := range p.Zeilen {
		wohin := map[string]texte.ID{uebernahme.Haushalt: texte.UebernehmenHaushalt, uebernahme.Handy: texte.UebernehmenHandy}[uebernahme.Wohin(z)]
		b.WriteString("<li>" + html.EscapeString(fmt.Sprintf(texte.T(texte.UebernehmenIst), z.X.Name, kontaktnetz.Kurz(z.Art), z.Y.Name)) +
			"<span class=k>→ " + wort(wohin) + "</span></li>")
	}
	return b.String() + "</ul>"
}

// uebernehmenSchreiben writes every line, within dieselbeWait, and says how it ended.
func uebernehmenSchreiben(ctx context.Context, cfg StartConfig, wer string, p uebernahme.Paket) (said, problem string) {
	ctx, cancel := context.WithTimeout(ctx, dieselbeWait)
	defer cancel()
	handy := ""
	for _, z := range p.Zeilen {
		if err := uebernahme.Schreiben(ctx, cfg.Contacts, cfg.Antworten, wer, z); err != nil {
			log.Printf("homes: uebernehmen: a line not written: %v", err)
			return "", texte.T(texte.UebernehmenFehler)
		}
		if uebernahme.Wohin(z) == uebernahme.Handy {
			handy = " " + texte.T(texte.UebernehmenFertigHandy)
		}
	}
	return texte.T(texte.UebernehmenFertig) + handy, ""
}

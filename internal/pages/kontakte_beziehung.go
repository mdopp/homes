package pages

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	kontaktnetz "github.com/mdopp/homes/internal/netz"
	"github.com/mdopp/homes/internal/texte"
)

// A contact's „Beziehungen“ (owner 2026-11-09), for the key's owner: each changed in place. The
// questions „Wie steht X zu Y?“ moved into the person's archive (archiv#27, ADR 0002).

const netzFrageStyle = `<style>.bzeile select{min-height:44px;font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--edge);border-radius:10px;padding:0 8px}
.bzeile label{display:flex;align-items:center;gap:10px;padding:8px 0;border-bottom:1px solid var(--edge)}.bzeile span{flex:1 1 auto;min-width:0;overflow-wrap:anywhere}.bzeile small{display:block}.bzeile select{flex:0 0 46%}</style>`

// netzArten are the kinds, Kennen sich nicht and extra as options, art chosen.
func netzArten(art string, extra ...string) string {
	var b strings.Builder
	for _, w := range append(append(kontaktnetz.Arten[:len(kontaktnetz.Arten):len(kontaktnetz.Arten)], kontaktnetz.Fremd), extra...) {
		text := kontaktnetz.Wort(w)
		if id, ok := map[string]texte.ID{kontaktnetz.Fremd: texte.NetzFremd, kontaktnetz.Keine: texte.NetzKeineBez, "vergessen": texte.NetzVergessen}[w]; ok {
			text = texte.T(id)
		}
		b.WriteString(`<option value="` + w + `"` + map[bool]string{true: " selected"}[w == art] + ">" + html.EscapeString(text) + "</option>")
	}
	return b.String()
}

// netzBeziehungen is „Beziehungen“ on a contact's page (owner 2026-11-09:
// „inkl. Änderbarkeit“), for the key's owner: one line per relation, saved on choosing (owner
// 2026-11-10); Keine Beziehung takes it out of the cards, Vergessen drops such an answer.
func netzBeziehungen(r *http.Request, cfg StartConfig, buch, uid, action string) string {
	if !netzDa(r, cfg) {
		return ""
	}
	id := cfg.Netz.Von(buch + "/" + uid)
	bs := cfg.Netz.Beziehungen(id)
	if len(bs) == 0 {
		return ""
	}
	esc, b := html.EscapeString, strings.Builder{}
	b.WriteString(netzFrageStyle + "<h2>" + wort(texte.NetzBeziehungen) + "</h2><div class=paar>")
	for _, z := range bs {
		extra, quelle := []string{kontaktnetz.Keine}, ""
		if z.Art == kontaktnetz.Keine || z.Art == kontaktnetz.Fremd {
			extra = append(extra, "vergessen")
		}
		if !z.Gesagt {
			quelle = " <small class=k>" + wort(texte.NetzTelefonbuch) + "</small>"
		}
		b.WriteString(`<form class=bzeile method=post action="` + esc(action) + `"><input type=hidden name=x value="` + esc(z.Mit.ID) + `"><input type=hidden name=y value="` + esc(id) +
			`"><input type=hidden name=antwort value=aendern><label><span>` + esc(z.Mit.Name) + quelle + `</span><select name=art aria-label="` + esc(fmt.Sprintf(texte.T(texte.NetzIst), z.Mit.Name)) + `" onchange="this.form.requestSubmit()">` + netzArten(z.Art, extra...) +
			`</select></label><noscript><button>` + wort(texte.NetzSpeichern) + `</button></noscript></form>`)
	}
	return b.String() + "</div>"
}

// netzAntwort takes a change posted from a contact's relations (x, y,
// antwort aendern, art a kind, keine or vergessen) and says how it ended;
// a kind, and a relation taken away, is written into the cards too,
// within dieselbeWait.
func netzAntwort(r *http.Request, cfg StartConfig) (said, problem string) {
	art := r.PostFormValue("art")
	if r.PostFormValue("antwort") != "aendern" || art == "" {
		return "", texte.T(texte.NetzOhneWahl)
	}
	if art == "vergessen" {
		art = ""
	}
	x, y, err := cfg.Netz.Aendern(r.PostFormValue("x"), r.PostFormValue("y"), art)
	switch {
	case errors.Is(err, kontaktnetz.ErrKeineFrage):
		return "", texte.T(texte.DieselbeWeg)
	case err != nil:
		log.Printf("homes: netz: an answer not kept: %v", err)
		return "", texte.T(texte.NetzFehler)
	case art == "":
		return texte.T(texte.NetzGesagtVergess), ""
	case art == kontaktnetz.Keine:
		said = fmt.Sprintf(texte.T(texte.NetzGesagtWeg), x.Name, y.Name)
	case art == kontaktnetz.Fremd:
		said = texte.T(texte.NetzGesagtFremd)
	default:
		said = fmt.Sprintf(texte.T(texte.NetzGesagtArt), x.Name, kontaktnetz.Wort(art), y.Name)
	}
	ctx, cancel := context.WithTimeout(r.Context(), dieselbeWait)
	defer cancel()
	if err := cfg.Netz.Eintragen(ctx, x, y, art); err != nil {
		log.Printf("homes: netz: a relation not written to the address book: %v", err)
		said += " " + texte.T(texte.NetzNichtImBuch)
	}
	return said, ""
}

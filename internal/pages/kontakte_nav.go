package pages

import (
	"html"
	"net/http"
	"strings"

	"github.com/mdopp/homes/internal/texte"
)

// The Kontakte area's own selection (owner 2026-11-08: „keine Struktur,
// keinen Pfad“), under the Haushalt pills on every Kontakte page: the
// segments Haushalt · Meine · Fragen n · Antworten n, each only where it
// applies (Meine and Fragen for the Immich key's owner, Antworten while
// answers wait), none while Haushalt would stand alone; and below
// Kontakte the path back, Kontakte › Antworten › Oma Hilde.

// kontakteHead is a Kontakte page's head: seg (a segment's path) marked;
// weg the path below Kontakte, labels and their links in turn, the page
// itself last.
func kontakteHead(r *http.Request, cfg StartConfig, title, seg string, weg ...string) *strings.Builder {
	b := familyHead(r, cfg, title, "Kontakte")
	b.WriteString(kontakteNavStyle)
	type segment struct {
		pfad string
		id   texte.ID
		n    int
	}
	segs := []segment{{KontaktePath, texte.KontakteHaushalt, 0}}
	if privatEigen(r, cfg) {
		segs = append(segs, segment{MeineKontaktePath, texte.KontakteMeine, 0},
			segment{DieselbePath, texte.KontakteFragen, len(cfg.Gesichter.Paare()) + len(cfg.Gesichter.Fragen())})
	}
	if cfg.pruefenAn() {
		if n := len(cfg.Antworten.ZuPruefen(r.Header.Get("Remote-User"))); n > 0 || seg == AntwortenPath {
			segs = append(segs, segment{AntwortenPath, texte.KontakteAntworten, n})
		}
	}
	if len(segs) > 1 {
		b.WriteString("<nav class=segmente aria-label=Kontakte>")
		for _, s := range segs {
			b.WriteString("<a href=" + s.pfad + map[bool]string{true: " aria-current=page"}[s.pfad == seg] + ">" + wort(s.id) + zahl(s.n) + "</a>")
		}
		b.WriteString("</nav>")
	}
	if len(weg) > 0 {
		b.WriteString(`<nav class=pfad aria-label="` + wort(texte.KontaktePfad) + `"><a href=` + KontaktePath + `>` + html.EscapeString(texte.Titel("kontakte")) + `</a>`)
		for i := 0; i+1 < len(weg); i += 2 {
			b.WriteString(` › <a href="` + html.EscapeString(weg[i+1]) + `">` + html.EscapeString(weg[i]) + `</a>`)
		}
		b.WriteString(" › <span>" + html.EscapeString(weg[len(weg)-1]) + "</span></nav>")
	}
	return b
}

// wieDas is a page's explanation, folded away at its end.
func wieDas(saetze ...texte.ID) string {
	var b strings.Builder
	for _, s := range saetze {
		b.WriteString("<p class=k>" + wort(s) + "</p>")
	}
	return "<details class=wie><summary>" + wort(texte.KontakteWie) + "</summary>" + b.String() + "</details>"
}

// The segments are a row of words, the current underlined in the accent;
// they wrap at a phone's width rather than scroll. The path is small.
const kontakteNavStyle = `<style>.segmente{display:flex;flex-wrap:wrap;gap:0 20px;margin:18px 0 0;border-bottom:1px solid var(--line)}
.segmente a{display:inline-flex;align-items:center;gap:2px;min-height:44px;margin-bottom:-1px;border-bottom:2px solid transparent;color:var(--muted);text-decoration:none;font-size:.95rem}
.segmente a[aria-current]{color:var(--ink);font-weight:600;border-color:var(--accent)}.pfad{margin:6px 0 -20px;font-size:.85rem;color:var(--muted);overflow-wrap:anywhere}.pfad a{display:inline-flex;align-items:center;min-height:44px;color:var(--muted)}
.oben{display:flex;flex-wrap:wrap;gap:0 18px}.oben .weiter-in{margin-top:10px}.wie{margin:26px 0 0;color:var(--muted)}</style>`

package pages

import (
	"cmp"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/mdopp/homes/internal/cards"
	kontaktnetz "github.com/mdopp/homes/internal/netz"
	"github.com/mdopp/homes/internal/texte"
)

// NetzPath is „Wer steht wem nahe“ (owner 2026-11-09), for the Immich
// key's owner only: a radial picture around one person (?mitte=, the
// owner by default), the closest in rings by strength, thin lines for
// the ties among them, each a link that puts it in the middle; under it
// the same as a ranked list with the reasons (the phone's and the
// screen reader's way), each with Ausblenden and Nicht berechnen; then
// the hidden ones, restorable. Anyone else goes to Kontakte.
const NetzPath = KontaktePath + "/netz"

const (
	netzZeigen    = 30 // in the picture; „Mehr zeigen“ doubles it
	netzJeKontakt = 5  // on a contact's page
)

const netzBildStyle = `<style>.netzbild{display:block;width:100%;max-width:400px;height:auto;margin:16px auto 0}.netzbild line{stroke:var(--muted);stroke-linecap:round}.netzbild line.zw{stroke:var(--line);stroke-width:.6}
.netzbild circle{fill:var(--card);stroke:var(--accent);stroke-width:1.5}.netzbild .mitte circle{fill:var(--accent)}.netzbild text{fill:var(--ink);font-size:10px;text-anchor:middle}.netzbild a:focus circle,.netzbild a:hover circle{fill:var(--accent)}
.netzliste form{flex-wrap:wrap;gap:0 4px;margin:4px -8px 0}</style>`

// netzDa: the viewer is the key's owner and there is a net.
func netzDa(r *http.Request, cfg StartConfig) bool { return cfg.Netz != nil && privatEigen(r, cfg) }

// NetzSeite is the page, and the hiding posted from it.
func NetzSeite(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && !fromHomes(r) {
			http.Error(w, "Das geht nur von Homes' eigener Seite.", http.StatusForbidden)
			return
		} else if r.Method != http.MethodPost && !showOnly(w, r, texte.T(texte.Netz)) {
			return
		}
		if !netzDa(r, cfg) {
			http.Redirect(w, r, KontaktePath, http.StatusSeeOther)
			return
		}
		said, problem := "", ""
		if r.Method == http.MethodPost {
			said, problem = netzVerbergen(r, cfg)
		}
		mitte, n := cmp.Or(r.FormValue("mitte"), cfg.Netz.Mitte()), netzZeigen
		if r.FormValue("mehr") != "" {
			n *= 2
		}
		b := kontakteHead(r, cfg, texte.T(texte.Netz), "", texte.T(texte.Netz))
		b.WriteString(kontakteStyle + netzBildStyle + "<h1>" + wort(texte.Netz) + "</h1>" + gesagt(said, problem))
		vs := cfg.Netz.Nahe(mitte, n)
		if k, ok := cfg.Netz.Person(mitte); !ok || len(vs) == 0 {
			b.WriteString("<p class=k>" + wort(map[bool]texte.ID{true: texte.NetzKeine, false: texte.NetzLeer}[ok]) + "</p>")
		} else {
			b.WriteString(netzBild(cfg, k, vs) + netzListe(cfg, k, vs))
			if len(vs) == n && n == netzZeigen {
				b.WriteString(`<p class=weiter-in><a href="` + html.EscapeString(NetzPath+"?"+url.Values{"mitte": {mitte}, "mehr": {"1"}}.Encode()) + `">` + wort(texte.NetzMehr) + "</a></p>")
			}
		}
		b.WriteString(netzVerborgene(cfg, mitte) + wieDas(texte.NetzHinweis))
		writePage(w, http.StatusOK, b)
	}
}

// netzVerbergen takes a row's Ausblenden, Nicht berechnen or Zurückholen.
func netzVerbergen(r *http.Request, cfg StartConfig) (said, problem string) {
	id, art := r.PostFormValue("id"), r.PostFormValue("art")
	k, ok := cfg.Netz.Person(id)
	gesagt := map[string]texte.ID{kontaktnetz.Aus: texte.NetzGesagtAus, kontaktnetz.Nicht: texte.NetzGesagtNicht, "": texte.NetzGesagtZurueck}
	if _, erlaubt := gesagt[art]; !erlaubt || id == "" {
		return "", texte.T(texte.NetzFehler)
	}
	if err := cfg.Netz.Verbergen(id, art); err != nil {
		return "", texte.T(texte.NetzFehler)
	}
	if !ok {
		k.Name = id
	}
	return fmt.Sprintf(texte.T(gesagt[art]), k.Name), ""
}

// netzMitte is the address that puts id in the middle.
func netzMitte(id string) string { return NetzPath + "?" + url.Values{"mitte": {id}}.Encode() }

// netzBild is the picture: the middle, the first fifth on the inner ring,
// up to half on the middle one, the rest outside; a line to each, its
// width the tie's strength, and thin ones among them.
func netzBild(cfg StartConfig, mitte kontaktnetz.Knoten, vs []kontaktnetz.Verbindung) string {
	const c = 200.0
	ids, xy := make([]string, len(vs)), make([][2]float64, len(vs))
	ringe := [3][2]int{{0, max(1, len(vs)/5)}, {max(1, len(vs)/5), max(2, len(vs)/2)}, {max(2, len(vs)/2), len(vs)}}
	for ring, rg := range ringe {
		for i := rg[0]; i < min(rg[1], len(vs)); i++ {
			a := 2*math.Pi*float64(i-rg[0])/float64(rg[1]-rg[0]) + float64(ring)*0.5 - math.Pi/2
			r := []float64{70, 120, 165}[ring]
			ids[i], xy[i] = vs[i].Mit.ID, [2]float64{c + r*math.Cos(a), c + r*math.Sin(a)}
		}
	}
	var b strings.Builder
	b.WriteString(`<svg class=netzbild viewBox="0 0 400 400" role=img aria-label="` + html.EscapeString(fmt.Sprintf(texte.T(texte.NetzBild), mitte.Name)) + `">`)
	for _, p := range cfg.Netz.Zwischen(ids) {
		b.WriteString(fmt.Sprintf(`<line class=zw x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`, xy[p[0]][0], xy[p[0]][1], xy[p[1]][0], xy[p[1]][1]))
	}
	for i, v := range vs {
		b.WriteString(fmt.Sprintf(`<line x1="200" y1="200" x2="%.1f" y2="%.1f" stroke-width="%.1f"/>`, xy[i][0], xy[i][1], 0.5+2.5*v.Kante.Staerke/vs[0].Kante.Staerke))
	}
	for i, v := range vs {
		b.WriteString(netzPunkt(v.Mit, xy[i][0], xy[i][1], 5, i%2 == 1 && xy[i][1] < c))
	}
	b.WriteString(`<g class=mitte>` + netzPunkt(mitte, c, c, 9, false) + "</g></svg>")
	return b.String()
}

// netzPunkt is one person in the picture, a link that puts it in the
// middle; its label below it, or above (oben) where its neighbours' are.
func netzPunkt(k kontaktnetz.Knoten, x, y, r float64, oben bool) string {
	dy := r + 11
	if oben {
		dy = -r - 4
	}
	return fmt.Sprintf(`<a href="%s" aria-label="%s"><circle cx="%.1f" cy="%.1f" r="%.0f"/><text x="%.1f" y="%.1f">%s</text></a>`, html.EscapeString(netzMitte(k.ID)),
		html.EscapeString(fmt.Sprintf(texte.T(texte.NetzMitte), k.Name)), x, y, r, x, y+dy, html.EscapeString(kurz(k.Name)))
}

// kurz is a name in the picture: the first name and the last one's initial.
func kurz(name string) string {
	ws := strings.Fields(name)
	if len(ws) < 2 {
		return trimRunes(name, 12)
	}
	return trimRunes(ws[0], 12) + " " + string([]rune(ws[len(ws)-1])[:1]) + "."
}

// netzListe is the ranked list: each its name (to the middle), its
// contact, its reasons and its two buttons.
func netzListe(cfg StartConfig, mitte kontaktnetz.Knoten, vs []kontaktnetz.Verbindung) string {
	var b strings.Builder
	b.WriteString(`<section class="karte kontakte netzliste"><ol>`)
	for _, v := range vs {
		b.WriteString(`<li><div><a href="` + html.EscapeString(netzMitte(v.Mit.ID)) + `">` + html.EscapeString(v.Mit.Name) + "</a>")
		if buch, uid, ok := strings.Cut(v.Mit.Kontakt, "/"); ok {
			b.WriteString(` <a class=ruf href="` + html.EscapeString(kontaktPfad(buch, uid)) + `">` + wort(texte.NetzKontakt) + "</a>")
		}
		b.WriteString(" <span class=k>" + html.EscapeString(cfg.Netz.Gruende(mitte.ID, v.Kante)) + "</span></div>" +
			netzKnoepfe(mitte.ID, v.Mit.ID, kontaktnetz.Aus, kontaktnetz.Nicht) + "</li>")
	}
	return b.String() + "</ol></section>"
}

// netzKnoepfe is a row's form: a button per art, posted with id.
func netzKnoepfe(mitte, id string, arten ...string) string {
	knopf := map[string][2]texte.ID{kontaktnetz.Aus: {texte.NetzAus, texte.NetzAusLaeuft}, kontaktnetz.Nicht: {texte.NetzNicht, texte.NetzNichtLaeuft}, "": {texte.NetzZurueck, texte.NetzZurueckLaeuft}}
	out := `<form method=post action="` + NetzPath + `"><input type=hidden name=mitte value="` + html.EscapeString(mitte) + `"><input type=hidden name=id value="` + html.EscapeString(id) + `">`
	for _, art := range arten {
		out += `<button class=leise name=art value="` + art + `" data-laeuft="` + wort(knopf[art][1]) + `">` + wort(knopf[art][0]) + "</button>"
	}
	return out + "</form>"
}

// netzVerborgene is „Ausgeblendet“: each hidden one, how, and Zurückholen.
func netzVerborgene(cfg StartConfig, mitte string) string {
	vs := cfg.Netz.Verborgene()
	if len(vs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="karte kontakte netzliste" aria-labelledby=netz-verborgen><h2 id=netz-verborgen>` + wort(texte.NetzVerborgen) + "</h2><ul>")
	for _, v := range vs {
		b.WriteString("<li><div>" + html.EscapeString(v.Name))
		if v.Art == kontaktnetz.Nicht {
			b.WriteString(" <span class=k>" + wort(texte.NetzNichtArt) + "</span>")
		}
		b.WriteString("</div>" + netzKnoepfe(mitte, v.ID, "") + "</li>")
	}
	return b.String() + "</ul></section>"
}

// netzNahe is „Steht nahe“ on a contact's page, for the key's owner: the
// closest netzNahe with their reasons, each to its contact (or into the
// picture), and the picture around this one.
func netzNahe(r *http.Request, cfg StartConfig, buch, uid string) string {
	if !netzDa(r, cfg) {
		return ""
	}
	id := cfg.Netz.Von(buch + "/" + uid)
	c := cards.Card{Title: texte.T(texte.NetzNahe), Link: &cards.Link{Href: netzMitte(id), Text: texte.T(texte.Netz)}}
	for _, v := range cfg.Netz.Nahe(id, netzJeKontakt) {
		href := netzMitte(v.Mit.ID)
		if b, u, ok := strings.Cut(v.Mit.Kontakt, "/"); ok {
			href = kontaktPfad(b, u)
		}
		c.AddLink(v.Mit.Name, cfg.Netz.Gruende(id, v.Kante), href)
	}
	return c.HTML()
}

// netzEinstieg is Kontakte's link to the page, for the key's owner.
func netzEinstieg(r *http.Request, cfg StartConfig) string {
	if !netzDa(r, cfg) {
		return ""
	}
	return `<p class=weiter-in><a href="` + NetzPath + `">` + wort(texte.Netz) + "</a></p>"
}

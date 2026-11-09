package pages

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/paperless"
	"github.com/mdopp/homes/internal/texte"
)

// DokumenteSteuerPath is a tax year's documents (owner 2026-11-09:
// „steuerlich relevante Dokumente markieren … Rechnungen eines Jahres mit
// Lohnkosten und Dingen, die das Büro betreffen“): those dokumente's
// ordnen tagged „steuerlich“, one card per Steuer-Art with its sum (and
// the Lohnkosten for §35a), the year switched with ‹ ›; ?jahr= picks it,
// last year by default. With ?zip=1 the download for the tax adviser:
// their files, one folder per Steuer-Art, and a list (CSV, as a German
// spreadsheet opens it).
const DokumenteSteuerPath = "/dokumente/steuer"

// steuerGruppe is one Steuer-Art of a year: its documents and their sums.
type steuerGruppe struct {
	art         string
	es          []paperless.Eintrag
	summe, lohn float64
}

// DokumenteSteuer is the page and its download (DokumenteSteuerPath).
func DokumenteSteuer(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !showOnly(w, r, "Für die Steuer") {
			return
		}
		if cfg.Docs == nil {
			notSetDocs.write(w, r, cfg, "Dokumente")
			return
		}
		now, loc := cfg.clock()
		jahr, err := strconv.Atoi(r.URL.Query().Get("jahr"))
		if err != nil || jahr < 1950 || jahr > now.Year()+1 {
			jahr = now.Year() - 1
		}
		var es []paperless.Eintrag
		fetch(r, func(ctx context.Context) { es, err = cfg.Docs.Steuerbelege(ctx) })
		gs := steuerGruppen(es, jahr, loc)
		if r.URL.Query().Get("zip") == "1" && err == nil {
			steuerZip(w, r, cfg, jahr, gs, loc)
			return
		}
		b := familyHead(r, cfg, texte.T("dok.steuer"), "Dokumente")
		link := func(j int, s string) string {
			return `<a href="` + DokumenteSteuerPath + "?jahr=" + strconv.Itoa(j) + `" aria-label="` + strconv.Itoa(j) + `">` + s + "</a>"
		}
		weiter := map[bool]string{true: link(jahr+1, "›")}[jahr < now.Year()]
		b.WriteString(dokFindenStyle + `<style>.jahr{display:flex;gap:18px;align-items:center;font-size:1.3rem}.jahr a{text-decoration:none;padding:0 10px}</style>` +
			`<p class=k><a href=` + DokumentePath + `>‹ ` + texte.T("dok.zurueck") + `</a></p><h1>` + texte.T("dok.steuer") + "</h1><p class=jahr>" +
			link(jahr-1, "‹") + "<b>" + strconv.Itoa(jahr) + "</b>" + weiter + "</p>")
		switch {
		case err != nil:
			log.Printf("homes: dokumente: steuer: %v", err)
			b.WriteString("<p class=k>" + texte.T("dok.fehler") + "</p>")
		case len(gs) == 0:
			b.WriteString("<p class=k>" + fmt.Sprintf(texte.T("dok.steuer.leer"), jahr) + "</p>")
		default:
			b.WriteString(`<p><a class=knopf href="` + DokumenteSteuerPath + "?jahr=" + strconv.Itoa(jahr) + `&amp;zip=1" download>` + texte.T("dok.steuer.laden") + "</a></p>")
		}
		opens := cfg.paperlessOpens(r)
		for _, g := range gs {
			c := cards.Card{Title: g.art + " · " + euro(g.summe)}
			if g.lohn > 0 {
				c.Note = fmt.Sprintf(texte.T("dok.steuer.lohn"), euro(g.lohn))
			}
			for _, e := range g.es {
				title, detail := dokZeile(e, "", loc)
				c.AddLink(title, detail, map[bool]string{true: cards.DocumentPage(e.ID)}[opens])
			}
			b.WriteString(c.HTML())
		}
		writePage(w, http.StatusOK, b)
	}
}

// steuerGruppen are the documents of jahr (their Steuerjahr, else the
// year they were made) by Steuer-Art, in its alphabet's order, each the
// oldest first (es is the newest first); one without a Steuer-Art last.
func steuerGruppen(es []paperless.Eintrag, jahr int, loc *time.Location) []steuerGruppe {
	by := map[string]*steuerGruppe{}
	for _, e := range slices.Backward(es) {
		j, err := strconv.Atoi(e.Felder["Steuerjahr"])
		if err != nil {
			j = e.Created.In(loc).Year()
		}
		if j != jahr {
			continue
		}
		art := cmp.Or(e.Felder["Steuer-Art"], texte.T("dok.steuer.ohne"))
		g := by[art]
		if g == nil {
			g = &steuerGruppe{art: art}
			by[art] = g
		}
		g.es = append(g.es, e)
		g.summe += betrag(e.Felder["Betrag"])
		g.lohn += betrag(e.Felder["Lohnkosten"])
	}
	var gs []steuerGruppe
	for _, g := range by {
		gs = append(gs, *g)
	}
	ohne := texte.T("dok.steuer.ohne")
	slices.SortFunc(gs, func(a, b steuerGruppe) int {
		if (a.art == ohne) != (b.art == ohne) {
			return map[bool]int{true: 1, false: -1}[a.art == ohne]
		}
		return cmp.Compare(a.art, b.art)
	})
	return gs
}

// betrag is an amount as paperless says it ("84,50 €"); 0 for none.
func betrag(s string) float64 {
	f, _ := strconv.ParseFloat(strings.Replace(strings.TrimSuffix(s, " €"), ",", ".", 1), 64)
	return f
}

// euro is an amount as the family reads it: "1.234,56 €".
func euro(f float64) string {
	cent := int64(f*100 + 0.5)
	return fmt.Sprintf("%s,%02d €", texte.Tausender(cent/100), cent%100)
}

var dateiRE = regexp.MustCompile(`[^\p{L}\p{N} ._&-]+`)

// steuerZip sends a year's files, a folder per Steuer-Art, and the list
// Steuer-<jahr>.csv (Datum, Absender, Titel, Steuer-Art, Betrag,
// Lohnkosten, Datei). A file Paperless does not hand out is a row that
// says so, the rest still comes.
func steuerZip(w http.ResponseWriter, r *http.Request, cfg StartConfig, jahr int, gs []steuerGruppe, loc *time.Location) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="Steuer-%d.zip"`, jahr))
	z := zip.NewWriter(w)
	var liste bytes.Buffer
	t := csv.NewWriter(&liste)
	t.Comma = ';'
	t.Write([]string{"Datum", "Absender", "Titel", "Steuer-Art", "Betrag", "Lohnkosten", "Datei"})
	for _, g := range gs {
		for _, e := range g.es {
			tag := e.Created.In(loc).Format("2006-01-02")
			name := dateiRE.ReplaceAllString(g.art, "") + "/" + strings.TrimSpace(dateiRE.ReplaceAllString(trimRunes(tag+" "+e.Correspondent+" - "+e.Title, 120), "")) + " (" + strconv.Itoa(e.ID) + ")"
			ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
			body, typ, err := cfg.Docs.Datei(ctx, e.ID)
			if err == nil {
				ext, _ := mime.ExtensionsByType(strings.TrimSpace(strings.Split(typ, ";")[0]))
				if strings.Contains(typ, "pdf") || len(ext) == 0 {
					ext = []string{".pdf"} // what Paperless archives; an original of another kind keeps its own
				}
				name += ext[len(ext)-1]
				var f io.Writer
				if f, err = z.Create(name); err == nil {
					_, err = io.Copy(f, body)
				}
				body.Close()
			}
			cancel()
			if err != nil {
				log.Printf("homes: dokumente: steuer: file %d: %v", e.ID, err)
				name = texte.T("dok.steuer.fehlt")
			}
			t.Write([]string{e.Created.In(loc).Format("02.01.2006"), e.Correspondent, e.Title, g.art,
				strings.TrimSuffix(e.Felder["Betrag"], " €"), strings.TrimSuffix(e.Felder["Lohnkosten"], " €"), name})
		}
	}
	t.Flush()
	if f, err := z.Create(fmt.Sprintf("Steuer-%d.csv", jahr)); err == nil {
		f.Write(append([]byte("\xEF\xBB\xBF"), liste.Bytes()...)) // the BOM: a spreadsheet reads it as UTF-8
	}
	if err := z.Close(); err != nil {
		log.Printf("homes: dokumente: steuer: zip: %v", err)
	}
}

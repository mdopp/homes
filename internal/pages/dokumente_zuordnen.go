package pages

import (
	"html"
	"net/http"
	"slices"
	"strconv"
)

// „Wem gehört dieser Scan?“ (laib DATA.md gap 6, owner 2026-11-10): a scan Dokumente
// cannot give to a person or the household stays its own, seen by no one in the
// family; the admins answer here, seeing only the sender, date, pages and when it
// came. Dokumente posts the list and takes the answers (cmd/homes zuordnen.go).
const ZuordnenPath = "/dokumente/zuordnen"

// Scan is one document waiting (JSON in lower case); Zeichen is Dokumente's mark of
// what is shown, so an answer counts only for what the admin saw; An the answer given.
type Scan struct {
	ID, Seiten                         int
	Zeichen, Absender, Datum, Seit, An string
}

type Person struct{ Login, Name string } // one a document may go to: a Paperless account

// Zuordnen is the list Dokumente left, and where an admin's answer goes.
type Zuordnen interface {
	Stand() ([]Scan, []Person)
	Antworte(id int, zeichen, an, von string) error
}

// offeneScans counts the scans nobody answered yet.
func offeneScans(z Zuordnen) int {
	offen, _ := z.Stand()
	return len(slices.DeleteFunc(slices.Clone(offen), func(s Scan) bool { return s.An != "" }))
}

// ZuordnenSeite is the admins' page: each scan, a button per person and „Haushalt“.
func ZuordnenSeite(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Zuordnen == nil || !isAdmin(r) {
			missing{http.StatusNotFound, "Diese Seite ist für Admins.", "Dokumente", DokumentePath, "Dokumente"}.write(w, r, cfg)
			return
		}
		if r.Method == http.MethodPost {
			id, _ := strconv.Atoi(r.FormValue("id"))
			if !fromHomes(r) || cfg.Zuordnen.Antworte(id, r.FormValue("zeichen"), r.FormValue("an"), r.Header.Get("Remote-User")) != nil {
				http.Redirect(w, r, ZuordnenPath+"?nicht=1", http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, ZuordnenPath+"?gemerkt=1", http.StatusSeeOther)
			return
		}
		offen, personen := cfg.Zuordnen.Stand()
		b := familyHead(r, cfg, "Wem gehört dieser Scan?", "Dokumente")
		b.WriteString("<h1>Wem gehört dieser Scan?</h1><p class=k>Dokumente konnte nicht erkennen, für wen diese Dokumente sind. Bis du antwortest, sieht sie niemand in der Familie; hier steht nur, was du zum Entscheiden brauchst.</p>")
		b.WriteString(gesagt(map[bool]string{true: "Gemerkt. Dokumente gibt es beim nächsten Durchgang weiter, spätestens in einer Stunde."}[r.URL.Query().Has("gemerkt")],
			map[bool]string{true: "Das ging nicht: die Liste hat sich inzwischen geändert. Bitte noch einmal wählen."}[r.URL.Query().Has("nicht")]))
		if len(offen) == 0 {
			b.WriteString("<p class=k>Gerade wartet nichts.</p>")
		}
		for _, s := range offen {
			von := s.Absender
			if von == "" {
				von = "unbekanntem Absender"
			}
			b.WriteString("<section class=karte><h2>Von " + html.EscapeString(von) + "</h2><p class=k>" + html.EscapeString(s.Datum+" · "+strconv.Itoa(s.Seiten)+" Seite(n) · eingegangen "+s.Seit) + "</p>")
			if i := slices.IndexFunc(personen, func(p Person) bool { return p.Login == s.An }); s.An != "" {
				an := map[bool]string{true: "den Haushalt", false: s.An}[s.An == "haushalt"]
				if i >= 0 {
					an = personen[i].Name
				}
				b.WriteString(zustand("gelb", "Geht an "+an+", sobald Dokumente das nächste Mal läuft (spätestens in einer Stunde)."))
			}
			b.WriteString(`<form method=post action=` + ZuordnenPath + `><input type=hidden name=id value=` + strconv.Itoa(s.ID) + `><input type=hidden name=zeichen value="` + html.EscapeString(s.Zeichen) + `">`)
			for _, p := range append(personen, Person{"haushalt", "Haushalt (die ganze Familie)"}) {
				b.WriteString(`<button name=an value="` + html.EscapeString(p.Login) + `" data-laeuft="Merkt …">` + html.EscapeString(p.Name) + `</button> `)
			}
			b.WriteString("</form></section>")
		}
		writePage(w, http.StatusOK, b)
	}
}

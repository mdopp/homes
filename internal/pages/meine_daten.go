package pages

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/laib"
	"github.com/mdopp/homes/internal/napi"
	"github.com/mdopp/homes/internal/texte"
)

// MeineDatenPath is „Meine Daten“ (homes#342): every way a person's data
// reaches the box on one page, each with when something last came, how
// much, and the next step; below, what Daten sichern said (homes#362: one
// entry in Mehr). The app's Daten sichern leads here.
const MeineDatenPath = "/meine-daten"

// stillNach is the quiet threshold of each source that comes by itself:
// the app backs up nightly, the archive fetches mail hourly, the phone
// sends its places all day. Every other source comes when the person
// sends it and is never quiet; the page writes each one's next to it.
var stillNach = map[string]time.Duration{"whatsapp": 48 * time.Hour, "signal": 48 * time.Hour, "sms": 48 * time.Hour, "mail": 24 * time.Hour, "standort": 24 * time.Hour}

// datenZeile is one source: its key (stillNach's), its name (markup), its
// dot (grau, "" current, gelb quiet or waiting, rot failed, lauf running)
// and its line; weg: never set up, the line is its way in.
type datenZeile struct {
	q, name, dot, says string
	weg                bool
}

// archivStand is what the person's archive says of itself in its status
// for Laib (archiv README „Status for laib“), read through `laib status
// archiv-<person>`: times and its own sentence of a failure — no
// content (homes ADR 0002).
type archivStand struct {
	Paket  map[string]struct{ Am time.Time } `json:"letztes_paket"`
	Import map[string]struct{ Am time.Time } `json:"letzter_import"`
	Fehler struct {
		Am     time.Time
		Quelle string
		Satz   string
	} `json:"letzter_fehler"`
}

// MeineDaten is the page: per source a dot and one line, the sources not
// set up below with their way in. The viewer's own only (Remote-User).
func MeineDaten(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != MeineDatenPath {
			Elsewhere(cfg)(w, r)
			return
		}
		if !showOnly(w, r, "Meine Daten") {
			return
		}
		person := r.Header.Get("Remote-User")
		if person == "" {
			http.Error(w, "kein Remote-User: diese Seite gehört hinter Authelia", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		da, wege := cardsList(texte.T(texte.MeineDatenDa)), cardsList(texte.T(texte.MeineDatenWege))
		zs, frei := meineZeilen(r, cfg, person)
		for _, z := range zs {
			if z.weg {
				wege.items = append(wege.items, quelle("grau", z.name, z.says, ""))
			} else {
				da.items = append(da.items, quelle(z.dot, z.name, z.says, schwelle(z.q)))
			}
		}
		if frei != "" {
			da.row("Auf der Box sind noch "+frei+" frei.", "", false)
		}
		wege.items = append(wege.items, meineWege(cfg, person)...)
		b := familyHead(r, cfg, "Meine Daten", "Meine Daten")
		b.WriteString(nachrichtenStyle + "<h1>Meine Daten</h1>")
		for _, c := range []*netzCard{da, wege} {
			if len(c.items) > 0 {
				b.WriteString(c.html())
			}
		}
		b.WriteString(sichernTeil(r, cfg, person)) // Daten sichern's, one page since homes#362
		writePage(w, http.StatusOK, b)
	}
}

// meineZeilen are the sources of the modules this box has, each read once:
// the phone's backups from Homes' ledger, the contacts as a count, mail
// from the archive's status, the places from Reitti, the scans from the
// scan's status.
func meineZeilen(r *http.Request, cfg StartConfig, person string) (zs []datenZeile, frei string) {
	now, loc := cfg.clock()
	var ar archivStand
	arOK, kn, scan, scanOK := false, -1, laib.Scan{}, false
	fetch(r, func(ctx context.Context) {
		ar, arOK = archivVon(ctx, cfg, person)
	}, func(ctx context.Context) {
		if cfg.Laib != nil && cfg.Docs != nil {
			var err error
			scan, err = cfg.Laib.ScanStatus(ctx)
			scanOK = err == nil
		}
	}, func(ctx context.Context) {
		if cfg.Sicherung != nil && cfg.Contacts != nil {
			if cs, _, err := cfg.Contacts.BookCards(ctx, "/"+url.PathEscape(person)+"/handy-kontakte/"); err == nil {
				kn = len(cs)
			}
		}
	})
	if cfg.Sicherung != nil {
		var hz []datenZeile
		hz, frei = handyZeilen(cfg, person, ar, now, loc)
		zs = append(zs, hz...)
	}
	if kn >= 0 {
		z := datenZeile{q: "kontakte", name: "Kontakte", says: texte.Anzahl(kn, "Kontakt", "Kontakte") + " im Adressbuch „Handy-Kontakte“"}
		if kn == 0 {
			z.weg, z.says = true, texte.T(texte.DatenInDerApp)
		}
		zs = append(zs, z)
	}
	if cfg.ArchivURL != "" {
		zs = append(zs, mailZeile(cfg, person, ar, arOK, now, loc))
	}
	if cfg.Reitti != nil {
		zs = append(zs, standortZeile(cfg, person, now, loc))
	}
	if scanOK {
		zs = append(zs, scanZeilen(scan, person, now, loc)...)
	}
	return zs, frei
}

// wann is „heute 03:12“, „Mo 09.11. 18:30“.
func wann(t, now time.Time, loc *time.Location) string {
	return dayWord(t, now, loc) + " " + t.In(loc).Format("15:04")
}

// angekommen is a source that came at t: current, or quiet past its threshold with the next step.
func angekommen(z datenZeile, t time.Time, wieviel string, now time.Time, loc *time.Location) datenZeile {
	z.dot, z.says = "", "zuletzt "+wann(t, now, loc)
	if wieviel != "" {
		z.says += " · " + wieviel
	}
	if d, ok := stillNach[z.q]; ok && now.Sub(t) > d {
		z.dot, z.says = "gelb", "still seit "+wann(t, now, loc)+" · "+texte.T(texte.ID("meine-daten.still."+z.q))
	}
	return z
}

// schwelle is a source's threshold in words, written under its line.
func schwelle(q string) string {
	if d, ok := stillNach[q]; ok {
		return fmt.Sprintf(texte.T(texte.MeineDatenNachTag), texte.Anzahl(int(d/(24*time.Hour)), "Tag", "Tagen"))
	} else if q == "kontakte" {
		return texte.T(texte.MeineDatenZahl)
	}
	return texte.T(texte.MeineDatenSelbst)
}

// gescheitert is the archive's failure for source q when it came after
// the last import of q: the archive's own sentence, which says what to do.
func gescheitert(z datenZeile, ar archivStand) datenZeile {
	if f := ar.Fehler; f.Quelle == z.q && f.Satz != "" && f.Am.After(ar.Import[z.q].Am) {
		z.dot, z.weg, z.says = "rot", false, texte.T(texte.MeineDatenFehler)+f.Satz
	}
	return z
}

// handyZeilen are the app's backups, from Homes' ledger and the inbox
// (the same as the app's /napi/sicherung/stand).
func handyZeilen(cfg StartConfig, person string, ar archivStand, now time.Time, loc *time.Location) (zs []datenZeile, frei string) {
	st, err := cfg.Sicherung.Stand(person)
	if err != nil {
		log.Printf("homes: meine daten: stand of %s: %v", person, err)
		return []datenZeile{{q: "sicherung", name: "Handy", dot: "grau", says: saysStandFehlt}}, ""
	}
	for _, q := range napi.Quellen {
		qs, z := st.Quellen[q], datenZeile{q: q, name: html.EscapeString(quelleName[q])}
		if t := am(qs); t.IsZero() {
			z.weg, z.says = true, texte.T(map[bool]texte.ID{true: texte.DatenTeilen, false: texte.DatenInDerApp}[q == "sozial"])
		} else {
			z = angekommen(z, t, texte.Anzahl(qs.Dateien, "Datei", "Dateien")+", "+texte.Groesse(qs.Bytes), now, loc)
		}
		if qs.Wartend > 0 {
			z.dot, z.says = "gelb", z.says+" · "+texte.Anzahl(qs.Wartend, "Datei", "Dateien")+map[bool]string{true: " wartet", false: " warten"}[qs.Wartend == 1]+": "+texte.T(texte.MeineDatenWartet)
		}
		zs = append(zs, gescheitert(z, ar))
	}
	return zs, texte.Groesse(st.FreiBytes)
}

// mailZeile is the mail the archive fetches by IMAP, from its status: the
// later of its last fetch and its last import.
func mailZeile(cfg StartConfig, person string, ar archivStand, arOK bool, now time.Time, loc *time.Location) datenZeile {
	z := datenZeile{q: "mail", name: "E-Mail"}
	t := ar.Paket["mail"].Am
	if i := ar.Import["mail"].Am; i.After(t) {
		t = i
	}
	switch {
	case !cfg.Archive[person]:
		z.weg, z.says = true, texte.T(texte.KontaktKeinArchiv)
	case !arOK:
		z.dot, z.says = "grau", texte.T(texte.MeineDatenStumm)
	case t.IsZero():
		z.weg, z.says = true, texte.T(texte.DatenMail)
	default:
		z = angekommen(z, t, "", now, loc)
	}
	return gescheitert(z, ar)
}

// standortZeile is the person's places in Reitti: not connected, connected
// with nothing yet, or the last point. The app's Standortverlauf row says
// the same (Zeile).
func standortZeile(cfg StartConfig, person string, now time.Time, loc *time.Location) datenZeile {
	z := datenZeile{q: "standort", name: `<a href=` + StandortPath + `>Standort</a>`}
	switch k, ok := cfg.Reitti.Konto(person); {
	case !ok:
		z.weg, z.says = true, "noch nicht verbunden"
	case k.Zuletzt.IsZero():
		z.dot, z.says = "gelb", "verbunden, noch kein Punkt angekommen"
	default:
		z = angekommen(z, k.Zuletzt, "", now, loc)
	}
	return z
}

// scanZeilen are the scanner and the inbox folder, from the scan's status:
// running while a stack of the person's waits or is split, the last stack
// the person started, or the way in.
func scanZeilen(st laib.Scan, person string, now time.Time, loc *time.Location) (zs []datenZeile) {
	for _, q := range []string{"scanner", "ordner"} {
		if q == "scanner" && !st.Scanner || q == "ordner" && st.Eingang == "" {
			continue
		}
		z := datenZeile{q: q, name: map[string]string{"scanner": "Scanner", "ordner": "Eingangsordner"}[q]}
		i := slices.IndexFunc(st.Letzte, func(s laib.Stack) bool { return s.Person == person && s.Quelle == q })
		var t time.Time
		if i >= 0 {
			t, _ = time.Parse(time.RFC3339, st.Letzte[i].At)
		}
		switch {
		case slices.ContainsFunc(st.Warten, func(s laib.ScanWartend) bool { return s.Person == person && s.Quelle == q }) ||
			q == "scanner" && st.Trennt != nil && st.Trennt.Person == person:
			z.dot, z.says = "lauf", "läuft gerade: Dokumente trennt und legt ab"
		case i < 0 && q == "scanner":
			z.weg, z.name, z.says = true, `<a href=`+ScannenPath+`>Scanner</a>`, texte.T(texte.MeineDatenScannen)
		case i < 0:
			z.weg, z.says = true, fmt.Sprintf(texte.T(texte.MeineDatenOrdner), st.Eingang)
		case st.Letzte[i].Ende == "fehler":
			z.dot, z.says = "rot", texte.T(texte.MeineDatenFehler)+strings.SplitN(st.Letzte[i].Meldung, "\n", 2)[0]
		default:
			z = angekommen(z, t, texte.Anzahl(len(st.Letzte[i].Dokumente), "Dokument", "Dokumente"), now, loc)
		}
		zs = append(zs, z)
	}
	return zs
}

// meineWege are the ways in Homes cannot see arrive: mail to Dokumente,
// Share → Homes, a Google location history; each while the box has it.
func meineWege(cfg StartConfig, person string) (out []string) {
	if m := cfg.DocumentsMail; m != "" {
		out = append(out, quelle("grau", "Dokumente per Mail", fmt.Sprintf(texte.T(texte.MeineDatenMailDok), strings.ToLower(person)+"+doc@"+m[strings.LastIndex(m, "@")+1:]), ""))
	}
	if cfg.Ablage != nil {
		out = append(out, quelle("grau", "Teilen", texte.T(texte.MeineDatenTeilen), ""))
	}
	if cfg.Reitti != nil {
		out = append(out, quelle("grau", "Google Takeout", texte.T(texte.MeineDatenTakeout), ""))
	}
	return out
}

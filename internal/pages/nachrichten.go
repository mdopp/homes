package pages

import (
	"context"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/texte"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/napi"
)

// DatenPath is the page „Daten sichern“ (haus#207, laib ADR 0029; owner
// 2026-11-03: more than messages go into the archive now).
const DatenPath = "/daten-sichern"

// NachrichtenPath is the page's old address, „Nachrichten sichern“: it
// leads to DatenPath, so a bookmark or an older app still lands.
const NachrichtenPath = "/nachrichten-sichern"

// What the page says that the catalog does not (texte: daten.*). The page is
// the one overview of everything that goes into the archive (owner
// 2026-11-08): the app keeps only what needs the phone, and links here.
const saysStandFehlt = "Der Stand lässt sich gerade nicht lesen. Später noch einmal versuchen."

// quelleName is how the page names a quelle.
var quelleName = map[string]string{"whatsapp": "WhatsApp", "signal": "Signal", "sms": "SMS und Anrufe", "sozial": "Facebook und LinkedIn"}

const nachrichtenStyle = `<style>.quelle{display:flex;gap:10px;align-items:flex-start}.quelle .punkt{margin-top:.55em}.schluessel .k{display:block;margin-top:6px}
.wechsel{margin:22px 0 0}.wechsel summary{font-weight:700}.wechsel ol{margin:8px 0 0;padding:10px 14px 10px 34px;background:var(--card);border-radius:14px}.wechsel ol li{margin:6px 0}</style>`

// Nachrichten is the page „Daten sichern“, the one overview: per source
// one line of what arrived (the same ledger the app's /napi/sicherung/stand
// reads), the phone's sources off said as „in der Homes-App einschalten“,
// the way into the archive for the keys, what never goes, and the phone
// change, folded. Its old address (NachrichtenPath) leads here.
func Nachrichten(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == NachrichtenPath {
			http.Redirect(w, r, DatenPath, http.StatusMovedPermanently)
			return
		}
		if !showOnly(w, r, "Daten sichern") {
			return
		}
		if cfg.Sicherung == nil {
			modulAus(w, r, cfg, "sicherung", DatenPath)
			return
		}
		person := r.Header.Get("Remote-User")
		if person == "" {
			http.Error(w, "kein Remote-User: diese Seite gehört hinter Authelia", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		b := familyHead(r, cfg, "Daten sichern", "Daten sichern")
		b.WriteString(nachrichtenStyle + "<h1>Daten sichern</h1>" + nachrichtenStand(r, cfg, person))
		if a, ok := cfg.Einrichtung.Eine(person, einrichten.Homes); ok && a.Erledigt == nil { // no app yet: the way to it
			b.WriteString(`<p><a href="` + einrichtenHref(einrichten.Homes) + `">` + html.EscapeString(a.Titel) + `</a></p>`)
		}
		b.WriteString(datenSchluessel(r, cfg, person) + "<details class=wechsel><summary>" + texte.T(texte.DatenNieTitel) + "</summary><p>" +
			html.EscapeString(texte.T(texte.DatenNie)) + "</p></details>" + nachrichtenZurueck(cfg, person))
		writePage(w, http.StatusOK, b)
	}
}

// datenSchluessel is the way to the keys: the archive's /tresor at home, why
// not otherwise; and that they belong in Bitwarden too.
func datenSchluessel(r *http.Request, cfg StartConfig, person string) string {
	weg := `<a class=knopf href="` + html.EscapeString(cfg.ArchivURL) + `/tresor">` + texte.T(texte.DatenSchluessel) + `</a><span class=k>` + texte.T(texte.DatenWelche) + `</span>`
	if cfg.ArchivURL == "" || !cfg.Archive[person] {
		weg = html.EscapeString(texte.T(texte.KontaktKeinArchiv))
	} else if !atHome(r) {
		weg = html.EscapeString(texte.T(texte.KontaktArchivDaheim))
	}
	return "<p class=schluessel>" + weg + "</p><p class=k>" + html.EscapeString(texte.T(texte.DatenBitwarden)) + "</p>"
}

// nachrichtenStand is the card „Dein Stand“: per source a dot and a line —
// grey, nothing yet; amber, files wait for the archive; green, arrived —
// then the room on the box; one plain line when it cannot be read.
func nachrichtenStand(r *http.Request, cfg StartConfig, person string) string {
	c := cardsList("Dein Stand")
	st, err := cfg.Sicherung.Stand(person)
	if err != nil {
		log.Printf("homes: nachrichten: stand of %s: %v", person, err)
		c.row(saysStandFehlt, "", false)
		return c.html()
	}
	now, loc := cfg.clock()
	for _, q := range napi.Quellen {
		qs := st.Quellen[q]
		dot, says := "grau", []string{"noch nichts angekommen", texte.T(map[bool]texte.ID{true: texte.DatenTeilen, false: texte.DatenInDerApp}[q == "sozial"])}
		if t := am(qs); !t.IsZero() {
			dot, says = "", []string{"zuletzt " + dayWord(t, now, loc) + " " + t.In(loc).Format("15:04"), texte.Anzahl(qs.Dateien, "Datei", "Dateien") + ", " + texte.Groesse(qs.Bytes)}
		}
		if qs.Wartend > 0 {
			dot, says = "gelb", append(says, texte.Anzahl(qs.Wartend, "Datei", "Dateien")+map[bool]string{true: " wartet", false: " warten"}[qs.Wartend == 1]+" auf dein Archiv")
		}
		c.items = append(c.items, quelle(dot, html.EscapeString(quelleName[q]), strings.Join(says, " · ")))
	}
	c.items = append(c.items, mitgesichert(r, cfg, person, now, loc)...)
	c.row("Auf der Box sind noch "+texte.Groesse(st.FreiBytes)+" frei.", "", false)
	return c.html()
}

// am is when a source's last file came; zero before the first.
func am(q napi.QuelleStand) (t time.Time) {
	if q.Zuletzt != nil {
		t, _ = time.Parse(time.RFC3339, *q.Zuletzt)
	}
	return t
}

// quelle is one line of „Dein Stand“: its dot, its name (markup), what is known.
func quelle(dot, name, says string) string {
	return `<li class=quelle><span class="punkt ` + dot + `"></span><div>` + name + `<span class=k>` + html.EscapeString(says) + `</span></div></li>`
}

// mitgesichert are what the archive keeps besides the phone's files (owner 2026-11-04): the
// location from Reitti's last point; the contacts the app mirrors into Handy-Kontakte and the
// calendars the archive takes over, each a count (neither tells Homes a time); E-Mail,
// which Homes cannot see, as the way to it.
func mitgesichert(r *http.Request, cfg StartConfig, person string, now time.Time, loc *time.Location) (out []string) {
	if k, ok := cfg.Reitti.Konto(person); cfg.Reitti != nil {
		dot, says := map[bool]string{true: "gelb", false: "grau"}[ok], "noch nichts aufgezeichnet"
		if !k.Zuletzt.IsZero() {
			dot, says = "", "zuletzt aufgezeichnet "+dayWord(k.Zuletzt, now, loc)+" "+k.Zuletzt.In(loc).Format("15:04")
		}
		out = append(out, quelle(dot, `<a href=`+StandortPath+`>Standort</a>`, says))
	}
	kn, kal := -1, -1 // how many; -1: not known
	fetch(r, func(ctx context.Context) {
		if cs, _, err := cfg.Contacts.BookCards(ctx, "/"+url.PathEscape(person)+"/handy-kontakte/"); err == nil {
			kn = len(cs)
		}
	}, func(ctx context.Context) {
		if ks, err := cfg.Calendar.PersonCalendars(ctx, person); err == nil {
			kal = len(ks)
		}
	})
	if kn >= 0 {
		out = append(out, quelle(map[bool]string{true: "", false: "grau"}[kn > 0], "Kontakte", map[bool]string{true: texte.Anzahl(kn, "Kontakt", "Kontakte") + " im Adressbuch „Handy-Kontakte“", false: "noch nichts angekommen · " + texte.T(texte.DatenInDerApp)}[kn > 0]))
	}
	out = append(out, quelle("grau", "E-Mail", texte.T(texte.DatenMail)))
	if kal >= 0 {
		out = append(out, quelle("grau", "Kalender", map[bool]string{true: texte.Anzahl(kal, "Kalender", "Kalender") + "; " + texte.T(texte.DatenTermine), false: texte.T(texte.DatenKeinKalender)}[kal > 0]))
	}
	return out
}

// nachrichtenZurueck is „Handy wechseln“, folded: what the archive keeps
// of the newest backup (napi's Letzte, the same as the app's
// /napi/sicherung/letzte), with date and size, and the change in three steps.
func nachrichtenZurueck(cfg StartConfig, person string) string {
	c := cardsList("Zum Zurückholen")
	letzte, err := cfg.Sicherung.Letzte(person)
	if err != nil {
		log.Printf("homes: nachrichten: letzte of %s: %v", person, err)
		c.row(saysStandFehlt, "", false)
	} else {
		_, loc := cfg.clock()
		am := func(ms int64) string {
			t := time.UnixMilli(ms)
			return "vom " + dayWords(t, loc) + ", " + t.In(loc).Format("15:04")
		}
		signal, neu := "noch keine Sicherung zum Zurückholen", int64(0)
		for _, d := range letzte["signal"] { // the newest
			if d.Geaendert > neu {
				neu, signal = d.Geaendert, "Sicherung "+am(d.Geaendert)+" · "+texte.Groesse(d.Groesse)
			}
		}
		c.row("Signal", signal, true)
		var says []string
		medien, medienBytes := 0, int64(0)
		for _, d := range letzte["whatsapp"] {
			if strings.HasPrefix(d.Pfad, "Media/") {
				medien++
				medienBytes += d.Groesse
			} else if d.Pfad == "Databases/msgstore.db.crypt15" {
				says = append(says, "Chats "+am(d.Geaendert)+" · "+texte.Groesse(d.Groesse))
			}
		}
		if medien > 0 {
			says = append(says, texte.Anzahl(medien, "Foto oder Sprachnachricht", "Fotos und Sprachnachrichten")+", "+texte.Groesse(medienBytes))
		}
		if len(says) == 0 {
			says = append(says, "noch nichts zum Zurückholen")
		}
		c.row("WhatsApp", strings.Join(says, " · "), true)
	}
	return "<details class=wechsel><summary>Handy wechseln</summary>" + c.html() + "<ol><li>" + strings.ReplaceAll(html.EscapeString(texte.T(texte.DatenWechsel)), "\n", "</li><li>") + "</li></ol><p class=k>" +
		html.EscapeString(texte.T(texte.DatenWechselMedien)) + "</p></details>"
}

package pages

import (
	"html"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/texte"

	"github.com/mdopp/homes/internal/einrichten"
	"github.com/mdopp/homes/internal/napi"
)

// DatenPath is the page „Daten sichern“'s address (haus#207): one with
// Meine Daten since homes#362 (owner 2026-11-12), it leads there, as its
// old address NachrichtenPath does, so a bookmark or an older app lands.
const DatenPath = "/daten-sichern"

// NachrichtenPath is the page's oldest address, „Nachrichten sichern“.
const NachrichtenPath = "/nachrichten-sichern"

// What Meine Daten says when Homes' ledger cannot be read.
const saysStandFehlt = "Der Stand lässt sich gerade nicht lesen. Später noch einmal versuchen."

// quelleName is how the page names a quelle.
var quelleName = map[string]string{"whatsapp": "WhatsApp", "signal": "Signal", "sms": "SMS und Anrufe", "sozial": "Facebook und LinkedIn"}

const nachrichtenStyle = `<style>.quelle{display:flex;gap:10px;align-items:flex-start}.quelle .punkt{margin-top:.55em}.schluessel .k{display:block;margin-top:6px}.schluessel .knopf{display:inline-flex;align-items:center;min-height:44px;padding:0 18px;border-radius:999px;background:var(--accent-soft);color:var(--accent);font-weight:600;text-decoration:none}.schwelle{display:block;font-size:.75rem;color:var(--faint)}.punkt.rot{background:var(--danger)}.punkt.lauf{background:var(--accent)}
.wechsel{margin:22px 0 0}.wechsel summary{font-weight:700}.wechsel ol{margin:8px 0 0;padding:10px 14px 10px 34px;background:var(--card);border-radius:14px}.wechsel ol li{margin:6px 0}</style>`

// Nachrichten leads Daten sichern's addresses to Meine Daten.
func Nachrichten(StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, MeineDatenPath, http.StatusMovedPermanently)
	}
}

// sichernTeil is what Daten sichern had beyond the sources (homes#362):
// the way to the Homes app while there is none, the way to the keys,
// what never goes, and the phone change, folded; "" without the backups.
func sichernTeil(r *http.Request, cfg StartConfig, person string) string {
	if cfg.Sicherung == nil {
		return ""
	}
	out := ""
	if a, ok := cfg.Einrichtung.Eine(person, einrichten.Homes); ok && a.Erledigt == nil { // no app yet: the way to it
		out = `<p><a href="` + einrichtenHref(einrichten.Homes) + `">` + html.EscapeString(a.Titel) + `</a></p>`
	}
	return out + datenSchluessel(r, cfg, person) + "<details class=wechsel><summary>" + texte.T(texte.DatenNieTitel) + "</summary><p>" +
		html.EscapeString(texte.T(texte.DatenNie)) + "</p></details>" + nachrichtenZurueck(cfg, person)
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

// am is when a source's last file came; zero before the first.
func am(q napi.QuelleStand) (t time.Time) {
	if q.Zuletzt != nil {
		t, _ = time.Parse(time.RFC3339, *q.Zuletzt)
	}
	return t
}

// quelle is one line of Meine Daten: its dot, its name (markup), what is
// known, and quieter below it its threshold ("": none).
func quelle(dot, name, says, schwelle string) string {
	if schwelle != "" {
		schwelle = "<small class=schwelle>" + html.EscapeString(schwelle) + "</small>"
	}
	return `<li class=quelle><span class="punkt ` + dot + `"></span><div>` + name + `<span class=k>` + html.EscapeString(says) + `</span>` + schwelle + `</div></li>`
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

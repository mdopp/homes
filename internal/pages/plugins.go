package pages

import (
	"cmp"
	"html"
	"net/http"

	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/modul"
	"github.com/mdopp/homes/internal/plugin"
)

// The plugins (homes ADR 0003 §3, homes#375): programs of their own that
// Laib registered with Homes. Homes renders what they serve on the
// contract (internal/plugin) and links into them for everything else; it
// names none of them. A plugin that never answered is not there; one that
// did not answer now is what it said last, with a quiet line.

// pluginIcons are the icon names a plugin may choose; another is iconBox.
var pluginIcons = map[string]string{"archiv": iconArchiv, "dokumente": iconDokumente, "daten": iconDaten, "zusammen": iconZusammen, "aufgaben": iconAufgaben}

// pluginArten sorts a plugin's task among Homes' own.
var pluginArten = map[string]int{"schluessel": artSchluessel, "einrichten": artEinrichten}

// pluginAufgaben are the tasks the viewer's plugins handed over, each
// with its text; kept ones („Stand: gestern“) while the plugin cannot say.
func pluginAufgaben(cfg StartConfig, ss []plugin.Stand) (out []aufgabe) {
	now, loc := cfg.clock()
	for _, s := range ss {
		stand := ""
		if !s.Jetzt && !s.Am.IsZero() {
			stand = "Stand: " + dayWord(s.Am, now, loc)
		}
		for _, t := range *cmp.Or(s.Aufgaben, &[]plugin.Aufgabe{}) {
			out = append(out, aufgabe{titel: t.Titel, warum: t.Warum, stand: stand, href: cmp.Or(t.Link, s.Link), extern: s.Name + " öffnen",
				art: cmp.Or(pluginArten[t.Art], artFrage), name: "plugin-" + s.Plugin + "-" + t.ID, icon: cmp.Or(s.Icon, "aufgabe")})
		}
	}
	return out
}

// pluginStill is the quiet line for each plugin that did not answer.
func pluginStill(ss []plugin.Stand) string {
	out := ""
	for _, s := range ss {
		if s.Still {
			out += "<p class=k>" + html.EscapeString(s.Name) + " antwortet gerade nicht.</p>"
		}
	}
	return out
}

// pluginEintraege are the viewer's plugins under Mehr: name, icon, link.
func pluginEintraege(r *http.Request, cfg StartConfig) (out []modul.Eintrag) {
	for _, s := range cfg.Plugins.Gemerkt(r.Header.Get("Remote-User")) {
		out = append(out, modul.Eintrag{Titel: s.Name, Pfad: s.Link, Icon: cmp.Or(pluginIcons[s.Icon], iconBox), Ort: modul.Mehr, Rang: 75})
	}
	return out
}

// pluginZeile is a plugin's line under Mehr, its quiet line while it is silent.
func pluginZeile(r *http.Request, cfg StartConfig, pfad string) string {
	for _, s := range cfg.Plugins.Gemerkt(r.Header.Get("Remote-User")) {
		if s.Link == pfad && s.Still {
			return s.Name + " antwortet gerade nicht."
		} else if s.Link == pfad {
			return s.Zeile
		}
	}
	return ""
}

// pluginKarten are the viewer's plugins' Start cards, as kept; a silent
// plugin's card is its quiet line.
func pluginKarten(cfg StartConfig, person string) string {
	out := ""
	for _, s := range cfg.Plugins.Gemerkt(person) {
		if s.Start == nil {
			continue
		}
		c := cards.Card{Title: s.Start.Titel}
		if s.Still {
			c.Add(s.Name+" antwortet gerade nicht.", "")
		} else {
			c.AddLink(cmp.Or(s.Start.Zeile, s.Name+" öffnen"), "", cmp.Or(s.Start.Link, s.Link))
		}
		out += c.HTML()
	}
	return out
}

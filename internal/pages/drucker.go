package pages

import (
	"context"
	"html"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/ha"
)

// A printer (owner 2026-11-14: „ist er erreichbar und welchen Füllstand
// hat die Farbe Schwarz“): the sensors of Home Assistant's IPP integration,
// one device's together. Its state sensor says idle, printing or stopped
// (an enum, no unit; unavailable when HA cannot reach it), each marker
// (ink, toner) its level in % with marker_type, marker_low_level and
// marker_high_level, but no colour: the colour is in the marker's name
// („… Black ink“). The uptime sensor is a timestamp and is left out.
type printer struct {
	status ha.State   // the state sensor; EntityID "" when HA's states have none
	inks   []ha.State // its markers, Schwarz, Cyan, Magenta, Gelb, then the others by name
	room   string
}

// inkLow is the level under which the black ink is „fast leer“.
const inkLow = 15

// printerWords are the IPP state sensor's states in the family's words.
var printerWords = map[string]string{"idle": "bereit", "printing": "druckt", "stopped": "angehalten"}

// inkColours are the words in a marker's name that say its colour, the
// colour's German name and its place in the line.
var inkColours = []struct{ word, name string }{{"black", "Schwarz"}, {"schwarz", "Schwarz"}, {"cyan", "Cyan"},
	{"magenta", "Magenta"}, {"yellow", "Gelb"}, {"gelb", "Gelb"}}

// findPrinters gathers the IPP sensors by device; what HA hides (the
// state sensor hidden) is left out, as Räume leaves out what HA hides.
func findPrinters(states []ha.State, reg ha.Registry) []printer {
	byDevice := map[string]*printer{}
	var keys []string
	for _, s := range states {
		if reg.Platforms[s.EntityID] != "ipp" || !strings.HasPrefix(s.EntityID, "sensor.") {
			continue
		}
		key := reg.Devices[s.EntityID]
		if key == "" {
			key = s.EntityID
		}
		p := byDevice[key]
		if p == nil {
			p = &printer{}
			byDevice[key], keys = p, append(keys, key)
		}
		switch unit, _ := s.Attributes["unit_of_measurement"].(string); {
		case unit == "%":
			p.inks = append(p.inks, s)
		case s.Attributes["device_class"] != "timestamp":
			p.status = s
		}
	}
	sort.Strings(keys)
	out := make([]printer, 0, len(keys))
	for _, k := range keys {
		p := byDevice[k]
		if reg.Hidden[p.status.EntityID] {
			continue
		}
		sort.SliceStable(p.inks, func(i, j int) bool { return inkOrder(p.inks[i]) < inkOrder(p.inks[j]) })
		p.room = oneLine(reg.Rooms[p.id()])
		out = append(out, *p)
	}
	return out
}

// inkOrder is a marker's place: its colour's, then its name.
func inkOrder(s ha.State) string {
	name := strings.ToLower(friendly(s))
	for i, c := range inkColours {
		if strings.Contains(name, c.word) {
			return strconv.Itoa(i/2) + name // black and schwarz share a place
		}
	}
	return "9" + name
}

// inkName is a marker's colour in German, else its name less the printer's.
func inkName(s, status ha.State) string {
	name := friendly(s)
	for _, c := range inkColours {
		if strings.Contains(strings.ToLower(name), c.word) {
			return c.name
		}
	}
	if rest := strings.TrimSpace(strings.TrimPrefix(name, friendly(status))); rest != "" {
		return rest
	}
	return name
}

// level is a marker's level in whole percent; !ok when HA has none.
func level(s ha.State) (int, bool) {
	v, err := strconv.ParseFloat(s.State, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return int(math.Round(v)), true
}

// reachable says whether HA reaches the printer.
func (p printer) reachable() bool {
	switch p.status.State {
	case "", "unavailable", "unknown":
		return false
	}
	return true
}

// id is the entity the printer's row opens and its room is read from: its
// state sensor, else its first marker.
func (p printer) id() string {
	if p.status.EntityID == "" && len(p.inks) > 0 {
		return p.inks[0].EntityID
	}
	return p.status.EntityID
}

// black is the printer's black marker; !ok when it has none.
func (p printer) black() (ha.State, bool) {
	if len(p.inks) > 0 && inkName(p.inks[0], p.status) == "Schwarz" {
		return p.inks[0], true
	}
	return ha.State{}, false
}

// row is the printer's one quiet row on its room's card: reachable (again,
// after a check) or not (since when, or what the last check found, with
// „Prüfen und neu verbinden“, owner 2026-11-14), and the black ink, a
// warning under inkLow. check is the printer's check running or answered;
// back the card the button lands on.
func (p printer) row(name string, now time.Time, loc *time.Location, check *pruefung, back string) cards.Row {
	r := cards.Row{Text: name + " · nicht erreichbar", Href: cards.ThingPage(p.id())}
	answered := check != nil && check.answered()
	if !p.reachable() {
		if p.status.State == "unavailable" {
			r.Detail = changedWords(p.status, now, loc)
		}
		p.checkRow(&r, check, answered, back)
		return r
	}
	r.Text = name + " · " + map[bool]string{true: "wieder erreichbar", false: "erreichbar"}[answered && check.wieder]
	if s, ok := p.black(); ok {
		if v, ok := level(s); ok {
			r.Detail, r.Warn = "Schwarz "+percent0(float64(v)), v < inkLow
			if r.Warn {
				r.Detail = "Schwarz fast leer · " + percent0(float64(v))
			}
		}
	}
	return r
}

// checkRow is an unreachable printer's row with its check: „Prüfe Verbindung
// …“ while it runs, what it found once answered, and the button that
// checks (again), a form to DruckerPruefenPath that lands back on the card.
func (p printer) checkRow(r *cards.Row, check *pruefung, answered bool, back string) {
	switch {
	case check != nil && !answered:
		r.Detail = saysPruefe
		return
	case answered && check.netz != "":
		r.Detail = check.netz
	}
	if p.status.EntityID != "" {
		r.Action = &cards.Action{Label: "Prüfen und neu verbinden", Entity: p.status.EntityID, Do: "pruefen", Back: back,
			Path: DruckerPruefenPath, Laeuft: "Prüfe …"}
	}
}

// printerRows are the rows for Räume's room cards: „Drucker“, or each
// printer's name when the house has more than one. c is the Home
// Assistant the checks were asked of.
func printerRows(states []ha.State, reg ha.Registry, now time.Time, loc *time.Location, c *ha.Client) []roomExtra {
	ps := findPrinters(states, reg)
	out := make([]roomExtra, 0, len(ps))
	for _, p := range ps {
		name := "Drucker"
		if len(ps) > 1 && p.status.EntityID != "" {
			name = friendly(p.status)
		}
		title := trimRunes(p.room, 60)
		if title == "" {
			title = noRoom
		}
		row := p.row(name, now, loc, laufend(c, p.status.EntityID), "/raeume#"+cards.Card{Title: title}.ID())
		out = append(out, roomExtra{room: p.room, name: name, row: row})
	}
	return out
}

// thingDetail is a thing page's state in the family's words: a printer's
// in its own (bereit, druckt, angehalten), anything else as Räume says it.
func thingDetail(domain string, s ha.State, reg ha.Registry) string {
	if w := printerWords[s.State]; w != "" && reg.Platforms[s.EntityID] == "ipp" {
		return w
	}
	return roomDetail(domain, s, reg.TempUnit)
}

// thisPrinter is the IPP printer id is one of the sensors of, as HA's
// states say it now; !ok for anything else, or while HA does not answer.
func thisPrinter(ctx context.Context, cfg StartConfig, id string, reg ha.Registry) (printer, bool) {
	if reg.Platforms[id] != "ipp" {
		return printer{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, backendWait)
	defer cancel()
	states, err := cfg.HA.States(ctx)
	if err != nil {
		return printer{}, false
	}
	for _, p := range findPrinters(states, reg) {
		if p.status.EntityID == id || slices.ContainsFunc(p.inks, func(s ha.State) bool { return s.EntityID == id }) {
			return p, true
		}
	}
	return printer{}, false
}

// inksLine is all its colours on one line („Farben: Schwarz 12 % · Cyan
// 40 % · …“), "" while HA reaches none of its inks.
func (p printer) inksLine() string {
	var parts []string
	for _, s := range p.inks {
		if v, ok := level(s); ok {
			parts = append(parts, inkName(s, p.status)+" "+percent0(float64(v)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "<p class=k>Farben: " + html.EscapeString(strings.Join(parts, " · ")) + "</p>"
}

// printerPage is what a printer's own page adds under its state: its
// colours, and on its state sensor's page the connection's check.
func printerPage(r *http.Request, cfg StartConfig, id string, reg ha.Registry) string {
	p, ok := thisPrinter(r.Context(), cfg, id, reg)
	if !ok {
		return ""
	}
	if p.status.EntityID == id {
		return p.inksLine() + printerCheck(r, cfg, id)
	}
	return p.inksLine()
}

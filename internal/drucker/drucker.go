// Package drucker is the household's printers as Home Assistant's IPP
// integration shows them (owner 2026-11-14: „ist er erreichbar und welchen
// Füllstand hat die Farbe Schwarz“), and how the family reads one: the
// same words on Homes' Geräte, a printer's own page and the app's „Deine
// Geräte“ („Bitte in der App und auf der Webseite gleich“).
//
// One device's IPP sensors together. Its state sensor says idle, printing
// or stopped (an enum, no unit; unavailable when HA cannot reach it), with
// state_reason (IPP's printer-state-reasons, keywords such as
// media-empty-error; absent for „none“) and state_message (the printer's
// own text, in its language). Each marker (ink, toner) says its level in %
// with marker_type, marker_low_level and marker_high_level, but no colour:
// the colour is in its name („… Black ink“). The uptime sensor is a
// timestamp and is left out.
package drucker

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/mdopp/homes/internal/ha"
)

// Drucker is one printer.
type Drucker struct {
	Status ha.State   // the state sensor; EntityID "" when HA's states have none
	Inks   []ha.State // its markers, Schwarz, Cyan, Magenta, Gelb, then the others by name
	Room   string     // its room's name, "" without one
}

// InkLow is the level under which the black ink is „fast leer“.
const InkLow = 15

// Words are the state sensor's states in the family's words.
var Words = map[string]string{"idle": "bereit", "printing": "druckt", "stopped": "angehalten"}

// inkColours are the words in a marker's name that say its colour, the
// colour's German name and its place in the line.
var inkColours = []struct{ word, name string }{{"black", "Schwarz"}, {"schwarz", "Schwarz"}, {"cyan", "Cyan"},
	{"magenta", "Magenta"}, {"yellow", "Gelb"}, {"gelb", "Gelb"}}

// reasons are the IPP printer-state-reasons a person can act on (RFC 8011
// §5.4.12, PWG 5100.9), without their -error or -warning, in short German.
var reasons = map[string]string{"media-empty": "Papier leer", "media-needed": "Papier leer", "media-jam": "Papierstau",
	"marker-supply-empty": "Tinte leer", "toner-empty": "Tinte leer", "cover-open": "Klappe offen", "door-open": "Klappe offen",
	"interlock-open": "Klappe offen", "input-tray-missing": "Papierfach fehlt", "output-area-full": "Ausgabefach voll",
	"marker-waste-full": "Resttintenbehälter voll"}

// Find gathers the IPP sensors by device; a printer whose state sensor HA
// hides is left out, as Geräte leaves out what HA hides.
func Find(states []ha.State, reg ha.Registry) []Drucker {
	byDevice := map[string]*Drucker{}
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
			p = &Drucker{}
			byDevice[key], keys = p, append(keys, key)
		}
		switch unit, _ := s.Attributes["unit_of_measurement"].(string); {
		case unit == "%":
			p.Inks = append(p.Inks, s)
		case s.Attributes["device_class"] != "timestamp":
			p.Status = s
		}
	}
	sort.Strings(keys)
	out := make([]Drucker, 0, len(keys))
	for _, k := range keys {
		p := byDevice[k]
		if reg.Hidden[p.Status.EntityID] {
			continue
		}
		sort.SliceStable(p.Inks, func(i, j int) bool { return inkOrder(p.Inks[i]) < inkOrder(p.Inks[j]) })
		p.Room = strings.Join(strings.Fields(reg.Rooms[p.ID()]), " ")
		out = append(out, *p)
	}
	return out
}

// Of is the printer whose state sensor or marker id is; !ok for anything else.
func Of(states []ha.State, reg ha.Registry, id string) (Drucker, bool) {
	if reg.Platforms[id] != "ipp" {
		return Drucker{}, false
	}
	for _, p := range Find(states, reg) {
		if p.Status.EntityID == id || p.has(id) {
			return p, true
		}
	}
	return Drucker{}, false
}

func (p Drucker) has(id string) bool {
	for _, s := range p.Inks {
		if s.EntityID == id {
			return true
		}
	}
	return false
}

// name is an entity's friendly name, else its id.
func name(s ha.State) string {
	if n, _ := s.Attributes["friendly_name"].(string); strings.TrimSpace(n) != "" {
		return strings.Join(strings.Fields(n), " ")
	}
	return s.EntityID
}

// inkOrder is a marker's place: its colour's, then its name.
func inkOrder(s ha.State) string {
	n := strings.ToLower(name(s))
	for i, c := range inkColours {
		if strings.Contains(n, c.word) {
			return strconv.Itoa(i/2) + n // black and schwarz share a place
		}
	}
	return "9" + n
}

// InkName is a marker's colour in German, else its name less the printer's.
func InkName(s, status ha.State) string {
	n := name(s)
	for _, c := range inkColours {
		if strings.Contains(strings.ToLower(n), c.word) {
			return c.name
		}
	}
	if rest := strings.TrimSpace(strings.TrimPrefix(n, name(status))); rest != "" {
		return rest
	}
	return n
}

// Level is a marker's level in whole percent; !ok when HA has none.
func Level(s ha.State) (int, bool) {
	v, err := strconv.ParseFloat(s.State, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return int(math.Round(v)), true
}

// Percent is a level as the family reads it: „12 %“, the two kept together.
func Percent(v int) string { return strconv.Itoa(v) + " %" }

// Reachable says whether HA reaches the printer.
func (p Drucker) Reachable() bool {
	switch p.Status.State {
	case "", "unavailable", "unknown":
		return false
	}
	return true
}

// ID is the entity a printer opens and its room is read from: its state
// sensor, else its first marker.
func (p Drucker) ID() string {
	if p.Status.EntityID == "" && len(p.Inks) > 0 {
		return p.Inks[0].EntityID
	}
	return p.Status.EntityID
}

// Black is the printer's black marker; !ok when it has none.
func (p Drucker) Black() (ha.State, bool) {
	if len(p.Inks) > 0 && InkName(p.Inks[0], p.Status) == "Schwarz" {
		return p.Inks[0], true
	}
	return ha.State{}, false
}

// Word is the printer's state as the family reads it: bereit, druckt,
// angehalten; „erreichbar“ for a state IPP has no word for; „nicht
// erreichbar“ while HA cannot reach it.
func (p Drucker) Word() string {
	if !p.Reachable() {
		return "nicht erreichbar"
	}
	if w := Words[p.Status.State]; w != "" {
		return w
	}
	return "erreichbar"
}

// Reason is the first state reason a person can act on, in short German
// („Papier leer“); "" for none. state_reason is one keyword, a list of
// them, or comma-separated (IPP's 1setOf as pyipp passes it on); a report
// (-report) only informs and is left out.
func (p Drucker) Reason() string {
	var all []string
	switch v := p.Status.Attributes["state_reason"].(type) {
	case string:
		all = strings.Split(v, ",")
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				all = append(all, s)
			}
		}
	}
	for _, r := range all {
		r = strings.ToLower(strings.TrimSpace(r))
		if strings.HasSuffix(r, "-report") {
			continue
		}
		if w := reasons[strings.TrimSuffix(strings.TrimSuffix(r, "-error"), "-warning")]; w != "" {
			return w
		}
	}
	return ""
}

// Detail is what follows the state while HA reaches the printer: the
// reason, then the black ink („Papier leer · Schwarz 12 %“, „Schwarz fast
// leer · 12 %“); warn when either asks for a hand. "" while unreachable.
func (p Drucker) Detail() (string, bool) {
	if !p.Reachable() {
		return "", false
	}
	var parts []string
	reason := p.Reason()
	if reason != "" {
		parts = append(parts, reason)
	}
	low := false
	if s, ok := p.Black(); ok {
		if v, ok := Level(s); ok {
			low = v < InkLow
			parts = append(parts, map[bool]string{true: "Schwarz fast leer · ", false: "Schwarz "}[low]+Percent(v))
		}
	}
	return strings.Join(parts, " · "), reason != "" || low
}

// Text is the printer in one line, as the app's „Deine Geräte“ shows it:
// „druckt · Schwarz 12 %“, „bereit · Papier leer · Schwarz 40 %“, „nicht
// erreichbar“.
func (p Drucker) Text() string {
	if d, _ := p.Detail(); d != "" {
		return p.Word() + " · " + d
	}
	return p.Word()
}

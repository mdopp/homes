package pages

import (
	"context"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/ha"
)

// what a room card shows, in this order (laib DEKONSTRUKTION: Lichter,
// Heizung, Rollläden, Medien), and the locks: a door lock is a device of
// the house too (owner 2026-11-03, the Nuki was missing). A lock's row
// only leads to its own page; opening a door is never one tap on a list.
var roomKinds = map[string]int{"light": 0, "climate": 1, "cover": 2, "media_player": 3, "lock": 4}

// maxRoomRows is the most one room card lists.
const maxRoomRows = 40

// noRoom is the card for what HA puts in no area.
const noRoom = "Ohne Raum"

// Raeume is one card per Home Assistant area (haus#12): its lights,
// heating, covers and media, the area the entity's own or else its
// device's; what has none in one "Ohne Raum" card at the end. A lamp is
// switched right on its row, and every thing is put on the person's
// Start with its ☆ (haus#12) — the forms of the thing's own page
// (haus#35), landing back on the thing's room. HA not answering, or its
// rooms not readable yet, leaves the cards out for one plain line
// (haus#33): a card that put every lamp in "Ohne Raum" because the
// registry failed would be wrong, not just short.
func Raeume(cfg StartConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && cfg.HA != nil { // a room's standard speaker (owner 2026-11-12)
			raeumeStandard(w, r, cfg)
			return
		}
		if !showOnly(w, r, "Räume") {
			return
		}
		if cfg.HA == nil {
			notSetHA.write(w, r, cfg, "Räume")
			return
		}
		writeRaeume(w, r, cfg, "")
	}
}

// writeRaeume is Geräte; saved is the room whose standard speaker was just chosen.
func writeRaeume(w http.ResponseWriter, r *http.Request, cfg StartConfig, saved string) {
	var (
		states  []ha.State
		haErr   error
		reg     ha.Registry
		roomsOK bool
		favs    []string
		favErr  error
	)
	person := r.Header.Get("Remote-User")
	pins := cfg.Favorites != nil && person != ""
	reads := []func(context.Context){
		func(ctx context.Context) { states, haErr = cfg.HA.States(ctx) },
		func(ctx context.Context) { reg, roomsOK = cfg.HA.CachedRegistry(ctx) }}
	if pins {
		reads = append(reads, func(context.Context) { favs, favErr = cfg.Favorites.For(person) })
	}
	fetch(r, reads...)
	states = cfg.allAsSeen(states) // a cover wired the wrong way round, turned over (haus#64)
	logErrs("räume", map[string]error{"home assistant": haErr, "favorites": favErr})
	if haErr == nil && !roomsOK {
		log.Printf("homes: räume page: home assistant's rooms are not known yet; the cards are left out")
	}
	b := familyHead(r, cfg, "Geräte", "Räume")
	b.WriteString(liveRefresh("seite", 120))
	b.WriteString(standardStyle + "<h1>Geräte</h1>") // the tab's name (owner 2026-10-24)
	if haErr == nil {                                // the way on only while HA answers: a line about it never names it (laib ADR 0021)
		b.WriteString(cfg.homeAssistant().Weiter("/", saysInHA))
	}
	switch {
	case haErr != nil:
		b.WriteString("<p class=k>" + saysNoHouse + "</p>")
	case !roomsOK:
		b.WriteString("<p class=k>" + saysNoRooms + "</p>")
	default:
		var pinned map[string]bool
		switch {
		case pins && favErr != nil:
			b.WriteString("<p class=k>" + saysNoFavorites + "</p>")
		case pins:
			b.WriteString("<p class=k>" + saysPinWhat + "</p>")
			pinned = map[string]bool{}
			for _, id := range favs {
				pinned[id] = true
			}
		}
		playing := laeuftGerade(states, reg, "/raeume", pinned)
		if len(playing.Rows) > 0 {
			b.WriteString(playing.HTML()) // what plays first, with its buttons: Medien's card (owner 2026-10-24)
		}
		standard := standardZeilen(states, reg, isAdmin(r), saved)
		now, loc := cfg.clock()
		for _, c := range roomCards(states, reg, pinned, playingIDs(playing), printerRows(states, reg, now, loc, cfg.HA)) {
			h := c.HTML()
			if end := strings.LastIndex(h, "</ul>"); end >= 0 {
				h = h[:end] + standard[c.Title] + h[end:] // the room's standard speaker, its last row
			}
			b.WriteString(h)
		}
	}
	writePage(w, http.StatusOK, b)
}

// saysPinWhat is what Räume's ☆ does, once above the rooms.
const saysPinWhat = "Mit ☆ steht ein Gerät auf Start unter „Deine Geräte“."

// saysInHA is what Home Assistant has that Homes does not (haus#64).
const saysInHA = "Für alles, was Homes nicht kann: Automationen, Einstellungen, Verlauf."

// roomCards groups the states by room: rooms by name, "Ohne Raum" last;
// in a room lights, heating, covers, media, locks, each by name, each
// leading to its own page; a lamp HA reaches with its switch; then the
// extra rows (a printer, owner 2026-11-14). pinned is the person's Start
// (nil: no ☆). What HA hides (hidden, config, diagnostic) is left out.
func roomCards(states []ha.State, reg ha.Registry, pinned, playing map[string]bool, extra []roomExtra) []cards.Card {
	byRoom := map[string][]roomThing{}
	twins := twinPlayers(states, reg) // one tile per thing (haus#83)
	for _, s := range states {
		domain, _, _ := strings.Cut(s.EntityID, ".")
		kind, ok := roomKinds[domain]
		if !ok {
			continue
		}
		if _, group := s.Attributes["entity_id"].([]any); group {
			continue // a group is not a thing in a room; its members are
		}
		if reg.Hidden[s.EntityID] || twins[s.EntityID] || playing[s.EntityID] { // what plays is on top, once
			continue
		}
		room := oneLine(reg.Rooms[s.EntityID])
		byRoom[room] = append(byRoom[room], roomThing{kind, s.EntityID, friendly(s), roomDetail(domain, s, reg.TempUnit), s, nil})
	}
	for _, e := range extra {
		byRoom[e.room] = append(byRoom[e.room], roomThing{kind: len(roomKinds), name: e.name, row: &e.row})
	}
	names := make([]string, 0, len(byRoom))
	for n := range byRoom {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "") != (names[j] == "") {
			return names[j] == ""
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	out := make([]cards.Card, 0, len(names))
	for _, n := range names {
		things := byRoom[n]
		sort.SliceStable(things, func(i, j int) bool {
			if things[i].kind != things[j].kind {
				return things[i].kind < things[j].kind
			}
			return strings.ToLower(things[i].name) < strings.ToLower(things[j].name)
		})
		title := trimRunes(n, 60)
		if n == "" {
			title = noRoom
		}
		c := cards.Card{Title: title}
		back := "/raeume#" + c.ID()
		for i, t := range things {
			if i == maxRoomRows {
				break
			}
			c.Rows = append(c.Rows, t.cardRow(pinned, back))
		}
		more(&c, len(things)-maxRoomRows)
		out = append(out, c)
	}
	return out
}

// roomThing is one thing on a room card; row, when set, is its row as it is.
type roomThing struct {
	kind             int
	id, name, detail string
	state            ha.State
	row              *cards.Row
}

// roomExtra is a row a room card shows that is not one entity's (a printer).
type roomExtra struct {
	room, name string
	row        cards.Row
}

// cardRow is the thing's row: a lamp switched and dimmed on it, a cover's
// buttons and position (haus#64), the ☆ when pinned is set.
func (t roomThing) cardRow(pinned map[string]bool, back string) cards.Row {
	if t.row != nil {
		return *t.row
	}
	row := cards.Row{Text: t.name, Detail: t.detail, Href: cards.ThingPage(t.id)}
	switch domain, _, _ := strings.Cut(t.id, "."); domain {
	case "light": // a tap switches; a dimmer has its slider too (haus#64)
		row.Action = umschalten(t.state, back)
		row.Slider = dimmer(t.state, back)
	case "cover": // Hoch, Stopp, Runter and the position (haus#64)
		if bs := coverButtons(t.state, back); len(bs) > 0 {
			row.Also = bs
		}
		row.Slider = coverSlider(t.state, back)
	}
	if pinned != nil {
		row.Pin = &cards.Pin{Entity: t.id, Name: t.name, On: pinned[t.id], Back: back}
	}
	return row
}

// roomDetail says a thing's state in the family's words: the room kinds,
// and the other things a widget shows (a thing's own page, haus#28).
// houseUnit is HA's temperature unit; an entity's own temperature_unit
// wins, °C is the last resort.
func roomDetail(domain string, s ha.State, houseUnit string) string {
	switch s.State {
	case "unavailable":
		return "nicht erreichbar"
	case "unknown", "":
		return "unbekannt"
	}
	switch domain {
	case "light":
		switch s.State {
		case "on":
			if v, ok := number(s.Attributes["brightness"]); ok {
				return "an · " + strconv.Itoa(int(math.Round(v/255*100))) + " %"
			}
			return "an"
		case "off":
			return "aus"
		}
	case "climate":
		unit := tempUnit(s, houseUnit)
		var parts []string
		if s.State == "off" {
			parts = append(parts, "aus")
		}
		if v, ok := number(s.Attributes["current_temperature"]); ok {
			parts = append(parts, decimal(v)+" "+unit)
		}
		if v, ok := number(s.Attributes["temperature"]); ok && s.State != "off" {
			parts = append(parts, "Soll "+decimal(v)+" "+unit)
		}
		if len(parts) == 0 {
			return "an"
		}
		return strings.Join(parts, " · ")
	case "cover":
		word := map[string]string{"open": "offen", "closed": "geschlossen", "opening": "öffnet", "closing": "schließt", "stopped": "angehalten"}[s.State]
		if word == "" {
			break
		}
		if v, ok := number(s.Attributes["current_position"]); ok && s.State == "open" && v < 100 {
			word += " · " + strconv.Itoa(int(math.Round(v))) + " %"
		}
		return word
	case "media_player":
		word := map[string]string{"playing": "spielt", "paused": "pausiert", "idle": "bereit", "on": "an", "off": "aus", "standby": "Standby", "buffering": "lädt"}[s.State]
		if word == "" {
			break
		}
		if s.State == "playing" || s.State == "paused" {
			title, _ := s.Attributes["media_title"].(string)
			artist, _ := s.Attributes["media_artist"].(string)
			what := trimRunes(oneLine(title), 80)
			if a := trimRunes(oneLine(artist), 60); a != "" {
				if what != "" {
					what += " · "
				}
				what += a
			}
			if what != "" {
				word += ": " + what
			}
		}
		return word
	case "switch":
		if w := map[string]string{"on": "an", "off": "aus"}[s.State]; w != "" {
			return w
		}
	case "binary_sensor":
		dc, _ := s.Attributes["device_class"].(string)
		words := map[string]string{"on": "an", "off": "aus"}
		if openings[dc] {
			words = map[string]string{"on": "offen", "off": "geschlossen"}
		}
		if w := words[s.State]; w != "" {
			return w
		}
	case "lock":
		if w := map[string]string{"locked": "verriegelt", "unlocked": "offen", "locking": "verriegelt gerade", "unlocking": "öffnet",
			"jammed": "klemmt", "open": "offen", "opening": "öffnet"}[s.State]; w != "" {
			return w
		}
	case "sensor":
		if u, _ := s.Attributes["unit_of_measurement"].(string); trimRunes(oneLine(u), 8) != "" {
			return trimRunes(oneLine(s.State), 40) + " " + trimRunes(oneLine(u), 8)
		}
	}
	return trimRunes(oneLine(s.State), 40)
}

// tempUnit is a heating's temperature unit: its own temperature_unit,
// else the house's (HA's unit system), °C as the last resort.
func tempUnit(s ha.State, houseUnit string) string {
	if u, _ := s.Attributes["temperature_unit"].(string); trimRunes(oneLine(u), 8) != "" {
		return trimRunes(oneLine(u), 8)
	}
	if houseUnit != "" {
		return trimRunes(oneLine(houseUnit), 8)
	}
	return "°C"
}

// openings are the binary sensors that say open or closed, not on or off.
var openings = map[string]bool{"door": true, "garage_door": true, "opening": true, "window": true}

// number reads an attribute HA sends as a JSON number.
func number(v any) (float64, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// decimal is a number the German way, to one place: 21,5 · 22.
func decimal(v float64) string {
	return strings.Replace(strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64), ".", ",", 1)
}

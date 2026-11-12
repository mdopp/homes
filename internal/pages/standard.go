package pages

import (
	"context"
	"errors"
	"html"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/ha"
)

// A room's standard speaker (owner 2026-11-12: „ein Ausgabegerät in einem Raum zum Standard
// erheben … konfigurierbar im System“) is HA's label ha.StandardLabel on the Music Assistant
// player there, in HA for anyone to see (laib ADR 0017); laib's home-assistant template plays
// music asked for by voice on it. Geräte shows it under the room's name; an admin chooses it,
// as it is the household's, not one person's.

const (
	saysStandard    = "Standard-Lautsprecher"
	saysAutomatisch = "automatisch"
)

// standardWahl is a speaker a room's standard can be: Music Assistant's entity, the thing's name.
type standardWahl struct{ entity, name string }

// standardWahlen are a room's speakers Music Assistant plays on, one per thing (its Cast twin
// and it are one, haus#83), by name. What Music Assistant does not play (a Cast alone, a voice
// satellite's speaker) cannot be one: the voice plays through Music Assistant.
func standardWahlen(states []ha.State, reg ha.Registry, room string) (out []standardWahl) {
	ma := map[string]string{} // twin key → Music Assistant's player
	for _, s := range states {
		if reg.Platforms[s.EntityID] == "music_assistant" && reg.Twins[s.EntityID] != "" {
			ma[reg.Twins[s.EntityID]] = s.EntityID
		}
	}
	for _, s := range states {
		entity := s.EntityID
		if reg.Platforms[entity] != "music_assistant" {
			entity = ma[reg.Twins[entity]]
		}
		if strings.HasPrefix(s.EntityID, "media_player.") && reg.Rooms[s.EntityID] == room && !reg.Hidden[s.EntityID] && entity != "" &&
			!slices.ContainsFunc(out, func(c standardWahl) bool { return c.entity == entity }) {
			out = append(out, standardWahl{entity, friendly(s)})
		}
	}
	slices.SortFunc(out, func(a, b standardWahl) int { return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)) })
	return out
}

// standardZeilen is each room's line under its name, by the card's title: for an admin the
// choice, for the others the standard once one is set; none where Music Assistant plays nowhere.
func standardZeilen(states []ha.State, reg ha.Registry, admin bool) map[string]string {
	out := map[string]string{}
	for room := range reg.AreaIDs {
		wahl := standardWahlen(states, reg, room)
		jetzt := slices.IndexFunc(wahl, func(c standardWahl) bool { return reg.Standard(c.entity) })
		switch {
		case len(wahl) > 0 && admin:
			var b strings.Builder
			b.WriteString(`<form method=post action=/raeume class=standard><input type=hidden name=raum value="` + html.EscapeString(room) + `"><label>` +
				saysStandard + ` <select name=standard onchange="this.form.requestSubmit()"><option value="">` + saysAutomatisch + `</option>`)
			for i, c := range wahl {
				b.WriteString(`<option value="` + html.EscapeString(c.entity) + `"` + map[bool]string{true: " selected"}[i == jetzt] + ">" + html.EscapeString(c.name) + "</option>")
			}
			out[trimRunes(room, 60)] = b.String() + `</select></label> <button data-laeuft="Speichert …">Speichern</button></form>`
		case jetzt >= 0:
			out[trimRunes(room, 60)] = "<p class=hinweis>" + saysStandard + ": " + html.EscapeString(wahl[jetzt].name) + "</p>"
		}
	}
	return out
}

// raeumeStandard takes an admin's choice (raum; standard, one of standardWahlen or "" for
// automatisch), writes it into HA and shows Geräte with what changed.
func raeumeStandard(w http.ResponseWriter, r *http.Request, cfg StartConfig) {
	if !fromHomes(r) || !isAdmin(r) {
		http.Error(w, "Den Standard-Lautsprecher eines Raums wählt ein Admin, auf Homes' eigener Seite.", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second) // a few writes and the rooms read again
	defer cancel()
	room, entity := r.PostFormValue("raum"), r.PostFormValue("standard")
	states, err := cfg.HA.States(ctx)
	reg, ok := cfg.HA.CachedRegistry(ctx)
	if err != nil || !ok {
		http.Error(w, saysNoHouse, http.StatusServiceUnavailable)
		return
	}
	wahl := standardWahlen(states, reg, room)
	i := slices.IndexFunc(wahl, func(c standardWahl) bool { return c.entity == entity })
	if reg.AreaIDs[room] == "" || (entity != "" && i < 0) {
		http.Error(w, "Diesen Lautsprecher gibt es in diesem Raum nicht mehr. Lade die Seite neu.", http.StatusConflict)
		return
	}
	players, area := []string{}, ""
	for e := range reg.Labels {
		if reg.Standard(e) && reg.Rooms[e] == room {
			players = append(players, e)
		}
	}
	for _, c := range wahl {
		players = append(players, c.entity)
	}
	if entity != "" && reg.Rooms[entity] == "" { // Music Assistant's player has no room; the voice finds it by one
		area = reg.AreaIDs[room]
	}
	if err := cfg.HA.SetStandard(ctx, entity, area, players); err != nil {
		log.Printf("homes: räume: standard speaker of %s: %v", room, err)
		say := "Home Assistant hat den Standard-Lautsprecher gerade nicht übernommen. Versuch es gleich noch einmal."
		if errors.Is(err, ha.ErrNotAdmin) {
			say = "Home Assistant lässt Homes keine Labels setzen: Homes' Benutzer dort ist kein Administrator."
		}
		http.Error(w, say, http.StatusBadGateway)
		return
	}
	log.Printf("homes: räume: %s set the standard speaker of %s to %q", r.Header.Get("Remote-User"), room, entity)
	said := room + ": " + saysStandard + " " + saysAutomatisch + "."
	if i >= 0 {
		said = room + ": " + saysStandard + " ist jetzt " + wahl[i].name + "."
	}
	writeRaeume(w, r, cfg, said)
}

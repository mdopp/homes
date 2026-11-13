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

	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/ha"
)

// A room's standard speaker (owner 2026-11-12: „ein Ausgabegerät in einem Raum zum Standard
// erheben … konfigurierbar im System“) is HA's label ha.StandardLabel on the Music Assistant
// player there, in HA for anyone to see (laib ADR 0017); laib's home-assistant template plays
// music asked for by voice on it. Geräte shows it as the room card's last row; an admin chooses
// it, as it is the household's, not one person's.

const saysAutomatisch = "automatisch"

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

// standardZeilen is each room's last row, by the card's title, where the choice matters: two or
// more speakers Music Assistant plays on (one is the room's anyway, owner 2026-11-13). An admin
// chooses there, sent on change (homesScript; a no-script page keeps a button), and after the
// room's choice (saved) the row says so; the others read the standard once one is set.
func standardZeilen(states []ha.State, reg ha.Registry, admin bool, saved string) map[string]string {
	out := map[string]string{}
	for room := range reg.AreaIDs {
		wahl := standardWahlen(states, reg, room)
		jetzt := slices.IndexFunc(wahl, func(c standardWahl) bool { return reg.Standard(c.entity) })
		if len(wahl) < 2 || (!admin && jetzt < 0) {
			continue
		}
		b := strings.Builder{}
		b.WriteString(`<li class="tu standard"><div>` + lautsprecherIcon + `<span class=was>Musik spielt auf</span>` + map[bool]string{true: `<span class=gespeichert role=status>Gespeichert</span>`}[room == saved])
		if !admin {
			out[trimRunes(room, 60)] = b.String() + `</div><span class=wahl>` + html.EscapeString(wahl[jetzt].name) + `</span></li>`
			continue
		}
		id := cards.Card{Title: room}.ID() + "-standard"
		b.WriteString(`</div><form method=post action="/raeume#` + id + `"><input type=hidden name=raum value="` + html.EscapeString(room) + `">` +
			`<select id=` + id + ` name=standard aria-label="Musik im Raum ` + html.EscapeString(room) + ` spielt auf" onchange="this.setAttribute('aria-busy','true');this.form.requestSubmit()">` +
			`<option value="">` + saysAutomatisch + `</option>`)
		for i, c := range wahl {
			b.WriteString(`<option value="` + html.EscapeString(c.entity) + `"` + map[bool]string{true: " selected"}[i == jetzt] + ">" + html.EscapeString(c.name) + "</option>")
		}
		out[trimRunes(room, 60)] = b.String() + `</select><noscript><button>Speichern</button></noscript></form></li>`
	}
	return out
}

// lautsprecherIcon is the standard's row's small speaker.
const lautsprecherIcon = `<svg viewBox="0 0 24 24" aria-hidden=true><path d="M4 9.5v5h3.5L12 18V6L7.5 9.5zM15.5 9a4 4 0 0 1 0 6M18 6.5a7.5 7.5 0 0 1 0 11"/></svg>`

// standardStyle is that row, as quiet as the room's other rows, the choice a small chip; over its
// words for a moment „Gespeichert“ after a choice, „Speichert …“ while one is sent.
const standardStyle = `<style>.karte li.standard>div{position:relative;flex:1 1 auto;display:flex;align-items:center;gap:8px;min-width:0;color:var(--muted);font-size:.9rem}.standard .wahl{flex:none;max-width:55%;text-align:right}
.standard svg{width:18px;height:18px;flex:none;fill:none;stroke:currentColor;stroke-width:1.8;stroke-linejoin:round;stroke-linecap:round}.standard form{flex:0 1 auto;min-width:0}.standard select{max-width:min(11rem,40vw);min-height:40px;padding:0 10px;border-radius:999px;background:var(--card);font-size:.9rem}.standard select[aria-busy=true]{opacity:.6}
.standard .gespeichert,.standard:has([aria-busy=true])>div::after{position:absolute;inset:0 0 0 26px;display:flex;align-items:center;background:var(--card);color:var(--ink)}.standard:has([aria-busy=true])>div::after{content:"Speichert …"}.gespeichert{animation:weg 4s forwards}@keyframes weg{0%,75%{opacity:1}100%{opacity:0;visibility:hidden}}</style>`

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
	writeRaeume(w, r, cfg, room)
}

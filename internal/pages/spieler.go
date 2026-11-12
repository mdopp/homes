package pages

import (
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/mdopp/homes/internal/cards"
	"github.com/mdopp/homes/internal/ha"
)

// saysNoPlayers is what Was wird genutzt says when the players do not answer.
const saysNoPlayers = "Lautsprecher und Fernseher antworten gerade nicht."

// HA's media player features (MediaPlayerEntityFeature) a button needs.
const (
	playFeature       = 16384
	volumeSetFeature  = 4
	volumeStepFeature = 1024
)

// The players' words and buttons, on Start's and Räume's „Läuft gerade“
// (the page Medien went with homes#362: nothing led to it).

// twinPlayers is the media players to leave out, so a thing is one tile
// (haus#83): of twins (one Chromecast through Cast and through Music
// Assistant, ha.Registry.Twins) all but the one to steer — the one that
// plays, then the paused one, then the device's own integration over
// Music Assistant's mirror, which can only steer what Music Assistant
// plays itself. A Jellyfin session is someone's app, not a thing in the
// house: always left out.
func twinPlayers(states []ha.State, reg ha.Registry) map[string]bool {
	rank := func(s ha.State) int {
		r := 0
		switch s.State {
		case "playing":
			r = 4
		case "paused":
			r = 2
		}
		if reg.Platforms[s.EntityID] != "music_assistant" {
			r++
		}
		return r
	}
	out := map[string]bool{}
	best := map[string]ha.State{}
	for _, s := range states {
		if !strings.HasPrefix(s.EntityID, "media_player.") {
			continue
		}
		if reg.Platforms[s.EntityID] == "jellyfin" {
			out[s.EntityID] = true
			continue
		}
		k := reg.Twins[s.EntityID]
		if k == "" {
			continue
		}
		b, ok := best[k]
		switch {
		case !ok:
			best[k] = s
		case rank(s) > rank(b):
			out[b.EntityID] = true
			best[k] = s
		default:
			out[s.EntityID] = true
		}
	}
	return out
}

// playingWords is what a player plays, and where: its room (unless the
// player is named for it), "pausiert", the title or else the app, and its
// volume.
func playingWords(s ha.State, room string) string {
	var parts []string
	if room = trimRunes(oneLine(room), 60); room != "" && !strings.EqualFold(room, friendly(s)) {
		parts = append(parts, room)
	}
	if s.State == "paused" {
		parts = append(parts, "pausiert")
	}
	title, _ := s.Attributes["media_title"].(string)
	artist, _ := s.Attributes["media_artist"].(string)
	app, _ := s.Attributes["app_name"].(string)
	switch {
	case oneLine(title) != "":
		parts = append(parts, trimRunes(oneLine(title), 80))
		if a := trimRunes(oneLine(artist), 60); a != "" {
			parts = append(parts, a)
		}
	case oneLine(app) != "":
		parts = append(parts, trimRunes(oneLine(app), 40))
	}
	if v, ok := number(s.Attributes["volume_level"]); ok && v >= 0 && v <= 1 {
		parts = append(parts, "Lautstärke\u00a0"+strconv.Itoa(int(math.Round(v*100)))+"\u00a0%") // kept on one line
	}
	return strings.Join(parts, " · ")
}

// playerButtons are what a playing or paused player can take, by its
// features: Pause or Weiter, then Leiser and Lauter; each lands on back.
// None for a player that is neither playing nor paused. Also is never nil
// when there is a button, so even a lone Pause sits on its own line, like
// the others' buttons.
func playerButtons(s ha.State, back string) (*cards.Action, []*cards.Action) {
	f, _ := number(s.Attributes["supported_features"])
	has := func(bit int64) bool { return int64(f)&bit != 0 }
	button := func(label, service string) *cards.Action {
		return &cards.Action{Label: label, Entity: s.EntityID, Do: "media_player." + service, Back: back}
	}
	var all []*cards.Action
	switch {
	case s.State == "playing" && has(pauseFeature):
		all = append(all, button("Pause", "media_pause"))
	case s.State == "paused" && has(playFeature):
		all = append(all, button("Weiter", "media_play"))
	}
	if (s.State == "playing" || s.State == "paused") && (has(volumeSetFeature) || has(volumeStepFeature)) {
		all = append(all, button("Leiser", "volume_down"), button("Lauter", "volume_up"))
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all[0], append([]*cards.Action{}, all[1:]...)
}

// playingCard is Läuft gerade: each playing or paused player with its
// room and what it plays, and its buttons landing on back.
func playingCard(on []ha.State, reg ha.Registry, back string) cards.Card {
	c := cards.Card{Title: "Läuft gerade", Look: cards.Boxes}
	for _, s := range on {
		row := cards.Row{Text: friendly(s), Detail: playingWords(s, reg.Rooms[s.EntityID]), Href: cards.ThingPage(s.EntityID)}
		row.Action, row.Also = playerButtons(s, back)
		c.Rows = append(c.Rows, row)
	}
	return c
}

// laeuftGerade is Läuft gerade for Geräte: the players that play or are
// paused, one per thing, playing first; no rows when none does.
func laeuftGerade(states []ha.State, reg ha.Registry, back string, pinned map[string]bool) cards.Card {
	twins := twinPlayers(states, reg) // a Jellyfin session is someone's app, not a thing in the house
	var on []ha.State
	for _, s := range states {
		if strings.HasPrefix(s.EntityID, "media_player.") && shown(s, reg.Hidden) && !twins[s.EntityID] && (s.State == "playing" || s.State == "paused") {
			on = append(on, s)
		}
	}
	sort.SliceStable(on, func(i, j int) bool {
		if (on[i].State == "playing") != (on[j].State == "playing") {
			return on[i].State == "playing"
		}
		return strings.ToLower(friendly(on[i])) < strings.ToLower(friendly(on[j]))
	})
	c := playingCard(on, reg, back)
	if pinned != nil { // its ☆ goes with it, as in its room
		for i, s := range on {
			c.Rows[i].Pin = &cards.Pin{Entity: s.EntityID, Name: friendly(s), On: pinned[s.EntityID], Back: back}
		}
	}
	return c
}

// playingIDs are the players a Läuft gerade card shows.
func playingIDs(c cards.Card) map[string]bool {
	ids := map[string]bool{}
	for _, r := range c.Rows {
		if id, ok := strings.CutPrefix(r.Href, cards.ThingPages); ok {
			if u, err := url.PathUnescape(id); err == nil {
				ids[u] = true
			}
		}
	}
	return ids
}

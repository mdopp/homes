package napi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/ha"
)

// The device picker and the app-icon menu read /napi/portal/start/addable:
// the actuators a device widget switches (actuatorDomains).

// GET /napi/portal/start/addable → {ok, rooms:[{room, cards:[{entity_id, name,
// domain, state, device_class?}]}], automations:[]}: rooms by name, the
// entities without one last, cards by name.
func (h *homeAPI) addable(w http.ResponseWriter, r *http.Request) {
	states, err := h.ha.States(r.Context())
	if err != nil {
		refuse(w, http.StatusBadGateway, "ha_unavailable")
		return
	}
	// rooms only label, they never break the list: the shared registry cache (#12), empty at first
	actx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	reg, _ := h.ha.CachedRegistry(actx)
	cancel()
	areas := reg.Rooms
	rooms := map[string][]map[string]any{}
	for _, s := range states {
		domain, _, _ := strings.Cut(s.EntityID, ".")
		if !actuatorDomains[domain] || !ha.EntityRE.MatchString(s.EntityID) {
			continue
		}
		name, _ := s.Attributes["friendly_name"].(string)
		if name == "" {
			name = s.EntityID
		}
		c := map[string]any{"entity_id": s.EntityID, "name": name, "domain": domain, "state": s.State}
		if dc, _ := s.Attributes["device_class"].(string); dc != "" {
			c["device_class"] = dc
		}
		room := areas[s.EntityID]
		rooms[room] = append(rooms[room], c)
	}
	names := make([]string, 0, len(rooms))
	for k := range rooms {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "") != (names[j] == "") {
			return names[j] == ""
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	out := []map[string]any{}
	for _, room := range names {
		cards := rooms[room]
		sort.SliceStable(cards, func(i, j int) bool {
			return strings.ToLower(cards[i]["name"].(string)) < strings.ToLower(cards[j]["name"].(string))
		})
		out = append(out, map[string]any{"room": room, "cards": cards})
	}
	answer(w, http.StatusOK, map[string]any{"ok": true, "rooms": out, "automations": []any{}})
}

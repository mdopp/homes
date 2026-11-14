package napi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mdopp/homes/internal/favoriten"
	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/ha/hafake"
)

// an Epson through HA's IPP integration, as on the box (2026-11-14)
func epson(fake *hafake.HA, state string) {
	fake.SetRegistries(nil, []map[string]any{{"id": "dev-epson"}}, []map[string]any{
		{"entity_id": "sensor.epson_wf_3540_series", "device_id": "dev-epson", "platform": "ipp"},
		{"entity_id": "sensor.epson_wf_3540_series_black_ink", "device_id": "dev-epson", "platform": "ipp"},
		{"entity_id": "sensor.temp"}})
	fake.Set("sensor.epson_wf_3540_series", state, map[string]any{"friendly_name": "EPSON WF-3540 Series", "device_class": "enum"})
	black := "42"
	if state == "unavailable" {
		black = "unavailable"
	}
	fake.Set("sensor.epson_wf_3540_series_black_ink", black, map[string]any{"friendly_name": "EPSON WF-3540 Series Black ink", "unit_of_measurement": "%"})
}

// The owner (2026-11-14): „Bitte in der App und auf der Webseite gleich.“ A
// pinned printer's card carries Geräte's words in text, on the favorites
// and on its own state; another sensor has none.
func TestPrinterCardText(t *testing.T) {
	store := favoriten.Open(t.TempDir())
	for _, id := range []string{"sensor.epson_wf_3540_series", "sensor.temp"} {
		store.Set("anna", id, true)
	}
	for state, want := range map[string]string{"printing": "druckt · Schwarz 42 %", "idle": "bereit · Schwarz 42 %",
		"unavailable": "nicht erreichbar"} {
		fake, h := houseWith(t, func(c *Config) {
			c.Favorites = store
			c.Whoami = func(string) (string, bool) { return "anna", true }
		})
		epson(fake, state)
		_, body := raw(t, h, "GET", "/napi/portal/favorites", "dev1", "")
		var out struct {
			Favorites []map[string]any `json:"favorites"`
		}
		json.Unmarshal([]byte(body), &out)
		if len(out.Favorites) != 2 || out.Favorites[0]["text"] != want || out.Favorites[1]["text"] != nil {
			t.Errorf("%s: favorites %s", state, body)
		}
		_, body = raw(t, h, "GET", "/napi/portal/state?entity_id=sensor.epson_wf_3540_series", "dev1", "")
		var one struct {
			Card map[string]any `json:"card"`
		}
		json.Unmarshal([]byte(body), &one)
		if one.Card["text"] != want || one.Card["state"] != state {
			t.Errorf("%s: state %s", state, body)
		}
	}
}

// The stream's card_state carries the same words, from the states it has
// seen; an ink's change makes its printer's words new.
func TestPrinterCardTextLive(t *testing.T) {
	fake := hafake.New("ha-token")
	defer fake.Close()
	epson(fake, "printing")
	c := &ha.Client{URL: fake.URL, Token: "ha-token"}
	if _, ok := c.CachedRegistry(context.Background()); !ok {
		t.Fatal("registry")
	}
	states, _ := c.States(context.Background())
	l := newLive(c, 0, 0)
	l.mu.Lock()
	defer l.mu.Unlock()
	var status ha.State
	for _, s := range states {
		l.card(s)
		if s.EntityID == "sensor.epson_wf_3540_series" {
			status = s
		}
	}
	if got := l.card(status)["text"]; got != "druckt · Schwarz 42 %" {
		t.Errorf("live text %v", got)
	}
	l.states["sensor.epson_wf_3540_series_black_ink"] = ha.State{EntityID: "sensor.epson_wf_3540_series_black_ink", State: "9",
		Attributes: map[string]any{"friendly_name": "EPSON WF-3540 Series Black ink", "unit_of_measurement": "%"}}
	if got := l.card(status)["text"]; got != "druckt · Schwarz fast leer · 9 %" {
		t.Errorf("after the ink: %v", got)
	}
	if got := l.card(ha.State{EntityID: "sensor.temp", State: "21", Attributes: map[string]any{}})["text"]; got != nil {
		t.Errorf("a temperature has text %v", got)
	}
}

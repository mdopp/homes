package napi

import (
	"encoding/json"
	"testing"

	"github.com/mdopp/homes/internal/favoriten"
	"github.com/mdopp/homes/internal/ha"
)

// homes-android#290: a device the app shows says why it needs a look in
// "achtung" — HA does not reach it, its battery is low, a printer's own
// (paper, ink, unreachable) — and has none otherwise.
func TestAchtung(t *testing.T) {
	for _, c := range []struct {
		s    ha.State
		want string
	}{
		{ha.State{EntityID: "light.flur", State: "unavailable"}, "nicht erreichbar"},
		{ha.State{EntityID: "light.flur", State: "on"}, ""},
		{ha.State{EntityID: "sensor.tuer_batterie", State: "12", Attributes: map[string]any{"device_class": "battery"}}, "Batterie fast leer"},
		{ha.State{EntityID: "sensor.tuer_batterie", State: "80", Attributes: map[string]any{"device_class": "battery"}}, ""},
		{ha.State{EntityID: "binary_sensor.rauchmelder_batterie", State: "on", Attributes: map[string]any{"device_class": "battery"}}, "Batterie fast leer"},
		{ha.State{EntityID: "binary_sensor.rauchmelder_batterie", State: "off", Attributes: map[string]any{"device_class": "battery"}}, ""},
		{ha.State{EntityID: "sensor.temp", State: "unknown"}, ""},
	} {
		got, _ := card(c.s)["achtung"].(string)
		if got != c.want {
			t.Errorf("%s %s: %q", c.s.EntityID, c.s.State, got)
		}
	}
}

func TestPrinterAchtung(t *testing.T) {
	store := favoriten.Open(t.TempDir())
	store.Set("anna", "sensor.epson_wf_3540_series", true)
	for _, c := range []struct {
		state, reason, black, want string
	}{
		{"idle", "", "42", ""},
		{"idle", "media-empty-error", "42", "Papier leer"},
		{"idle", "", "8", "Schwarz fast leer"},
		{"unavailable", "", "", "nicht erreichbar"},
	} {
		fake, h := houseWith(t, func(cfg *Config) {
			cfg.Favorites = store
			cfg.Whoami = func(string) (string, bool) { return "anna", true }
		})
		epson(fake, c.state)
		if c.state != "unavailable" {
			fake.Set("sensor.epson_wf_3540_series", c.state, map[string]any{"friendly_name": "EPSON", "device_class": "enum", "state_reason": c.reason})
			fake.Set("sensor.epson_wf_3540_series_black_ink", c.black, map[string]any{"friendly_name": "EPSON Black ink", "unit_of_measurement": "%"})
		}
		_, body := raw(t, h, "GET", "/napi/portal/favorites", "dev1", "")
		var out struct {
			Favorites []map[string]any `json:"favorites"`
		}
		json.Unmarshal([]byte(body), &out)
		got, _ := out.Favorites[0]["achtung"].(string)
		if len(out.Favorites) != 1 || got != c.want {
			t.Errorf("%+v: %s", c, body)
		}
	}
}

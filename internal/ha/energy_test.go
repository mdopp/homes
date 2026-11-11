package ha

import (
	"context"
	"testing"

	"github.com/mdopp/homes/internal/ha/hafake"
)

// The energy dashboard's setup: the battery's two counters, the devices
// with the dashboard's name or none and the one they are part of; the
// grid and the sun are the history's own (HOMES_ENERGY_STATS). HA without
// a dashboard answers not_found: nothing, no error.
func TestEnergyPrefs(t *testing.T) {
	fake := hafake.New("tok")
	defer fake.Close()
	c := &Client{URL: fake.URL, Token: "tok"}
	p, err := c.EnergyPrefs(context.Background())
	if err != nil || len(p.Batteries) != 0 || len(p.Devices) != 0 {
		t.Fatalf("no dashboard: %+v %v, want nothing and no error", p, err)
	}
	fake.SetEnergyPrefs(map[string]any{
		"energy_sources": []any{
			map[string]any{"type": "grid", "flow_from": []any{map[string]any{"stat_energy_from": "sensor.bezug"}}},
			map[string]any{"type": "solar", "stat_energy_from": "sensor.pv"},
			map[string]any{"type": "battery", "stat_energy_from": "sensor.akku_aus", "stat_energy_to": "sensor.akku_ein"},
		},
		"device_consumption": []any{
			map[string]any{"stat_consumption": "sensor.wp", "name": "Wärmepumpe"},
			map[string]any{"stat_consumption": "sensor.dose", "included_in_stat": "sensor.wp"},
			map[string]any{"stat_consumption": "not an id"},
		},
	})
	p, err = c.EnergyPrefs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Batteries) != 1 || p.Batteries[0] != (Battery{Out: "sensor.akku_aus", In: "sensor.akku_ein"}) {
		t.Fatalf("batteries %+v", p.Batteries)
	}
	if len(p.Devices) != 2 || p.Devices[0] != (Device{"sensor.wp", "Wärmepumpe", ""}) || p.Devices[1] != (Device{"sensor.dose", "", "sensor.wp"}) {
		t.Fatalf("devices %+v", p.Devices)
	}
}

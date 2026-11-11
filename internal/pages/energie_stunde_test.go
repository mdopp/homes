package pages

import (
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/ha/hafake"
)

// The owner, 2026-11-11: „Bei Energie sollte angezeigt werden, was in der
// Stunde, die ich ausgewählt habe, auch verbraucht wurde.“ Without a
// battery or devices in HA's energy dashboard (it answers not_found): the
// latest hour HA has summed is chosen, its line says what was used and
// where it came from, every bar carries its own line for a tap, an hour
// still to come says so, the table has a Verbrauch column.
func TestEnergieStundeSaysWhatWasUsed(t *testing.T) {
	fake, hc := house(t)
	cfg := senecSince2June(t, fake) // hours 0–9: sun 0,3, into the grid 0,1, from it 0,2
	cfg.HA = hc
	_, body, _ := familyPage(t, Energie, cfg, "GET", "/energie", "family")
	line := "<p><b>9–10 Uhr:</b> 0,4 kWh verbraucht · 0,2 selbst erzeugt · 0,2 aus dem Netz · 0,1 ins Netz</p>"
	mustContain(t, body, "<div class=stunde aria-live=polite>"+line+"</div>",
		`<g class=gewaehlt aria-pressed=true tabindex=0 data-stunde=`, `data-zeile="&lt;p&gt;&lt;b&gt;9–10 Uhr:&lt;/b&gt; 0,4 kWh verbraucht`,
		`role=button aria-label="9–10 Uhr"`, `data-zeile="&lt;p&gt;&lt;b&gt;12–13 Uhr:&lt;/b&gt; noch keine Werte&lt;/p&gt;"`,
		`data-zeile="&lt;p&gt;&lt;b&gt;23–24 Uhr:`,
		"<th scope=col>Verbrauch</th>", "<tr><th scope=row>09 Uhr</th><td>0,3 kWh</td><td>0,1 kWh</td><td>0,2 kWh</td><td>0,4 kWh</td></tr>")
	if n := strings.Count(body, "aria-pressed=true"); n != 1 {
		t.Fatalf("%d hours chosen, want one", n)
	}
	mustNotContain(t, body, "class=geraete", "Akku")
	// the other spans: no hour to choose, no prefs asked
	_, body, _ = familyPage(t, Energie, cfg, "GET", "/energie?zeitraum=woche", "family")
	mustNotContain(t, body, "class=stunde", "data-zeile", "<th scope=col>Verbrauch</th>")
	mustContain(t, body, "role=img")
}

// With a battery and devices in the dashboard: the battery counts (out of
// it used, into it not), and the three devices that used most stand under
// the line with the rest — never a part of another device; an hour the
// devices have no numbers for has no device line. Only the day by the
// hour asks for their statistics.
func TestEnergieStundeWithBatteryAndDevices(t *testing.T) {
	fake, hc := house(t)
	cfg := senecSince2June(t, fake)
	cfg.HA = hc
	fake.SetEnergyPrefs(map[string]any{
		"energy_sources": []any{map[string]any{"type": "battery", "stat_energy_from": "sensor.akku_aus", "stat_energy_to": "sensor.akku_ein"}},
		"device_consumption": []any{
			map[string]any{"stat_consumption": "sensor.wp_energie"},
			map[string]any{"stat_consumption": "sensor.trockner", "name": "Trockner <unten>"},
			map[string]any{"stat_consumption": "sensor.kuehl"},
			map[string]any{"stat_consumption": "sensor.router"},
			map[string]any{"stat_consumption": "sensor.dose", "included_in_stat": "sensor.wp_energie"},
		},
	})
	kwh := map[string]any{"device_class": "energy", "unit_of_measurement": "kWh"}
	fake.Set("sensor.wp_energie", "100", map[string]any{"friendly_name": "Wärmepumpe", "device_class": "energy", "unit_of_measurement": "kWh"})
	fake.Set("sensor.kuehl", "10", map[string]any{"friendly_name": "Kühlschrank", "device_class": "energy", "unit_of_measurement": "kWh"})
	fake.Set("sensor.router", "1", kwh)
	nine := time.Date(2026, 10, 14, 9, 0, 0, 0, berlin(t))
	for id, v := range map[string]float64{"sensor.akku_aus": 0.3, "sensor.akku_ein": 0.1, "sensor.wp_energie": 0.3, "sensor.trockner": 0.2,
		"sensor.kuehl": 0.05, "sensor.router": 0.02, "sensor.dose": 0.25} {
		fake.SetStatistics(id, []hafake.Stat{{Start: nine, Change: v}})
	}
	_, body, _ := familyPage(t, Energie, cfg, "GET", "/energie", "family")
	mustContain(t, body, "<div class=stunde aria-live=polite><p><b>9–10 Uhr:</b> 0,6 kWh verbraucht · 0,1 selbst erzeugt · 0,3 aus dem Akku · 0,2 aus dem Netz · 0,1 ins Netz · 0,1 in den Akku</p>"+
		"<p class=geraete>Wärmepumpe 0,3 kWh · Trockner &lt;unten&gt; 0,2 kWh · Kühlschrank 0,05 kWh · Rest 0,05 kWh</p></div>",
		"<tr><th scope=row>09 Uhr</th><td>0,3 kWh</td><td>0,1 kWh</td><td>0,2 kWh</td><td>0,6 kWh</td></tr>",
		// 8 o'clock: no battery, no devices — the sun and the grid alone
		`data-zeile="&lt;p&gt;&lt;b&gt;8–9 Uhr:&lt;/b&gt; 0,4 kWh verbraucht · 0,2 selbst erzeugt · 0,2 aus dem Netz · 0,1 ins Netz&lt;/p&gt;"`)
	mustNotContain(t, body, "sensor.dose", "Router", "<unten>")
	for _, a := range fake.StatisticsAsked() {
		ids := a["statistic_ids"].([]any)
		if a["period"] == "day" && len(ids) != 3 {
			t.Errorf("the days asked for %v, want the three counters alone", ids)
		}
		if a["period"] == "hour" && len(ids) != 10 {
			t.Errorf("the hours asked for %v, want the counters, the battery's two and the five devices", ids)
		}
	}
}

// The scale says what it shows (owner, 2026-11-11: the bottom read „0,1 kWh“
// under the zero line, where the feed-in hangs): the top, a 0 at the
// line, the bottom negative.
func TestEnergyChartAxisIsHonest(t *testing.T) {
	bs := []bucket{{v: map[string]float64{"solar": 0.5, "bezug": 0.2, "einsp": 0.1}}, {v: map[string]float64{"solar": 0.1}}}
	svg := energyChart("Heute", bs, -1, "", nil)
	mustContain(t, svg, ">0,7 kWh</text>", ">−0,1 kWh</text>", ">0</text>")
	mustNotContain(t, svg, ">0,1 kWh</text>")
	// nothing went out: the 0 is the bottom, no negative number
	svg = energyChart("Heute", []bucket{{v: map[string]float64{"solar": 2}}}, -1, "", nil)
	mustContain(t, svg, ">2 kWh</text>", ">0</text>")
	mustNotContain(t, svg, "−")
	// a sliver went out: its number stays, the 0 that would sit on it goes
	svg = energyChart("Heute", []bucket{{v: map[string]float64{"solar": 9, "einsp": 0.1}}}, -1, "", nil)
	mustContain(t, svg, ">9 kWh</text>", ">−0,1 kWh</text>")
	mustNotContain(t, svg, ">0</text>")
	// a hundredth went out: its number, not "−0"
	svg = energyChart("Heute", []bucket{{v: map[string]float64{"solar": 0.4, "einsp": 0.01}}}, -1, "", nil)
	mustContain(t, svg, ">−0,01 kWh</text>")
	mustNotContain(t, svg, ">−0 kWh</text>")
}

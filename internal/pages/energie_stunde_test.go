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
		`<g aria-pressed=true tabindex=0 data-stunde=`, `data-zeile="&lt;p&gt;&lt;b&gt;9–10 Uhr:&lt;/b&gt; 0,4 kWh verbraucht`,
		`role=button aria-label="9–10 Uhr"`, `data-zeile="&lt;p&gt;&lt;b&gt;12–13 Uhr:&lt;/b&gt; noch keine Werte&lt;/p&gt;"`,
		`data-zeile="&lt;p&gt;&lt;b&gt;23–24 Uhr:`,
		"<th scope=col>Verbrauch</th>", "<tr><th scope=row>09 Uhr</th><td>0,3 kWh</td><td>0,1 kWh</td><td>0,2 kWh</td><td>0,4 kWh</td></tr>")
	if n := strings.Count(body, "aria-pressed=true tabindex=0"); n != 1 {
		t.Fatalf("%d hours chosen, want one", n)
	}
	mustNotContain(t, body, "class=geraete", "Akku")
	// the other spans too (owner, 2026-11-11: „Das sollte bei jedem
	// stehen“): today, the day HA has summed so far, is chosen
	_, body, _ = familyPage(t, Energie, cfg, "GET", "/energie?zeitraum=woche", "family")
	mustContain(t, body, "<div class=stunde aria-live=polite><p><b>Mittwoch, 14. Oktober:</b> 4 kWh verbraucht · 2 selbst erzeugt · 2 aus dem Netz · 1 ins Netz</p></div>",
		`role=button aria-label="Dienstag, 13. Oktober"`, "<th scope=col>Verbrauch</th>", "role=group aria-label=\"Die letzten sieben Tage")
}

// With a battery and devices in the dashboard: the battery counts (out of
// it used, into it not), and the three devices that used most stand under
// the line with the rest — never a part of another device; an hour the
// devices have no numbers for has no device line. The days ask for them
// too, for the other spans.
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
		if a["period"] == "day" && len(ids) != 10 {
			t.Errorf("the days asked for %v, want the counters, the battery's two and the five devices", ids)
		}
		if a["period"] == "hour" && len(ids) != 10 {
			t.Errorf("the hours asked for %v, want the counters, the battery's two and the five devices", ids)
		}
	}
}

// Woche and Jahr: a day's and a month's battery and devices come from the
// days HA has summed, a month's as the sum of its days; the estimate never
// counts, a month still to come has no values; the sun's share is what
// neither went into the grid nor into the battery.
func TestEnergieChosenDayAndMonth(t *testing.T) {
	fake, hc := house(t)
	cfg := senecSince2June(t, fake)
	cfg.HA = hc
	fake.SetEnergyPrefs(map[string]any{
		"energy_sources":     []any{map[string]any{"type": "battery", "stat_energy_from": "sensor.akku_aus", "stat_energy_to": "sensor.akku_ein"}},
		"device_consumption": []any{map[string]any{"stat_consumption": "sensor.wp_energie", "name": "Wärmepumpe"}},
	})
	loc := berlin(t)
	nine, first, second := time.Date(2026, 10, 14, 9, 0, 0, 0, loc), time.Date(2026, 10, 1, 12, 0, 0, 0, loc), time.Date(2026, 10, 2, 12, 0, 0, 0, loc)
	fake.SetStatistics("sensor.akku_aus", []hafake.Stat{{Start: first, Change: 1}, {Start: nine, Change: 0.3}})
	fake.SetStatistics("sensor.akku_ein", []hafake.Stat{{Start: nine, Change: 0.1}})
	fake.SetStatistics("sensor.wp_energie", []hafake.Stat{{Start: second, Change: 2}, {Start: nine, Change: 0.3}})
	// Woche: today, with its battery and device; 1 and 2 October are not in it
	_, body, _ := familyPage(t, Energie, cfg, "GET", "/energie?zeitraum=woche", "family")
	mustContain(t, body, "<div class=stunde aria-live=polite><p><b>Mittwoch, 14. Oktober:</b> 4,2 kWh verbraucht · 1,9 selbst erzeugt · 0,3 aus dem Akku · 2 aus dem Netz · 1 ins Netz · 0,1 in den Akku</p>"+
		"<p class=geraete>Wärmepumpe 0,3 kWh · Rest 3,9 kWh</p></div>",
		"<tr><th scope=row>Mittwoch, 14. Oktober</th><td>3 kWh</td><td>1 kWh</td><td>2 kWh</td><td>4,2 kWh</td></tr>",
		`data-zeile="&lt;p&gt;&lt;b&gt;Dienstag, 13. Oktober:&lt;/b&gt; 7 kWh verbraucht · 4 selbst erzeugt · 3 aus dem Netz · 6 ins Netz&lt;/p&gt;"`)
	// Jahr: October, its days summed — the battery's 1 + 0,3, the device's 2 + 0,3
	_, body, _ = familyPage(t, Energie, cfg, "GET", "/energie?zeitraum=jahr", "family")
	mustContain(t, body, "<div class=stunde aria-live=polite><p><b>Oktober 2026:</b> 96,2 kWh verbraucht · 53,9 selbst erzeugt · 1,3 aus dem Akku · 41 aus dem Netz · 79 ins Netz · 0,1 in den Akku</p>"+
		"<p class=geraete>Wärmepumpe 2,3 kWh · Rest 93,9 kWh</p></div>",
		`data-zeile="&lt;p&gt;&lt;b&gt;November 2026:&lt;/b&gt; noch keine Werte&lt;/p&gt;"`,
		`data-zeile="&lt;p&gt;&lt;b&gt;September 2026:&lt;/b&gt; 180 kWh verbraucht · 120 selbst erzeugt · 60 aus dem Netz · 180 ins Netz&lt;/p&gt;"`)
	if n := strings.Count(body, "aria-pressed=true tabindex=0"); n != 1 {
		t.Fatalf("%d months chosen, want one", n)
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

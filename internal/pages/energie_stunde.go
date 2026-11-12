package pages

import (
	"html"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/energie"
	"github.com/mdopp/homes/internal/ha"
)

// The chosen bar (owner, 2026-11-11: „Bei Energie sollte angezeigt
// werden, was in der Stunde, die ich ausgewählt habe, auch verbraucht
// wurde“, and „Das sollte bei jedem stehen“): under every span's chart one
// line says what the house used in the hour, day or month tapped — from
// the sun, the grid, the battery — and, when HA's energy dashboard counts
// devices, the three that used most. Only what was, never the estimate.
// The latest bar HA has values for is chosen at first; the table has a
// Verbrauch column.

// statSlot is one statistic the history reads, and the key its amounts
// are kept under.
type statSlot struct{ key, id string }

// historySlots are the history's three counters.
func historySlots(st energie.Stats) []statSlot {
	return []statSlot{{"solar", st.Solar}, {"bezug", st.Bezug}, {"einsp", st.Einspeisung}}
}

// stundeSlots are what the chosen bar reads besides them: each battery's
// counters and each device's, from HA's energy dashboard.
func stundeSlots(p ha.EnergyPrefs) []statSlot {
	var out []statSlot
	for _, b := range p.Batteries {
		out = append(out, statSlot{"entladen", b.Out}, statSlot{"geladen", b.In})
	}
	for _, d := range p.Devices {
		out = append(out, statSlot{"geraet:" + d.Stat, d.Stat})
	}
	return out
}

// geraet is one device the dashboard counts, by the name the family knows.
type geraet struct {
	stat, name string
	teil       bool
}

// geraeteOf are the dashboard's devices, each named as the dashboard names
// it, else as HA does (friendly_name), else by its id.
func geraeteOf(p ha.EnergyPrefs, states []ha.State) []geraet {
	names := map[string]string{}
	for _, s := range states {
		if n, _ := s.Attributes["friendly_name"].(string); n != "" {
			names[s.EntityID] = n
		}
	}
	var out []geraet
	for _, d := range p.Devices {
		name := d.Name
		if name == "" {
			name = names[d.Stat]
		}
		if name == "" {
			name = d.Stat
		}
		out = append(out, geraet{d.Stat, name, d.PartOf != ""})
	}
	return out
}

// wahlBalken are the chart's bars to choose from: each bar's line, what
// the house used (hat: HA has a number for that bar), and the bar chosen
// at first.
type wahlBalken struct {
	zeile     []string
	name      []string
	verbrauch []float64
	hat       []bool
	wahl      int
}

// wahlOf reads each bar's battery and devices from a (hourly on the Tag
// chart, else daily; a Jahr bar sums its month's days); the bar chosen at
// first is the latest one HA has values for, else the one now.
func wahlOf(view string, bs []bucket, a amounts, geraete []geraet, now time.Time) *wahlBalken {
	next := func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }
	switch view {
	case "tag":
		next = func(t time.Time) time.Time { return t.Add(time.Hour) }
	case "jahr":
		next = func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }
	}
	w := &wahlBalken{wahl: -1}
	jetzt := -1
	for i, x := range bs {
		at := time.Unix(x.at, 0).In(now.Location())
		sum := func(key string) (float64, bool) { return sumDays(a[key], at, next(at)) }
		s := energie.Stunde{Solar: x.v["solar"], Einsp: x.v["einsp"], Bezug: x.v["bezug"]}
		var e, g bool
		s.Entladen, e = sum("entladen")
		s.Geladen, g = sum("geladen")
		hat := len(x.v) > 0 || e || g
		for _, d := range geraete {
			kwh, _ := sum("geraet:" + d.stat)
			s.Geraete = append(s.Geraete, energie.Geraet{Name: d.name, KWh: kwh, Teil: d.teil})
		}
		name := x.long
		if view == "tag" {
			name = stundeName(at)
		}
		w.zeile, w.name = append(w.zeile, wahlZeile(name, s, hat)), append(w.name, name)
		w.verbrauch, w.hat = append(w.verbrauch, s.Verbrauch()), append(w.hat, hat)
		if hat && !at.After(now) {
			w.wahl = i
		}
		if !at.After(now) && now.Before(next(at)) {
			jetzt = i
		}
	}
	if w.wahl < 0 {
		w.wahl = max(jetzt, 0)
	}
	return w
}

// stundeName is an hour as the family says it: „12–13 Uhr“.
func stundeName(at time.Time) string {
	end := at.Add(time.Hour).Hour()
	if end == 0 {
		end = 24
	}
	return strconv.Itoa(at.Hour()) + "–" + strconv.Itoa(end) + " Uhr"
}

// wahlZeile is the line under the chart for one bar, by its name (HTML):
// what the house used and where it came from, then the devices that used
// most.
func wahlZeile(name string, s energie.Stunde, hat bool) string {
	head := "<p><b>" + html.EscapeString(name) + ":</b> "
	if !hat {
		return head + "noch keine Werte</p>"
	}
	parts := []string{stundeKWh(s.Verbrauch()) + " kWh verbraucht"}
	for _, p := range []struct {
		v     float64
		words string
	}{{s.Selbst(), "selbst erzeugt"}, {s.Entladen, "aus dem Akku"}, {s.Bezug, "aus dem Netz"}, {s.Einsp, "ins Netz"}, {s.Geladen, "in den Akku"}} {
		if p.v >= 0.005 {
			parts = append(parts, stundeKWh(p.v)+" "+p.words)
		}
	}
	line := head + strings.Join(parts, " · ") + "</p>"
	top, rest := s.Top(3)
	if len(top) == 0 {
		return line
	}
	var gs []string
	for _, g := range top {
		gs = append(gs, html.EscapeString(g.Name)+" "+stundeKWh(g.KWh)+" kWh")
	}
	if rest >= 0.005 {
		gs = append(gs, "Rest "+stundeKWh(rest)+" kWh")
	}
	return line + "<p class=geraete>" + strings.Join(gs, " · ") + "</p>"
}

// stundeKWh is a bar's amount: to two places below 0,1 (0,05), to one
// below 100 (0,9), else whole with the thousands marked (2.130).
func stundeKWh(v float64) string {
	if math.Abs(v) >= 99.95 {
		return strings.TrimSuffix(energyWords(v), " kWh")
	}
	if math.Round(v*100) < 10 {
		return strings.Replace(strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64), ".", ",", 1)
	}
	return decimal(v)
}

// bar opens bar i's group, of class cls: a button that chooses the bar,
// the chosen one pressed; without bars to choose as it was.
func (t *wahlBalken) bar(cls string, i int, x bucket) string {
	g := "<g"
	if cls != "" {
		g += " class=" + cls
	}
	if t == nil {
		return g + ">"
	}
	pressed := map[bool]string{true: " aria-pressed=true tabindex=0", false: " aria-pressed=false tabindex=-1"}[i == t.wahl]
	return g + pressed + ` data-stunde=` + strconv.FormatInt(x.at, 10) + ` data-zeile="` + html.EscapeString(t.zeile[i]) +
		`" role=button aria-label="` + html.EscapeString(t.name[i]) + `">`
}

// role is the chart's: a group of bars to choose from, or without them a
// picture.
func (t *wahlBalken) role() string {
	if t == nil {
		return "img"
	}
	return "group"
}

// cell is bar i's Verbrauch in the table; "" without bars to choose.
func (t *wahlBalken) cell(i int) string {
	switch {
	case t == nil:
		return ""
	case t.hat[i]:
		return "<td>" + energyWords(t.verbrauch[i]) + "</td>"
	}
	return "<td>–</td>"
}

// html is the chosen bar's line and the script that changes it.
func (t *wahlBalken) html() string {
	if t == nil || len(t.zeile) == 0 {
		return ""
	}
	return stundeStyle + "<div class=stunde aria-live=polite>" + t.zeile[t.wahl] + "</div>" + stundeScript
}

// stundeScript chooses a bar: a tap on its bar, or on the keyboard the
// arrows, Home and End (one tab stop, the chosen bar), Enter and Space.
// The choice outlives the page's own refresh (a new swap runs it again).
const stundeScript = `<script>(function(){var W=window;
function wahl(g,fokus){var f=g.closest('figure');f.querySelectorAll('g[data-zeile]').forEach(function(x){var an=x===g;x.setAttribute('aria-pressed',an);x.setAttribute('tabindex',an?0:-1)});
var z=f.querySelector('.stunde');if(z)z.innerHTML=g.getAttribute('data-zeile');W.homesStunde=g.getAttribute('data-stunde');if(fokus)g.focus()}
if(!W.homesStundeAn){W.homesStundeAn=1;
document.addEventListener('click',function(e){var g=e.target.closest&&e.target.closest('g[data-zeile]');if(g)wahl(g)});
document.addEventListener('keydown',function(e){var g=e.target.closest&&e.target.closest('g[data-zeile]');if(!g)return;
if(e.key==='Enter'||e.key===' '){e.preventDefault();wahl(g);return}
var gs=[].slice.call(g.parentNode.querySelectorAll('g[data-zeile]')),i=gs.indexOf(g),n={ArrowLeft:i-1,ArrowRight:i+1,Home:0,End:gs.length-1}[e.key];
if(n===undefined)return;e.preventDefault();wahl(gs[Math.max(0,Math.min(gs.length-1,n))],true)})}
var s=W.homesStunde&&document.querySelector('g[data-stunde="'+W.homesStunde+'"]');if(s)wahl(s)})()</script>`

// stundeStyle is the chosen bar and its line.
const stundeStyle = `<style>.verlauf g[data-zeile]{cursor:pointer;outline:none}.verlauf g[aria-pressed=true] .treffer{fill:var(--line)}
.verlauf g[data-zeile]:focus-visible .treffer{stroke:var(--accent);stroke-width:1.5}
.verlauf .stunde{margin:6px 4px 0;font-size:.92rem;color:var(--ink);font-variant-numeric:tabular-nums}.verlauf .stunde p{margin:0}
.verlauf .stunde .geraete{margin-top:2px;font-size:.85rem;color:var(--e-wort)}</style>`

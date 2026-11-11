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

// The chosen hour (owner, 2026-11-11: „Bei Energie sollte angezeigt
// werden, was in der Stunde, die ich ausgewählt habe, auch verbraucht
// wurde“): under the Tag chart one line says what the house used in the
// hour tapped — from the sun, the grid, the battery — and, when HA's energy
// dashboard counts devices, the three that used most. The latest hour HA
// has summed is chosen at first; the table has a Verbrauch column.

// statSlot is one statistic the history reads, and the key its amounts
// are kept under.
type statSlot struct{ key, id string }

// historySlots are the history's three counters.
func historySlots(st energie.Stats) []statSlot {
	return []statSlot{{"solar", st.Solar}, {"bezug", st.Bezug}, {"einsp", st.Einspeisung}}
}

// stundeSlots are what the chosen hour reads besides them: each battery's
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

// stundenTag is the Tag chart's hours: each bar's line, what the house
// used (hat: HA has a number for that hour), and the bar chosen at first.
type stundenTag struct {
	zeile     []string
	name      []string
	verbrauch []float64
	hat       []bool
	wahl      int
}

// stundenOf reads each hour of the Tag chart's bars from hourly; the bar
// chosen at first is the latest hour HA has summed, else the hour now.
func stundenOf(bs []bucket, hourly amounts, geraete []geraet, now time.Time) *stundenTag {
	t := &stundenTag{wahl: -1}
	jetzt := -1
	for i, x := range bs {
		at := time.Unix(x.at, 0).In(now.Location())
		s := energie.Stunde{Solar: x.v["solar"], Einsp: x.v["einsp"], Bezug: x.v["bezug"],
			Entladen: hourly["entladen"][x.at], Geladen: hourly["geladen"][x.at]}
		_, e := hourly["entladen"][x.at]
		_, g := hourly["geladen"][x.at]
		hat := len(x.v) > 0 || e || g
		for _, d := range geraete {
			s.Geraete = append(s.Geraete, energie.Geraet{Name: d.name, KWh: hourly["geraet:"+d.stat][x.at], Teil: d.teil})
		}
		t.zeile, t.name = append(t.zeile, stundeZeile(at, s, hat)), append(t.name, stundeName(at))
		t.verbrauch, t.hat = append(t.verbrauch, s.Verbrauch()), append(t.hat, hat)
		if hat && !at.After(now) {
			t.wahl = i
		}
		if !at.After(now) && now.Before(at.Add(time.Hour)) {
			jetzt = i
		}
	}
	if t.wahl < 0 {
		t.wahl = max(jetzt, 0)
	}
	return t
}

// stundeName is an hour as the family says it: „12–13 Uhr“.
func stundeName(at time.Time) string {
	end := at.Add(time.Hour).Hour()
	if end == 0 {
		end = 24
	}
	return strconv.Itoa(at.Hour()) + "–" + strconv.Itoa(end) + " Uhr"
}

// stundeZeile is the line under the chart for one hour (HTML): what the
// house used and where it came from, then the devices that used most.
func stundeZeile(at time.Time, s energie.Stunde, hat bool) string {
	head := "<p><b>" + stundeName(at) + ":</b> "
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

// stundeKWh is an hour's amount: to two places below 0,1 (0,05), else to
// one (0,9).
func stundeKWh(v float64) string {
	if math.Round(v*100) < 10 {
		return strings.Replace(strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64), ".", ",", 1)
	}
	return decimal(v)
}

// bar opens bar i's group: on the Tag chart a button that chooses the
// hour, the chosen one marked; elsewhere as it was.
func (t *stundenTag) bar(group string, i int, x bucket) string {
	if t == nil {
		return group
	}
	attrs := ` data-stunde=` + strconv.FormatInt(x.at, 10) + ` data-zeile="` + html.EscapeString(t.zeile[i]) +
		`" role=button aria-label="` + t.name[i] + `"`
	if i == t.wahl {
		return strings.Replace(group, "<g", "<g class=gewaehlt aria-pressed=true tabindex=0"+attrs, 1)
	}
	return strings.Replace(group, "<g", "<g aria-pressed=false tabindex=-1"+attrs, 1)
}

// role is the chart's: a picture, or on the Tag chart a group of bars to
// choose from.
func (t *stundenTag) role() string {
	if t == nil {
		return "img"
	}
	return "group"
}

// cell is bar i's Verbrauch in the table; "" off the Tag chart.
func (t *stundenTag) cell(i int) string {
	switch {
	case t == nil:
		return ""
	case t.hat[i]:
		return "<td>" + energyWords(t.verbrauch[i]) + "</td>"
	}
	return "<td>–</td>"
}

// html is the chosen hour's line and the script that changes it.
func (t *stundenTag) html() string {
	if t == nil || len(t.zeile) == 0 {
		return ""
	}
	return stundeStyle + "<div class=stunde aria-live=polite>" + t.zeile[t.wahl] + "</div>" + stundeScript
}

// stundeScript chooses an hour: a tap on its bar, or on the keyboard the
// arrows, Home and End (one tab stop, the chosen bar), Enter and Space.
// The choice outlives the page's own refresh (a new swap runs it again).
const stundeScript = `<script>(function(){var W=window;
function wahl(g,fokus){var f=g.closest('figure');f.querySelectorAll('g[data-zeile]').forEach(function(x){var an=x===g;x.classList.toggle('gewaehlt',an);x.setAttribute('aria-pressed',an);x.setAttribute('tabindex',an?0:-1)});
var z=f.querySelector('.stunde');if(z)z.innerHTML=g.getAttribute('data-zeile');W.homesStunde=g.getAttribute('data-stunde');if(fokus)g.focus()}
if(!W.homesStundeAn){W.homesStundeAn=1;
document.addEventListener('click',function(e){var g=e.target.closest&&e.target.closest('g[data-zeile]');if(g)wahl(g)});
document.addEventListener('keydown',function(e){var g=e.target.closest&&e.target.closest('g[data-zeile]');if(!g)return;
if(e.key==='Enter'||e.key===' '){e.preventDefault();wahl(g);return}
var gs=[].slice.call(g.parentNode.querySelectorAll('g[data-zeile]')),i=gs.indexOf(g),n={ArrowLeft:i-1,ArrowRight:i+1,Home:0,End:gs.length-1}[e.key];
if(n===undefined)return;e.preventDefault();wahl(gs[Math.max(0,Math.min(gs.length-1,n))],true)})}
var s=W.homesStunde&&document.querySelector('g[data-stunde="'+W.homesStunde+'"]');if(s)wahl(s)})()</script>`

// stundeStyle is the chosen bar and its line.
const stundeStyle = `<style>.verlauf g[data-zeile]{cursor:pointer;outline:none}.verlauf g.gewaehlt .treffer{fill:var(--line)}
.verlauf g[data-zeile]:focus-visible .treffer{stroke:var(--accent);stroke-width:1.5}
.verlauf .stunde{margin:6px 4px 0;font-size:.92rem;color:var(--ink);font-variant-numeric:tabular-nums}.verlauf .stunde p{margin:0}
.verlauf .stunde .geraete{margin-top:2px;font-size:.85rem;color:var(--e-wort)}</style>`

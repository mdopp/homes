package tagaktion

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/ha"
)

// What came of a touched tag (homes ADR 0004 §9, the owner 2026-11-27:
// „wenn eine Aktion ausgelöst wird, sehe ich das auch irgendwo?“): Homes
// stores nothing; it reads HA's own traces of the automations that trigger
// on the tag. After a report it waits a moment for their runs; the tags'
// list says each tag's last run. The family's line is Homes' words, never
// HA's text: a time condition says its hours, another its alias or a plain
// sentence.

// Lauf is what one automation made of a touch.
type Lauf struct {
	Automation, Name string
	Lief             bool
	Grund            string // why it did not: „nur zwischen 23 und 5 Uhr“, „noch nicht gemeldet“
	Wann             time.Time
	Person           string // who touched it, when the trigger says (the raw tag_scanned's person)
}

// Satz is the run in one line: „‚Gute Nacht‘ ausgeführt“.
func (l Lauf) Satz() string {
	switch {
	case l.Lief:
		return "‚" + l.Name + "‘ ausgeführt"
	case l.Grund == nichtGemeldet:
		return "‚" + l.Name + "‘ " + nichtGemeldet
	}
	return "‚" + l.Name + "‘ nicht ausgeführt – " + l.Grund
}

// Zeile is the runs after the tag's name: „Büro: ‚Gute Nacht‘ ausgeführt“.
func Zeile(tagName string, ls []Lauf) string {
	var s []string
	for _, l := range ls {
		s = append(s, l.Satz())
	}
	if tagName == "" {
		return strings.Join(s, "; ")
	}
	return tagName + ": " + strings.Join(s, "; ")
}

const nichtGemeldet = "noch nicht gemeldet"

// laufWarte is how long a report waits for the runs (Dienst.LaufWarte
// unless set); laufTakt how often it looks meanwhile.
var (
	laufWarte = 4 * time.Second
	laufTakt  = 300 * time.Millisecond
)

// spurenCache keeps HA's runs: a list for spurenFrisch, a run in full for
// good (a stopped run does not change).
type spurenCache struct {
	mu     sync.Mutex
	listen map[string]gelesen
	spuren map[string]ha.Spur // automation id + run id
}

type gelesen struct {
	at    time.Time
	laufe []ha.Lauf
}

const spurenFrisch = 10 * time.Second

func (d *Dienst) laeufe(ctx context.Context, id string, frisch bool) ([]ha.Lauf, error) {
	d.spuren.mu.Lock()
	g, ok := d.spuren.listen[id]
	d.spuren.mu.Unlock()
	if ok && !frisch && time.Since(g.at) < spurenFrisch {
		return g.laufe, nil
	}
	ls, err := d.HA.Laeufe(ctx, id)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(ls, func(a, b ha.Lauf) int { return b.Start.Compare(a.Start) }) // newest first
	d.spuren.mu.Lock()
	defer d.spuren.mu.Unlock()
	if d.spuren.listen == nil {
		d.spuren.listen, d.spuren.spuren = map[string]gelesen{}, map[string]ha.Spur{}
	}
	d.spuren.listen[id] = gelesen{time.Now(), ls}
	return ls, nil
}

func (d *Dienst) spur(ctx context.Context, id string, l ha.Lauf) (ha.Spur, error) {
	key := id + "\x00" + l.RunID
	d.spuren.mu.Lock()
	s, ok := d.spuren.spuren[key]
	d.spuren.mu.Unlock()
	if ok {
		return s, nil
	}
	s, err := d.HA.Spur(ctx, id, l.RunID)
	if err != nil {
		return s, err
	}
	if s.Execution != "running" {
		d.spuren.mu.Lock()
		if len(d.spuren.spuren) > 500 { // a box's worth of runs; start over rather than grow
			clear(d.spuren.spuren)
		}
		d.spuren.spuren[key] = s
		d.spuren.mu.Unlock()
	}
	return s, nil
}

// vonTag is the tag that triggered the run and who touched it, if the
// trigger was a tag (a tag trigger's event, or an event trigger on tag_scanned).
func vonTag(s ha.Spur) (tag, person string) {
	ev, _ := s.Trigger["event"].(map[string]any)
	data, _ := ev["data"].(map[string]any)
	if t, ok := ev["event_type"].(string); ok && t != "tag_scanned" {
		return "", ""
	}
	tag, _ = data["tag_id"].(string)
	person, _ = data["person"].(string)
	return tag, person
}

// lauf is a run in the family's words.
func lauf(v Verbindung, s ha.Spur) Lauf {
	l := Lauf{Automation: v.ID, Name: v.Name, Wann: s.Start}
	_, l.Person = vonTag(s)
	switch s.Execution {
	case "finished", "running":
		l.Lief = true
	case "failed_conditions":
		l.Grund = bedingung(s)
	case "failed_single", "failed_max_runs":
		l.Grund = "sie lief gerade schon"
	case "cancelled", "aborted":
		l.Grund = "abgebrochen"
	default:
		l.Grund = "mit einem Fehler abgebrochen"
	}
	return l
}

// bedingung is the condition the run stopped at, in words.
func bedingung(s ha.Spur) string {
	const allgemein = "Bedingung nicht erfüllt"
	rest, ok := strings.CutPrefix(s.LastStep, "condition/")
	if !ok {
		return allgemein
	}
	i, err := strconv.Atoi(strings.SplitN(rest, "/", 2)[0])
	conds, _ := s.Config["conditions"].([]any)
	if c, ok := s.Config["condition"].([]any); ok {
		conds = c
	}
	if err != nil || i < 0 || i >= len(conds) {
		return allgemein
	}
	c, _ := conds[i].(map[string]any)
	if a, _ := c["alias"].(string); strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	if c["condition"] == "time" {
		if w := zeitWorte(c); w != "" {
			return w
		}
	}
	return allgemein
}

// zeitWorte is a time condition's hours: „nur zwischen 23 und 5 Uhr“; a
// time from an entity (input_datetime) or weekdays alone are no hours.
func zeitWorte(c map[string]any) string {
	after, before := uhr(c["after"]), uhr(c["before"])
	switch {
	case after != "" && before != "":
		return "nur zwischen " + after + " und " + before + " Uhr"
	case after != "":
		return "erst ab " + after + " Uhr"
	case before != "":
		return "nur bis " + before + " Uhr"
	}
	return ""
}

// uhr is „23:00:00“ as „23“, „22:30:00“ as „22:30“; "" for anything else.
func uhr(v any) string {
	s, _ := v.(string)
	var h, m, sec int
	if n, _ := fmt.Sscanf(s, "%d:%d:%d", &h, &m, &sec); n < 2 || h > 23 || m > 59 {
		return ""
	}
	if m == 0 {
		return strconv.Itoa(h)
	}
	return fmt.Sprintf("%d:%02d", h, m)
}

// letzter is the newest run of v since seit that tag triggered; false: none.
func (d *Dienst) letzter(ctx context.Context, v Verbindung, tag string, seit time.Time, frisch bool) (Lauf, bool) {
	ls, err := d.laeufe(ctx, v.ID, frisch)
	if err != nil {
		log.Printf("homes: tag %s: the runs of %s: %v", tag, v.ID, err)
		return Lauf{}, false
	}
	for i, l := range ls {
		if i >= 5 || l.Start.Before(seit) { // the newest few: an old touch is not worth reading every run
			break
		}
		s, err := d.spur(ctx, v.ID, l)
		if err != nil {
			log.Printf("homes: tag %s: run %s of %s: %v", tag, l.RunID, v.ID, err)
			continue
		}
		if t, _ := vonTag(s); t == tag {
			return lauf(v, s), true
		}
	}
	return Lauf{}, false
}

// Nachher waits up to laufWarte for the runs of the automations that
// trigger on tag, started since seit (a second's grace for the clocks);
// one HA has not shown by then is „noch nicht gemeldet“. None: nil.
func (d *Dienst) Nachher(ctx context.Context, tag string, seit time.Time) []Lauf {
	mit, err := d.Verbunden(ctx)
	if err != nil || len(mit[tag]) == 0 {
		return nil
	}
	seit = seit.Add(-time.Second)
	out := make([]Lauf, len(mit[tag]))
	da := make([]bool, len(out))
	warte := laufWarte
	if d.LaufWarte > 0 {
		warte = d.LaufWarte
	}
	bis := time.Now().Add(warte)
	for {
		alle := true
		for i, v := range mit[tag] {
			if !da[i] {
				out[i], da[i] = d.letzter(ctx, v, tag, seit, true)
				alle = alle && da[i]
			}
		}
		if alle || !time.Now().Before(bis) {
			break
		}
		select {
		case <-ctx.Done():
			bis = time.Now()
		case <-time.After(laufTakt):
		}
	}
	for i, v := range mit[tag] {
		if !da[i] {
			out[i] = Lauf{Automation: v.ID, Name: v.Name, Grund: nichtGemeldet}
		}
	}
	return out
}

// Zuletzt is, per tag, the newest run any of its automations made of it;
// a tag without one is absent. mit is Verbunden's answer.
func (d *Dienst) Zuletzt(ctx context.Context, mit map[string][]Verbindung) map[string]Lauf {
	out := map[string]Lauf{}
	for tag, vs := range mit {
		for _, v := range vs {
			if l, ok := d.letzter(ctx, v, tag, time.Time{}, false); ok && (out[tag].Wann.IsZero() || l.Wann.After(out[tag].Wann)) {
				out[tag] = l
			}
		}
	}
	return out
}

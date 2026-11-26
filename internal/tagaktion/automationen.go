package tagaktion

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/assistent"
	"github.com/mdopp/homes/internal/bote"
	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/jsonfile"
)

// A tag and HA's automations (ADR 0004 §8): the automations that trigger
// on a tag are read from their configs; linking adds one tag trigger to an
// automation through HA's config API and keeps the config it had, so
// „Lösen“ brings exactly that back; a new automation is a sentence passed
// to the Bote, whose Automat writes it with its code.

// Verbindung is an automation that triggers on a tag.
type Verbindung struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Automationen are HA's automations whose name holds q (case aside), by name.
func (d *Dienst) Automationen(ctx context.Context, q string) ([]ha.Automation, error) {
	all, err := d.HA.Automations(ctx)
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	out := []ha.Automation{}
	for _, a := range all {
		if strings.Contains(strings.ToLower(a.Name), q) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b ha.Automation) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// Verbunden is, per tag id, the automations that trigger on it: a tag
// trigger with its id, or an event trigger on tag_scanned with it.
func (d *Dienst) Verbunden(ctx context.Context) (map[string][]Verbindung, error) {
	all, err := d.Automationen(ctx, "")
	if err != nil {
		return nil, err
	}
	out := map[string][]Verbindung{}
	for _, a := range all {
		cfg, err := d.HA.AutomationConfig(ctx, a.ID)
		if err != nil {
			if errors.Is(err, ha.ErrNotFound) { // from YAML, or gone meanwhile
				continue
			}
			return nil, err
		}
		for _, tag := range tagsIn(cfg) {
			out[tag] = append(out[tag], Verbindung{a.ID, a.Name})
		}
	}
	return out, nil
}

// triggers is an automation's trigger list under its key: "triggers"
// (HA since 2024.10) or "trigger"; one trigger alone is a list of one.
func triggers(cfg map[string]any) (key string, list []any) {
	key = "triggers"
	if _, ok := cfg["triggers"]; !ok {
		if _, ok := cfg["trigger"]; ok {
			key = "trigger"
		}
	}
	switch v := cfg[key].(type) {
	case []any:
		return key, v
	case map[string]any:
		return key, []any{v}
	}
	return key, nil
}

// tagIDs are the tag ids one trigger fires on: a tag trigger's tag_id (one
// or a list), an event trigger's tag_scanned tag_id.
func tagIDs(t any) []string {
	m, _ := t.(map[string]any)
	kind, _ := m["trigger"].(string)
	if kind == "" {
		kind, _ = m["platform"].(string)
	}
	switch kind {
	case "tag":
		switch v := m["tag_id"].(type) {
		case string:
			return []string{v}
		case []any:
			var out []string
			for _, x := range v {
				if s, ok := x.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	case "event":
		data, _ := m["event_data"].(map[string]any)
		if id, ok := data["tag_id"].(string); ok && m["event_type"] == "tag_scanned" {
			return []string{id}
		}
	}
	return nil
}

func tagsIn(cfg map[string]any) []string {
	_, list := triggers(cfg)
	var out []string
	for _, t := range list {
		for _, id := range tagIDs(t) {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

func kopie(cfg map[string]any) map[string]any {
	b, _ := json.Marshal(cfg)
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}

// mitTag is cfg with one more trigger, the tag, in the config's own style.
func mitTag(cfg map[string]any, tag string) map[string]any {
	out := kopie(cfg)
	key, list := triggers(out)
	t := map[string]any{"trigger": "tag", "tag_id": tag}
	if key == "trigger" {
		t = map[string]any{"platform": "tag", "tag_id": tag}
	}
	out[key] = append(slices.Clone(list), t)
	return out
}

// ohneTag is cfg without the triggers on tag (another id in a tag
// trigger's list stays).
func ohneTag(cfg map[string]any, tag string) map[string]any {
	out := kopie(cfg)
	key, list := triggers(out)
	keep := []any{}
	for _, t := range list {
		ids := tagIDs(t)
		if !slices.Contains(ids, tag) {
			keep = append(keep, t)
			continue
		}
		if m, _ := t.(map[string]any); len(ids) > 1 {
			rest := []any{}
			for _, id := range ids {
				if id != tag {
					rest = append(rest, id)
				}
			}
			m["tag_id"] = rest
			keep = append(keep, m)
		}
	}
	out[key] = keep
	return out
}

func gleich(a, b map[string]any) bool { return reflect.DeepEqual(kopie(a), kopie(b)) }

// boteWarte is how long the Bote has for a new automation's draft: less
// than the page's script waits (a minute), so its answer is a sentence.
var boteWarte = 45 * time.Second

// vorher is what Homes changed: the config before and the one it wrote.
type vorher struct {
	Vorher  map[string]any `json:"vorher"`
	Nachher map[string]any `json:"nachher"`
}

func (d *Dienst) verbindungenPfad() string { return filepath.Join(d.Dir, "tag-verbindungen.json") }

// gemerkt reads the kept configs; vmu is held.
func (d *Dienst) gemerkt() map[string]vorher {
	m := map[string]vorher{}
	if b, err := os.ReadFile(d.verbindungenPfad()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func (d *Dienst) merke(key string, v *vorher) {
	m := d.gemerkt()
	if v == nil {
		delete(m, key)
	} else {
		m[key] = *v
	}
	if d.Dir == "" {
		return
	}
	if err := jsonfile.Save(d.verbindungenPfad(), m); err != nil { // Lösen then takes the trigger out only
		log.Printf("homes: tag-verbindungen: %v", err)
	}
}

// Wunsch is a change to the tag's automations someone asked for.
type Wunsch struct {
	Art        string `json:"art"` // verbinden, loesen, neu
	Tag        string `json:"tag"`
	Automation string `json:"automation,omitempty"` // verbinden, loesen
	Satz       string `json:"satz,omitempty"`       // neu: what should happen
}

// The kinds of Wunsch.
const (
	Verbinden      = "verbinden"
	Loesen         = "loesen"
	NeueAutomation = "neu"
)

// plan is a Wunsch with its names, checked against HA as it is now.
type plan struct {
	Wunsch
	TagName, AutoName string
}

func (p plan) tag() string  { return "‚" + p.TagName + "‘" }
func (p plan) auto() string { return "‚" + p.AutoName + "‘" }

// vorschau is the change in one sentence, before anything is written.
func (p plan) vorschau() string {
	switch p.Art {
	case Verbinden:
		return "Wenn jemand " + p.tag() + " berührt, startet " + p.auto() + ". Was die Automation sonst auslöst, bleibt."
	case Loesen:
		return p.auto() + " startet dann nicht mehr, wenn jemand " + p.tag() + " berührt. Sonst bleibt sie, wie sie ist."
	}
	return assistent.Name() + " bekommt: „Wenn jemand " + p.tag() + " berührt: " + p.Satz + "“ und schlägt die Automation vor; angelegt wird sie erst mit seinem Code."
}

// wunschSatz is the change as the admins read it, with who asked.
func (p plan) wunschSatz(wer string) string {
	switch p.Art {
	case Verbinden:
		return wer + " möchte " + p.tag() + " mit " + p.auto() + " verbinden."
	case Loesen:
		return wer + " möchte " + p.tag() + " von " + p.auto() + " lösen."
	}
	return wer + " wünscht sich für " + p.tag() + " eine neue Automation: „" + p.Satz + "“"
}

// planen checks w against HA: the automation is there, linked or not as
// the change needs, a new one has its sentence and the Bote to take it.
func (d *Dienst) planen(ctx context.Context, w Wunsch) (plan, error) {
	p := plan{Wunsch: w, TagName: d.Name(ctx, w.Tag)}
	if p.TagName == "" {
		p.TagName = "dieser Tag"
	}
	if w.Art == NeueAutomation {
		p.Satz = strings.Join(strings.Fields(w.Satz), " ")
		if p.Satz == "" || len([]rune(p.Satz)) > 500 {
			return p, nein(http.StatusBadRequest, "Sag in einem Satz, was passieren soll, zum Beispiel „Licht im Flur an“.")
		}
		if d.Bote == nil {
			return p, nein(http.StatusServiceUnavailable, "Neue Automationen nimmt auf dieser Box gerade niemand an: "+assistent.Name()+" ist nicht eingerichtet.")
		}
		return p, nil
	}
	if w.Art != Verbinden && w.Art != Loesen {
		return p, nein(http.StatusBadRequest, "Das kennt Homes nicht. Bitte aktualisiere die App.")
	}
	all, err := d.HA.Automations(ctx)
	if err != nil {
		return p, HANein("automationen", err)
	}
	i := slices.IndexFunc(all, func(a ha.Automation) bool { return a.ID == w.Automation })
	if i < 0 {
		return p, nein(http.StatusNotFound, "Diese Automation gibt es in Home Assistant nicht (mehr).")
	}
	p.AutoName = all[i].Name
	cfg, err := d.HA.AutomationConfig(ctx, w.Automation)
	if errors.Is(err, ha.ErrNotFound) {
		return p, nein(http.StatusConflict, p.auto()+" steht nicht in Home Assistants Editor und lässt sich von Homes nicht ändern.")
	}
	if err != nil {
		return p, HANein("automation", err)
	}
	switch hat := slices.Contains(tagsIn(cfg), w.Tag); {
	case w.Art == Verbinden && hat:
		return p, nein(http.StatusConflict, p.tag()+" startet "+p.auto()+" schon.")
	case w.Art == Loesen && !hat:
		return p, nein(http.StatusConflict, p.auto()+" startet nicht, wenn jemand "+p.tag()+" berührt.")
	}
	return p, nil
}

// ausfuehren writes the change, as admin (an admin's confirmed change, or
// their Freigeben); it says what came of it, and for a new automation the
// Bote's reply (its draft and the code it asks for).
func (d *Dienst) ausfuehren(ctx context.Context, p plan, admin bote.Person) (satz, antwort string, err error) {
	if p.Art == NeueAutomation {
		text := "Bitte lege in Home Assistant eine Automation an. Auslöser: der NFC-Tag " + p.tag() + " (trigger: tag, tag_id: " + p.Tag + "). Was passieren soll: " + p.Satz
		c, cancel := context.WithTimeout(ctx, boteWarte)
		defer cancel()
		antwort, err := d.Bote.Stream(c, admin, text, nil)
		if err != nil {
			log.Printf("homes: tag %s: the Bote: %v", p.Tag, err)
			if c.Err() != nil && ctx.Err() == nil {
				return "", "", nein(http.StatusGatewayTimeout, assistent.Name()+" hat nicht rechtzeitig geantwortet. Versuch es gleich noch einmal.")
			}
			return "", "", nein(http.StatusBadGateway, assistent.Name()+" antwortet gerade nicht. Versuch es gleich noch einmal.")
		}
		return assistent.Name() + " schlägt die Automation vor. Antworte im Sprechen-Feld mit dem Code, dann legt er sie an.", antwort, nil
	}
	d.vmu.Lock()
	defer d.vmu.Unlock()
	cfg, err := d.HA.AutomationConfig(ctx, p.Automation)
	if err != nil {
		return "", "", HANein("automation", err)
	}
	key := p.Tag + "|" + p.Automation
	if p.Art == Verbinden {
		if slices.Contains(tagsIn(cfg), p.Tag) {
			return "Verbunden: " + p.tag() + " startet " + p.auto() + " schon.", "", nil
		}
		neu := mitTag(cfg, p.Tag)
		if err := d.HA.SetAutomationConfig(ctx, p.Automation, neu); err != nil {
			return "", "", HANein("automation schreiben", err)
		}
		d.merke(key, &vorher{cfg, neu})
		log.Printf("homes: tag %s linked to automation %s by %s", p.Tag, p.Automation, admin.User)
		return "Verbunden: " + p.tag() + " startet jetzt " + p.auto() + ".", "", nil
	}
	if !slices.Contains(tagsIn(cfg), p.Tag) {
		d.merke(key, nil)
		return "Gelöst: " + p.auto() + " startet schon nicht mehr auf " + p.tag() + ".", "", nil
	}
	v, ok := d.gemerkt()[key]
	neu, satz := ohneTag(cfg, p.Tag), "Gelöst: Homes hat "+p.tag()+" aus "+p.auto()+" genommen, der Rest bleibt, wie er ist."
	switch {
	case ok && gleich(cfg, v.Nachher):
		neu, satz = v.Vorher, "Gelöst: "+p.auto()+" ist wieder genau wie vorher."
	case ok:
		satz = "Gelöst: " + p.auto() + " wurde inzwischen geändert, darum hat Homes nur " + p.tag() + " herausgenommen; der Rest bleibt, wie er jetzt ist."
	}
	if err := d.HA.SetAutomationConfig(ctx, p.Automation, neu); err != nil {
		return "", "", HANein("automation schreiben", err)
	}
	d.merke(key, nil)
	log.Printf("homes: tag %s unlinked from automation %s by %s (as before: %v)", p.Tag, p.Automation, admin.User, ok && gleich(cfg, v.Nachher))
	return satz, "", nil
}

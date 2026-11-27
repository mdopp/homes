package tagaktion

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/ha"
	"github.com/mdopp/homes/internal/ha/hafake"
)

// ADR 0004 §9: what came of a touch, read from HA's traces; the family's
// line in Homes' words.

func mitBedingungen(r *rig) {
	r.ha.AddAutomation("1700000000003", "Nachtlicht", map[string]any{"id": "1700000000003", "alias": "Nachtlicht",
		"triggers":   []any{map[string]any{"trigger": "tag", "tag_id": buero}},
		"conditions": []any{map[string]any{"condition": "time", "after": "23:00:00", "before": "05:00:00"}, map[string]any{"condition": "state", "entity_id": "person.anna", "state": "home"}}})
	r.ha.AddAutomation("1700000000004", "Bambi", map[string]any{"id": "1700000000004", "alias": "Bambi",
		"triggers":   []any{map[string]any{"trigger": "event", "event_type": "tag_scanned", "event_data": map[string]any{"tag_id": buero}}},
		"conditions": []any{map[string]any{"condition": "state", "alias": "nur wenn jemand zu Hause ist", "entity_id": "zone.home", "state": "1"}}})
}

func TestWhatCameOfATouch(t *testing.T) {
	r := newRig(t)
	mitBedingungen(r)
	r.ha.RunsOnTag("1700000000002", "finished", "action/0")
	r.ha.RunsOnTag("1700000000003", "failed_conditions", "condition/0")
	r.ha.RunsOnTag("1700000000004", "failed_conditions", "condition/0")
	e, err := r.d.Ausloesen(context.Background(), buero, "anna")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Lauf{
		"1700000000002": {Name: "Flurlicht", Lief: true},
		"1700000000003": {Name: "Nachtlicht", Grund: "nur zwischen 23 und 5 Uhr"},
		"1700000000004": {Name: "Bambi", Grund: "nur wenn jemand zu Hause ist"},
	}
	if len(e.Laeufe) != 3 {
		t.Fatalf("%+v", e.Laeufe)
	}
	for _, l := range e.Laeufe {
		w := want[l.Automation]
		if l.Name != w.Name || l.Lief != w.Lief || l.Grund != w.Grund || l.Person != "anna" || l.Wann.IsZero() {
			t.Errorf("%+v", l)
		}
	}
	if !strings.HasPrefix(e.Text, "Bürotür: ") {
		t.Fatalf("%q", e.Text)
	}
	for _, s := range []string{"‚Flurlicht‘ ausgeführt", "‚Nachtlicht‘ nicht ausgeführt – nur zwischen 23 und 5 Uhr", "‚Bambi‘ nicht ausgeführt – nur wenn jemand zu Hause ist"} {
		if !strings.Contains(e.Text, s) {
			t.Errorf("%q lacks %q", e.Text, s)
		}
	}
}

// No run within the wait: „noch nicht gemeldet“, honestly; a run of
// before the touch or of another tag is not this touch's.
func TestNoRunYetIsSaid(t *testing.T) {
	r := newRig(t)
	cfg := r.ha.AutomationConfig("1700000000002")
	r.ha.AddTrace("1700000000002", hafake.Trace("alt", time.Now().Add(-time.Hour), "finished", "action/0", cfg,
		map[string]any{"platform": "tag", "event": map[string]any{"event_type": "tag_scanned", "data": map[string]any{"tag_id": buero}}}))
	r.ha.AddTrace("1700000000002", hafake.Trace("anderer", time.Now().Add(time.Second), "finished", "action/0", cfg,
		map[string]any{"platform": "tag", "event": map[string]any{"event_type": "tag_scanned", "data": map[string]any{"tag_id": "homes-anderer000"}}}))
	began := time.Now()
	e, _ := r.d.Ausloesen(context.Background(), buero, "anna")
	if took := time.Since(began); took < 300*time.Millisecond || took > 2*time.Second {
		t.Fatalf("waited %v", took)
	}
	if len(e.Laeufe) != 1 || e.Laeufe[0].Lief || e.Laeufe[0].Grund != "noch nicht gemeldet" || e.Text != "Bürotür: ‚Flurlicht‘ noch nicht gemeldet" {
		t.Fatalf("%+v %q", e.Laeufe, e.Text)
	}
	// a tag without automations waits for nothing and says nothing
	began = time.Now()
	e, _ = r.d.Ausloesen(context.Background(), "homes-leer00000000", "anna")
	if e.Laeufe != nil || e.Text != "" || time.Since(began) > 200*time.Millisecond {
		t.Fatalf("%+v", e)
	}
}

func TestZuletztIsTheNewestRunByTheTag(t *testing.T) {
	r := newRig(t)
	mitBedingungen(r)
	trig := func(tag, person string) map[string]any {
		return map[string]any{"platform": "tag", "event": map[string]any{"event_type": "tag_scanned", "data": map[string]any{"tag_id": tag, "person": person}}}
	}
	now := time.Now()
	r.ha.AddTrace("1700000000002", hafake.Trace("a", now.Add(-3*time.Hour), "finished", "action/0", r.ha.AutomationConfig("1700000000002"), trig(buero, "ben")))
	r.ha.AddTrace("1700000000003", hafake.Trace("b", now.Add(-time.Hour), "failed_conditions", "condition/0", r.ha.AutomationConfig("1700000000003"), trig(buero, "")))
	r.ha.AddTrace("1700000000003", hafake.Trace("c", now.Add(-time.Minute), "finished", "action/0", r.ha.AutomationConfig("1700000000003"),
		map[string]any{"platform": "time", "now": "x"})) // not a tag's run
	mit, _ := r.d.Verbunden(context.Background())
	z := r.d.Zuletzt(context.Background(), mit)
	l, ok := z[buero]
	if !ok || l.Automation != "1700000000003" || l.Satz() != "‚Nachtlicht‘ nicht ausgeführt – nur zwischen 23 und 5 Uhr" || l.Person != "" {
		t.Fatalf("%+v", z)
	}
	if _, ok := z["homes-anderer000"]; ok {
		t.Fatal("a tag without runs")
	}
}

func TestConditionWords(t *testing.T) {
	cases := []struct {
		c    map[string]any
		want string
	}{
		{map[string]any{"condition": "time", "after": "22:30:00"}, "erst ab 22:30 Uhr"},
		{map[string]any{"condition": "time", "before": "06:00"}, "nur bis 6 Uhr"},
		{map[string]any{"condition": "time", "after": "input_datetime.wecker"}, "Bedingung nicht erfüllt"},
		{map[string]any{"condition": "time", "weekday": []any{"sat"}}, "Bedingung nicht erfüllt"},
		{map[string]any{"condition": "numeric_state"}, "Bedingung nicht erfüllt"},
		{map[string]any{"condition": "time", "after": "23:00:00", "alias": " Nachts "}, "Nachts"},
	}
	for _, c := range cases {
		s := lauf(Verbindung{"x", "X"}, spurMit(c.c))
		if s.Grund != c.want {
			t.Errorf("%v: %q", c.c, s.Grund)
		}
	}
	for exe, want := range map[string]string{"error": "mit einem Fehler abgebrochen", "failed_single": "sie lief gerade schon", "cancelled": "abgebrochen"} {
		s := spurMit(nil)
		s.Execution = exe
		if l := lauf(Verbindung{"x", "X"}, s); l.Lief || l.Grund != want {
			t.Errorf("%s: %+v", exe, l)
		}
	}
}

// spurMit is a run stopped at its first condition, c.
func spurMit(c map[string]any) ha.Spur {
	return ha.Spur{Lauf: ha.Lauf{Execution: "failed_conditions", LastStep: "condition/0"}, Config: map[string]any{"conditions": []any{c}}}
}

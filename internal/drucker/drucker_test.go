package drucker

import (
	"testing"

	"github.com/mdopp/homes/internal/ha"
)

func epson(state string, reason any, black string) Drucker {
	attrs := map[string]any{"friendly_name": "EPSON WF-3540 Series", "device_class": "enum"}
	if reason != nil {
		attrs["state_reason"] = reason
	}
	return Drucker{Status: ha.State{EntityID: "sensor.epson", State: state, Attributes: attrs},
		Inks: []ha.State{{EntityID: "sensor.epson_black_ink", State: black, Attributes: map[string]any{"friendly_name": "EPSON WF-3540 Series Black ink", "unit_of_measurement": "%"}}}}
}

// The owner (2026-11-14): „Wird auch der Status ‚druckt‘ dargestellt?“ The
// state in the family's words, idle as the printer's page says it.
func TestWord(t *testing.T) {
	for state, want := range map[string]string{"idle": "bereit", "printing": "druckt", "stopped": "angehalten",
		"processing": "erreichbar", "unavailable": "nicht erreichbar", "unknown": "nicht erreichbar", "": "nicht erreichbar"} {
		if got := epson(state, nil, "40").Word(); got != want {
			t.Errorf("%q: %q, want %q", state, got, want)
		}
	}
}

// IPP's printer-state-reasons as HA's ipp sensor passes them on
// (state_reason: one keyword, a list, or comma-separated): what a person
// can act on, short; a -report, an unknown keyword and none say nothing.
func TestReason(t *testing.T) {
	for _, c := range []struct {
		reason any
		want   string
	}{{"media-empty-error", "Papier leer"}, {"media-needed-warning", "Papier leer"}, {"media-jam-error", "Papierstau"},
		{"marker-supply-empty-warning", "Tinte leer"}, {"cover-open-error", "Klappe offen"}, {"door-open", "Klappe offen"},
		{[]any{"other-warning", "media-jam-error"}, "Papierstau"}, {"spool-area-full-report, media-empty-error", "Papier leer"},
		{"media-empty-report", ""}, {"marker-supply-low-warning", ""}, {"something-new-error", ""}, {nil, ""}} {
		if got := epson("stopped", c.reason, "40").Reason(); got != c.want {
			t.Errorf("%v: %q, want %q", c.reason, got, c.want)
		}
	}
}

// The line after the state: the reason, then the black ink, a warning
// when either asks for a hand; nothing while unreachable. Text is the one
// line the app shows.
func TestDetailAndText(t *testing.T) {
	for _, c := range []struct {
		p            Drucker
		detail, text string
		warn         bool
	}{
		{epson("printing", nil, "42"), "Schwarz 42 %", "druckt · Schwarz 42 %", false},
		{epson("idle", nil, "12"), "Schwarz fast leer · 12 %", "bereit · Schwarz fast leer · 12 %", true},
		{epson("stopped", "media-empty-error", "40"), "Papier leer · Schwarz 40 %", "angehalten · Papier leer · Schwarz 40 %", true},
		{epson("idle", nil, "unknown"), "", "bereit", false},
		{epson("unavailable", "media-empty-error", "40"), "", "nicht erreichbar", false},
	} {
		d, warn := c.p.Detail()
		if d != c.detail || warn != c.warn || c.p.Text() != c.text {
			t.Errorf("%s: %q %v %q, want %q %v %q", c.p.Status.State, d, warn, c.p.Text(), c.detail, c.warn, c.text)
		}
	}
}

// Of finds the printer from its state sensor or a marker, nothing else.
func TestOf(t *testing.T) {
	p := epson("idle", nil, "40")
	states := []ha.State{p.Status, p.Inks[0], {EntityID: "sensor.temp", State: "21"}}
	reg := ha.Registry{Platforms: map[string]string{"sensor.epson": "ipp", "sensor.epson_black_ink": "ipp"},
		Devices: map[string]string{"sensor.epson": "d", "sensor.epson_black_ink": "d"}}
	for _, id := range []string{"sensor.epson", "sensor.epson_black_ink"} {
		if got, ok := Of(states, reg, id); !ok || got.Status.EntityID != "sensor.epson" {
			t.Errorf("%s: %+v %v", id, got, ok)
		}
	}
	if _, ok := Of(states, reg, "sensor.temp"); ok {
		t.Error("a temperature is a printer")
	}
}

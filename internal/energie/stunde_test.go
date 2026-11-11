package energie

import (
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// The owner's hour (2026-11-11): 0,6 from the sun stayed, 0,3 from the grid,
// 0,05 into it — 0,9 used.
func TestStundeVerbrauchWithoutBattery(t *testing.T) {
	s := Stunde{Solar: 0.65, Einsp: 0.05, Bezug: 0.3}
	if !near(s.Verbrauch(), 0.9) || !near(s.Selbst(), 0.6) {
		t.Fatalf("Verbrauch %v, Selbst %v; want 0.9, 0.6", s.Verbrauch(), s.Selbst())
	}
	if top, rest := s.Top(3); top != nil || rest != 0 {
		t.Fatalf("no devices: top %v rest %v, want nothing", top, rest)
	}
}

// A battery: what came out of it is used, what went in is not.
func TestStundeVerbrauchWithBattery(t *testing.T) {
	s := Stunde{Solar: 1.0, Einsp: 0.1, Bezug: 0.2, Entladen: 0.3, Geladen: 0.4}
	if !near(s.Verbrauch(), 1.0) {
		t.Fatalf("Verbrauch %v, want 1.0 (1 − 0,1 + 0,2 + 0,3 − 0,4)", s.Verbrauch())
	}
	if !near(s.Selbst()+s.Bezug+s.Entladen, s.Verbrauch()) {
		t.Fatalf("Selbst %v + Bezug + Entladen does not add up to Verbrauch %v", s.Selbst(), s.Verbrauch())
	}
	// at night the battery alone: nothing from the sun, nothing below 0
	night := Stunde{Entladen: 0.5, Bezug: 0.1}
	if !near(night.Verbrauch(), 0.6) || night.Selbst() != 0 {
		t.Fatalf("night: Verbrauch %v Selbst %v, want 0.6 and 0", night.Verbrauch(), night.Selbst())
	}
	// charged from the grid at a cheap hour: never a negative use
	if v := (Stunde{Bezug: 0.2, Geladen: 0.5}).Verbrauch(); v != 0 {
		t.Fatalf("Verbrauch %v, want 0", v)
	}
}

// The three that used most, a part of another device never added, the
// rest of what the house used.
func TestStundeTopDevices(t *testing.T) {
	s := Stunde{Solar: 0.6, Bezug: 0.3, Geraete: []Geraet{
		{Name: "Trockner", KWh: 0.2}, {Name: "Wärmepumpe", KWh: 0.4}, {Name: "Steckdose Keller", KWh: 0.3, Teil: true},
		{Name: "Kühlschrank", KWh: 0.05}, {Name: "Router", KWh: 0.01}, {Name: "Aus", KWh: 0},
	}}
	top, rest := s.Top(3)
	if len(top) != 3 || top[0].Name != "Wärmepumpe" || top[1].Name != "Trockner" || top[2].Name != "Kühlschrank" {
		t.Fatalf("top %v, want Wärmepumpe, Trockner, Kühlschrank", top)
	}
	if !near(rest, 0.25) {
		t.Fatalf("rest %v, want 0.25", rest)
	}
	// devices that used nothing this hour: nothing to list
	if top, _ := (Stunde{Bezug: 0.2, Geraete: []Geraet{{Name: "Trockner"}}}).Top(3); top != nil {
		t.Fatalf("top %v, want none", top)
	}
}

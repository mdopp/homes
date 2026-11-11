package energie

import (
	"math"
	"sort"
)

// Stunde is what the counters added in one hour, in kWh: what the sun
// made, what went into the grid and came from it, what came out of the
// battery and went into it (0 without one), and what each device HA's
// energy dashboard counts used. The Energie page's chosen hour says what
// the house used from it (owner, 2026-11-11).
type Stunde struct {
	Solar, Einsp, Bezug, Entladen, Geladen float64
	Geraete                                []Geraet
}

// Geraet is one device's use in the hour. Teil says it is part of another
// device counted as well (a socket behind a sub-meter): it is never added
// to the others.
type Geraet struct {
	Name string
	KWh  float64
	Teil bool
}

// Verbrauch is what the house used: what the sun made and stayed, what
// came from the grid and out of the battery, less what went into it.
// Never below 0 (counters summed a moment apart).
func (s Stunde) Verbrauch() float64 {
	return math.Max(0, s.Solar-s.Einsp+s.Bezug+s.Entladen-s.Geladen)
}

// Selbst is what the house used straight from the sun: what it made, less
// what went into the grid and into the battery. With Bezug and Entladen it
// adds up to Verbrauch.
func (s Stunde) Selbst() float64 {
	return math.Max(0, s.Solar-s.Einsp-s.Geladen)
}

// Top is the n devices that used most in the hour (parts of another
// device left out, as are those that used nothing) and the rest of
// Verbrauch, never below 0. No device: nothing, and no rest.
func (s Stunde) Top(n int) (top []Geraet, rest float64) {
	for _, g := range s.Geraete {
		if !g.Teil && g.KWh >= 0.005 {
			top = append(top, g)
		}
	}
	if len(top) == 0 {
		return nil, 0
	}
	sort.SliceStable(top, func(i, j int) bool { return top[i].KWh > top[j].KWh })
	if len(top) > n {
		top = top[:n]
	}
	rest = s.Verbrauch()
	for _, g := range top {
		rest -= g.KWh
	}
	return top, math.Max(0, rest)
}

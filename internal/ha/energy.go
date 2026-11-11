package ha

import (
	"context"
	"errors"
)

// EnergyPrefs is what HA's energy dashboard is set up with
// (energy/get_prefs): the house battery's counters and the devices whose
// use it counts, each a statistic id. The Energie page reads an hour's
// consumption from them; HA without an energy dashboard has none.
type EnergyPrefs struct {
	Batteries []Battery
	Devices   []Device
}

// Battery is one battery's counters: what came out of it (stat_energy_from)
// and what went in (stat_energy_to).
type Battery struct{ Out, In string }

// Device is one device the dashboard counts: its statistic, the name the
// dashboard gives it ("" when it names none) and, when it is part of
// another device (a socket behind a sub-meter), that one's statistic.
type Device struct{ Stat, Name, PartOf string }

// EnergyPrefs reads the energy dashboard's setup. HA without one answers
// not_found: no batteries, no devices, no error.
func (c *Client) EnergyPrefs(ctx context.Context) (EnergyPrefs, error) {
	var raw struct {
		Sources []struct {
			Type string `json:"type"`
			From string `json:"stat_energy_from"`
			To   string `json:"stat_energy_to"`
		} `json:"energy_sources"`
		Devices []struct {
			Stat   string `json:"stat_consumption"`
			Name   string `json:"name"`
			PartOf string `json:"included_in_stat"`
		} `json:"device_consumption"`
	}
	err := c.once(ctx, map[string]any{"type": "energy/get_prefs"}, &raw)
	if errors.Is(err, errNotFound) {
		return EnergyPrefs{}, nil
	}
	if err != nil {
		return EnergyPrefs{}, err
	}
	var p EnergyPrefs
	for _, s := range raw.Sources {
		b := Battery{Out: statID(s.From), In: statID(s.To)}
		if s.Type == "battery" && (b.Out != "" || b.In != "") {
			p.Batteries = append(p.Batteries, b)
		}
	}
	for _, d := range raw.Devices {
		if EntityRE.MatchString(d.Stat) {
			p.Devices = append(p.Devices, Device{d.Stat, d.Name, statID(d.PartOf)})
		}
	}
	return p, nil
}

// statID is id when it is one Statistics reads, else "".
func statID(id string) string {
	if EntityRE.MatchString(id) {
		return id
	}
	return ""
}

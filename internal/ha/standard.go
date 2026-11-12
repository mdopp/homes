package ha

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// StandardLabel is HA's label that makes a media player its room's standard
// speaker (owner 2026-11-12): laib's home-assistant template plays music
// asked for by voice there. It lives in HA, seen and changed there too
// (laib ADR 0017); Homes sets it per room.
const StandardLabel = "Standard-Lautsprecher"

type label struct {
	LabelID string `json:"label_id"`
	Name    string `json:"name"`
}

// standardLabel is the id of the label named StandardLabel, "" without one.
func standardLabel(labels []label) string {
	i := slices.IndexFunc(labels, func(l label) bool { return strings.EqualFold(strings.TrimSpace(l.Name), StandardLabel) })
	if i < 0 {
		return ""
	}
	return labels[i].LabelID
}

// SetStandard makes entity the one of players with StandardLabel (made
// when HA has none); entity "" only takes it off. With areaID, entity is
// put into that area: a Music Assistant player has none, and the voice
// finds the standard by its area. Other labels stay. The cached registry
// is read again after, so the next page shows the change.
func (c *Client) SetStandard(ctx context.Context, entity, areaID string, players []string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	w, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer w.close()
	defer w.closeOn(ctx)()
	cmd := func(m map[string]any, out any) error {
		id, err := w.send(m)
		if err == nil {
			err = w.result(id, out)
		}
		return err
	}
	var labels []label
	if err := cmd(map[string]any{"type": "config/label_registry/list"}, &labels); err != nil {
		return err
	}
	made := label{LabelID: standardLabel(labels)}
	if made.LabelID == "" && entity != "" {
		if err := cmd(map[string]any{"type": "config/label_registry/create", "name": StandardLabel, "icon": "mdi:speaker"}, &made); err != nil {
			return err
		}
	}
	for _, p := range append(slices.Clone(players), entity) {
		var e struct {
			Labels []string `json:"labels"`
		}
		if p == "" || made.LabelID == "" {
			continue
		} else if err := cmd(map[string]any{"type": "config/entity_registry/get", "entity_id": p}, &e); err != nil {
			return err
		}
		upd := map[string]any{"type": "config/entity_registry/update", "entity_id": p,
			"labels": slices.DeleteFunc(slices.Clone(e.Labels), func(l string) bool { return l == made.LabelID })}
		if p == entity {
			upd["labels"] = append(upd["labels"].([]string), made.LabelID)
			if areaID != "" {
				upd["area_id"] = areaID
			}
		}
		if err := cmd(upd, nil); err != nil {
			return fmt.Errorf("ha: %s: %w", p, err)
		}
	}
	if reg, err := c.registries(ctx); err == nil {
		c.registry.put(reg)
	}
	return nil
}

// put keeps reg as the cached registry, the temperature unit as it was.
func (a *registryCache) put(reg Registry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	reg.TempUnit = a.reg.TempUnit
	a.reg, a.ok, a.next = reg, true, time.Now().Add(5*time.Minute)
}

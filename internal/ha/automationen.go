package ha

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// HA's automations as Homes links a tag to them (homes ADR 0004 §8): the
// list from the automation entities, one automation's config through HA's
// config API (an administrator's, as Homes' HA user is), written back
// whole; HA reloads the automation on the write.

// Automation is one of HA's automations: its config id (the entity's
// attribute id), its entity, its name and whether it is on.
type Automation struct {
	ID, Entity, Name string
	An               bool
}

// Automations lists HA's automations that have a config id (one from
// YAML without an id cannot be changed through the config API).
func (c *Client) Automations(ctx context.Context) ([]Automation, error) {
	states, err := c.States(ctx)
	if err != nil {
		return nil, err
	}
	out := []Automation{}
	for _, s := range states {
		if !strings.HasPrefix(s.EntityID, "automation.") {
			continue
		}
		id, _ := s.Attributes["id"].(string)
		if id == "" {
			continue
		}
		name, _ := s.Attributes["friendly_name"].(string)
		if strings.TrimSpace(name) == "" {
			name = s.EntityID
		}
		out = append(out, Automation{ID: id, Entity: s.EntityID, Name: name, An: s.State == "on"})
	}
	return out, nil
}

func automationPath(id string) string { return "/api/config/automation/config/" + url.PathEscape(id) }

// AutomationConfig reads one automation's config as HA keeps it;
// ErrNotFound when HA has none under id.
func (c *Client) AutomationConfig(ctx context.Context, id string) (map[string]any, error) {
	var out map[string]any
	err := c.do(ctx, "GET", automationPath(id), nil, nil, &out)
	var he *Error
	if errors.As(err, &he) && he.Status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	return out, err
}

// SetAutomationConfig writes one automation's whole config; HA checks and
// reloads it.
func (c *Client) SetAutomationConfig(ctx context.Context, id string, cfg map[string]any) error {
	return c.do(ctx, "POST", automationPath(id), nil, cfg, nil)
}

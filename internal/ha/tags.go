package ha

import (
	"cmp"
	"context"
	"net/url"
	"time"
)

// The phone as a trigger (ADR 0004): Homes fires the app's reports as HA
// events, and names NFC tags in HA's own tag registry.

// Tag is one entry of HA's tag registry.
type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Fire fires event on HA's bus with data (POST /api/events/<event>,
// administrators only).
func (c *Client) Fire(ctx context.Context, event string, data map[string]any) error {
	return c.do(ctx, "POST", "/api/events/"+url.PathEscape(event), nil, data, nil)
}

// Tags lists HA's tag registry (tag/list). An entry names its id "id";
// "tag_id" is read too, should HA say it that way.
func (c *Client) Tags(ctx context.Context) ([]Tag, error) {
	var raw []struct {
		Tag
		TagID string `json:"tag_id"`
	}
	if err := c.command(ctx, map[string]any{"type": "tag/list"}, &raw); err != nil {
		return nil, err
	}
	out := make([]Tag, 0, len(raw))
	for _, t := range raw {
		out = append(out, Tag{ID: cmp.Or(t.ID, t.TagID), Name: t.Name})
	}
	return out, nil
}

// CreateTag adds the tag id with its name to HA's tag registry (tag/create).
func (c *Client) CreateTag(ctx context.Context, id, name string) error {
	return c.command(ctx, map[string]any{"type": "tag/create", "tag_id": id, "name": name}, nil)
}

// command sends one WebSocket command and reads its result into out (nil:
// the result is dropped), within 20s unless ctx ends sooner.
func (c *Client) command(ctx context.Context, m map[string]any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	w, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer w.close()
	defer w.closeOn(ctx)()
	id, err := w.send(m)
	if err != nil {
		return err
	}
	var discard any
	if out == nil {
		out = &discard
	}
	return w.result(id, out)
}

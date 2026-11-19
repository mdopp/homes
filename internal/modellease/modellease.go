// Package modellease is Laib's GPU lease as Homes speaks it (laib ADR
// 0011, templates/llama "The lease"): GET /api/model-lease says which
// model answers, for whom and until when; POST takes or renews a window
// for a preset, DELETE gives it back. Taking and giving back want the
// model key (HOMES_LLM_KEY); it is sent as the bearer and nowhere else:
// no error, log line or answer of this package carries it. The presets
// come from the proxy's /v1/models, the household's first.
package modellease

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client is the lease API next to the model proxy.
type Client struct {
	Base string // the proxy's OpenAI base, http://…:11435/v1 (HOMES_LLM_URL)
	Key  string // the model key, sent as the bearer of POST and DELETE
	HTTP *http.Client
}

// FromEnv is the lease beside HOMES_LLM_URL with HOMES_LLM_KEY, or nil
// when either is unset: no model key, no switching.
func FromEnv() *Client {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("HOMES_LLM_URL")), "/")
	key := strings.TrimSpace(os.Getenv("HOMES_LLM_KEY"))
	if base == "" || key == "" {
		return nil
	}
	return &Client{Base: base, Key: key}
}

// API is the lease's address: the proxy's port, /api/model-lease (laib
// state.ModelLease derives it the same way).
func (c *Client) API() string { return strings.TrimSuffix(c.Base, "/v1") + "/api/model-lease" }

// Stand is GET's answer. Times are Unix seconds; nil is none.
type Stand struct {
	State       string   `json:"state"`  // none, preparing, ready
	Mode        string   `json:"mode"`   // haushalt with no window, else the window: shared, full, …
	Alias       string   `json:"alias"`  // the preset that answers
	Holder      string   `json:"holder"` // who holds the window; "" with none
	ExpiresAt   *int64   `json:"expires_at"`
	LastRenewed *int64   `json:"last_renewed_at"`
	RenewAfter  *int64   `json:"renew_after"`
	Allowed     []string `json:"allowed"`
}

// Held is a window someone else holds: POST's and DELETE's 409.
type Held struct {
	Holder    string
	ExpiresAt int64
}

func (h *Held) Error() string { return "the lease is held by " + h.Holder }

// Error is any other refusal: the status and the lease's reason word.
type Error struct {
	Status int
	Reason string
}

func (e *Error) Error() string { return fmt.Sprintf("lease answered %d %s", e.Status, e.Reason) }

// wait bounds one call: the card stays usable when the lease is away.
const wait = 8 * time.Second

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: wait}
}

// do sends one request (the key only with keyed) and decodes a 2xx answer
// into out. The transport's error is cut to its kind: a URL's query or a
// header never reaches a log line.
func (c *Client) do(ctx context.Context, method, url string, body any, keyed bool, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return errors.New("lease: bad address")
	}
	req.Header.Set("Content-Type", "application/json")
	if keyed {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return errors.New("lease: not reachable")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode/100 == 2 {
		if out != nil && json.Unmarshal(raw, out) != nil {
			return &Error{Status: resp.StatusCode, Reason: "unreadable"}
		}
		return nil
	}
	var no struct {
		Reason    string `json:"reason"`
		Holder    string `json:"holder"`
		ExpiresAt int64  `json:"expires_at"`
	}
	json.Unmarshal(raw, &no)
	if resp.StatusCode == http.StatusConflict && no.Holder != "" {
		return &Held{Holder: no.Holder, ExpiresAt: no.ExpiresAt}
	}
	return &Error{Status: resp.StatusCode, Reason: no.Reason}
}

// Read is the lease now.
func (c *Client) Read(ctx context.Context) (Stand, error) {
	var s Stand
	err := c.do(ctx, http.MethodGet, c.API(), nil, false, &s)
	return s, err
}

// Presets are the presets the proxy lists, the household's first.
func (c *Client) Presets(ctx context.Context) ([]string, error) {
	var l struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, c.Base+"/models", nil, true, &l); err != nil {
		return nil, err
	}
	var out []string
	for _, m := range l.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out, nil
}

// Take takes or renews holder's window for preset, for ttl from the
// first take (a renewal keeps the end). A *Held when someone else holds one.
func (c *Client) Take(ctx context.Context, holder, preset string, ttl time.Duration) error {
	return c.do(ctx, http.MethodPost, c.API(), map[string]any{"model": preset, "ttl_s": int(ttl.Seconds()), "holder": holder}, true, nil)
}

// Release gives holder's window back; never the bodyless call, which is
// the admin's way out of anyone's.
func (c *Client) Release(ctx context.Context, holder string) error {
	return c.do(ctx, http.MethodDelete, c.API(), map[string]string{"holder": holder}, true, nil)
}

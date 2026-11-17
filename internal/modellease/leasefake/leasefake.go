// Package leasefake is Laib's model proxy as far as the lease goes, for
// tests and cmd/homesdemo: GET/POST/DELETE /api/model-lease and GET
// /v1/models with laib's rules (lease/lease.go): the key on POST and
// DELETE, a holder of [a-z0-9-]{1,64}, ttl_s clamped to 300–86400, a
// renewal that keeps the end, someone else's window a 409, and a window
// whose holder missed two renewals (the grace, 10 min) or whose end
// passed given back. Its clock is Now.
package leasefake

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sync"
	"time"
)

// Request is one request the fake saw.
type Request struct {
	Method, Path, Auth string
	Body               []byte
}

// Window is the standing lease.
type Window struct {
	Holder, Mode, Alias       string
	Deadline, Until, LastSeen time.Time
	RenewAfter                int64
	Preparing                 bool
}

// Server is the fake.
type Server struct {
	*httptest.Server
	Key       string
	Household string
	Presets   []string          // the household's first
	Modes     map[string]string // preset → window; unlisted: full
	Now       func() time.Time
	// Down answers every request 503, as a lease that is not there
	Down bool
	// Preparing answers a take 202, its weights still loading
	Preparing bool

	mu   sync.Mutex
	seen []Request
	win  *Window
}

const grace = 10 * time.Minute

var holderRE = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// New is a proxy with the box's presets: Qwen the household's, the two
// Gemmas in the shared window, Qwen 3.8 in full.
func New(key string) *Server {
	s := &Server{Key: key, Household: "qwen3.6-35b-a3b",
		Presets: []string{"qwen3.6-35b-a3b", "gemma-4-e4b", "gemma-4-12b", "qwen3.8-27b"},
		Modes:   map[string]string{"gemma-4-e4b": "shared", "gemma-4-12b": "shared"}, Now: time.Now}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

// Base is the OpenAI base, as HOMES_LLM_URL has it.
func (s *Server) Base() string { return s.URL + "/v1" }

// Seen are the requests so far.
func (s *Server) Seen() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seen)
}

// Hold puts a window in place as if holder had taken it, until end; no
// holder, none.
func (s *Server) Hold(holder, preset string, end time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if holder == "" {
		s.win = nil
		return
	}
	now := s.Now()
	s.win = &Window{Holder: holder, Mode: s.mode(preset), Alias: preset, Deadline: end, Until: minTime(now.Add(grace), end), LastSeen: now, RenewAfter: 300}
}

// Lease is the standing window, nil with none.
func (s *Server) Lease() *Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	if s.win == nil {
		return nil
	}
	w := *s.win
	return &w
}

func (s *Server) mode(preset string) string {
	if m, ok := s.Modes[preset]; ok {
		return m
	}
	return "full"
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (s *Server) expire() {
	if s.win != nil && s.Now().After(s.win.Until) {
		s.win = nil
	}
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, Request{Method: r.Method, Path: r.URL.Path, Auth: r.Header.Get("Authorization"), Body: body})
	if s.Down {
		reply(w, http.StatusServiceUnavailable, map[string]any{"error": "down"})
		return
	}
	s.expire()
	keyed := r.Header.Get("Authorization") == "Bearer "+s.Key
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		data := []map[string]any{}
		for _, p := range s.Presets {
			data = append(data, map[string]any{"id": p, "object": "model"})
		}
		reply(w, http.StatusOK, map[string]any{"object": "list", "data": data})
	case r.URL.Path != "/api/model-lease":
		reply(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case r.Method == http.MethodGet:
		reply(w, http.StatusOK, s.status())
	case !keyed:
		reply(w, http.StatusUnauthorized, map[string]any{"ok": false, "reason": "unauthorized"})
	case r.Method == http.MethodPost:
		s.take(w, body)
	case r.Method == http.MethodDelete:
		var in struct {
			Holder string `json:"holder"`
		}
		if json.Unmarshal(body, &in) != nil || !holderRE.MatchString(in.Holder) {
			reply(w, http.StatusBadRequest, map[string]any{"ok": false, "reason": "invalid_holder"})
			return
		}
		if s.win != nil && s.win.Holder != in.Holder {
			reply(w, http.StatusConflict, map[string]any{"ok": false, "reason": "held", "holder": s.win.Holder, "expires_at": s.win.Deadline.Unix()})
			return
		}
		s.win = nil
		reply(w, http.StatusOK, map[string]any{"ok": true, "state": "released"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) take(w http.ResponseWriter, body []byte) {
	var in struct {
		Model  string `json:"model"`
		TTL    int64  `json:"ttl_s"`
		Holder string `json:"holder"`
	}
	switch {
	case json.Unmarshal(body, &in) != nil:
		reply(w, http.StatusBadRequest, map[string]any{"ok": false, "reason": "invalid_json"})
		return
	case !holderRE.MatchString(in.Holder):
		reply(w, http.StatusBadRequest, map[string]any{"ok": false, "reason": "invalid_holder"})
		return
	case !slices.Contains(s.Presets, in.Model) || in.Model == s.Household:
		reply(w, http.StatusBadRequest, map[string]any{"ok": false, "reason": "invalid_model"})
		return
	case s.win != nil && s.win.Holder != in.Holder:
		reply(w, http.StatusConflict, map[string]any{"ok": false, "reason": "held", "holder": s.win.Holder, "expires_at": s.win.Deadline.Unix(), "mode": s.win.Mode})
		return
	}
	ttl := time.Duration(min(max(in.TTL, 300), 86400)) * time.Second
	now := s.Now()
	renew := int64(min(ttl/3, grace/2).Seconds())
	if s.win != nil && s.win.Mode == s.mode(in.Model) { // a renewal: the end stands
		s.win.Until, s.win.LastSeen, s.win.RenewAfter, s.win.Alias = minTime(now.Add(grace), s.win.Deadline), now, renew, in.Model
	} else {
		s.win = &Window{Holder: in.Holder, Mode: s.mode(in.Model), Alias: in.Model, Deadline: now.Add(ttl), Until: minTime(now.Add(grace), now.Add(ttl)),
			LastSeen: now, RenewAfter: renew, Preparing: s.Preparing}
	}
	if s.win.Preparing {
		reply(w, http.StatusAccepted, map[string]any{"ok": true, "state": "preparing", "model": s.win.Mode, "alias": in.Model, "retry_after": 30, "expires_at": nil})
		return
	}
	reply(w, http.StatusOK, map[string]any{"ok": true, "state": "ready", "model": s.win.Mode, "alias": in.Model,
		"expires_at": s.win.Deadline.Unix(), "renew_after": s.win.RenewAfter, "allowed": []string{in.Model}})
}

func (s *Server) status() map[string]any {
	if s.win == nil {
		return map[string]any{"state": "none", "model": "", "alias": s.Household, "holder": "", "expires_at": nil, "last_renewed_at": nil,
			"renew_after": nil, "retry_after": 30, "mode": "haushalt", "allowed": []string{s.Household}}
	}
	out := map[string]any{"state": "ready", "model": s.win.Mode, "alias": s.win.Alias, "holder": s.win.Holder, "expires_at": s.win.Deadline.Unix(),
		"last_renewed_at": s.win.LastSeen.Unix(), "renew_after": s.win.RenewAfter, "retry_after": 30, "mode": s.win.Mode, "allowed": []string{s.win.Alias}}
	if s.win.Preparing {
		out["state"], out["expires_at"] = "preparing", nil
	}
	return out
}

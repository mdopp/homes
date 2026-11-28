// Package hafake is a fake Home Assistant for tests: the REST calls Homes
// makes (/api/states, /api/services, /api/history/period,
// /api/camera_proxy, /api/stt, /api/config/automation/config,
// /api/mobile_app/registrations, /api/webhook, /api/config/config_entries) and the WebSocket
// (/api/websocket: auth, subscribe_events, the three registries, the Assist pipelines' debug store,
// the recorder's long-term statistics). Its frame
// code is written for the server side on purpose, so it shares nothing with
// the client it tests. Never a live box.
package hafake

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Call is one service call HA received.
type Call struct {
	Domain, Service string
	Data            map[string]any
}

// HA is the fake.
type HA struct {
	*httptest.Server
	Token string

	mu       sync.Mutex
	states   map[string]map[string]any
	order    []string
	calls    []Call
	history  map[string][]map[string]any
	histDown bool
	queries  []url.Values
	starts   []string
	down     bool
	callFail int
	areas    []map[string]any
	devices  []map[string]any
	entities []map[string]any
	labels   []map[string]any // the label registry: label_id, name
	regsDown bool
	tempUnit string // /api/config's unit_system.temperature; "" makes /api/config fail
	subs     []*conn
	cameras  map[string]camera
	assist   []Pipeline
	notAdmin bool
	stt      map[string]string // provider → the words it hears
	heard    []Speech
	stats    map[string][]Stat // statistic id → what it added, per hour (or coarser)
	apply    bool              // a call turns a light or switch on or off, like HA (the demo)
	statDown bool
	statAsks []map[string]any
	energy   map[string]any            // energy/get_prefs' answer; nil: HA's not_found (no energy dashboard)
	diag     map[string]any            // config entry id → its diagnostics' data (GET /api/diagnostics/config_entry/<id>)
	events   []Event                   // what POST /api/events fired
	tags     []map[string]any          // the tag registry: id, name
	autos    map[string]map[string]any // the automations' configs by id (/api/config/automation/config/<id>)
	mobile   mobileApp
	traces   map[string][]map[string]any // automation id → its runs' full traces (trace/list, trace/get), oldest first
	plaene   map[string][2]string        // automation id → how its next run on a tag ends: script_execution, last_step
}

// AddTrace puts one run's full trace (trace/get's shape: run_id,
// timestamp, script_execution, last_step, trace, config) under automation id.
func (h *HA) AddTrace(id string, trace map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.traces == nil {
		h.traces = map[string][]map[string]any{}
	}
	h.traces[id] = append(h.traces[id], trace)
}

// RunsOnTag makes automation id run, as HA would, whenever a tag it
// triggers on is scanned (a tag_scanned event or a webhook's scan_tag): a
// trace ending as execution at lastStep (finished at the last action,
// failed_conditions at condition/0, …). Without it, a scan leaves no trace.
func (h *HA) RunsOnTag(id, execution, lastStep string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.plaene == nil {
		h.plaene = map[string][2]string{}
	}
	h.plaene[id] = [2]string{execution, lastStep}
}

// laufen is a scan's runs: each planned automation whose config has a tag
// trigger on its tag_id. h.mu is held.
func (h *HA) laufen(data map[string]any) {
	for id, plan := range h.plaene {
		cfg := h.autos[id]
		trig, _ := cfg["triggers"].([]any)
		if t, ok := cfg["trigger"].([]any); ok {
			trig = t
		}
		for _, t := range trig {
			m, _ := t.(map[string]any)
			ed, _ := m["event_data"].(map[string]any)
			platform := "tag"
			if m["event_type"] == "tag_scanned" && ed["tag_id"] == data["tag_id"] {
				platform = "event"
			} else if m["tag_id"] != data["tag_id"] {
				continue
			}
			if h.traces == nil {
				h.traces = map[string][]map[string]any{}
			}
			h.traces[id] = append(h.traces[id], Trace(fmt.Sprintf("run-%s-%d", id, len(h.traces[id])+1), time.Now(), plan[0], plan[1], cfg,
				map[string]any{"platform": platform, "description": "Tag scanned", "event": map[string]any{"event_type": "tag_scanned", "data": data}}))
			break
		}
	}
}

// Trace is one run's full trace as trace/get answers it: trigger the
// trigger's variables; a failed_conditions run's last step answers false.
func Trace(runID string, start time.Time, execution, lastStep string, config, trigger map[string]any) map[string]any {
	steps := map[string]any{"trigger/0": []any{map[string]any{"path": "trigger/0", "timestamp": start.UTC().Format(time.RFC3339Nano),
		"changed_variables": map[string]any{"trigger": trigger}}}}
	if lastStep != "" {
		steps[lastStep] = []any{map[string]any{"path": lastStep, "result": map[string]any{"result": execution != "failed_conditions"}}}
	}
	return map[string]any{"run_id": runID, "domain": "automation", "item_id": config["id"], "state": "stopped", "script_execution": execution,
		"last_step": lastStep, "trigger": "Tag scanned", "timestamp": map[string]any{"start": start.UTC().Format(time.RFC3339Nano),
			"finish": start.UTC().Format(time.RFC3339Nano)}, "config": config, "trace": steps}
}

// traceCmd answers trace/list (the runs' summaries) and trace/get (one
// run in full). h.mu is held.
func (h *HA) traceCmd(m map[string]any) (any, bool) {
	runs := h.traces[fmt.Sprint(m["item_id"])]
	if m["type"] == "trace/list" {
		out := []any{}
		for _, t := range runs {
			sum := maps.Clone(t)
			delete(sum, "trace")
			delete(sum, "config")
			out = append(out, sum)
		}
		return out, true
	}
	for _, t := range runs {
		if t["run_id"] == m["run_id"] {
			return t, true
		}
	}
	return nil, false
}

// mobileApp is the mobile_app integration (homes ADR 0004 §7): its
// registrations, their webhooks, what came through them.
type mobileApp struct {
	off     bool                 // the integration not loaded: registrations answer 404
	regs    []map[string]any     // what was registered, in order
	hooks   map[string][2]string // webhook id → config entry id, device_id
	deleted map[string]bool      // webhook ids of removed registrations: 410, as HA says
	calls   []Webhook
	next    int
	// kaputt: registrations are taken (201, a webhook) but their entry's
	// setup fails, as HA did without os_version: no device, a webhook
	// that answers an empty 200 and does nothing
	kaputt bool
	broken map[string]string // entry id → title of entries whose setup failed
}

// MobileAppSetupFails makes registrations be taken but not set up.
func (h *HA) MobileAppSetupFails(fails bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mobile.kaputt = fails
}

// BrokenEntries is the titles of mobile_app entries whose setup failed.
func (h *HA) BrokenEntries() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, t := range h.mobile.broken {
		out = append(out, t)
	}
	return out
}

// Webhook is one command a registration's webhook received.
type Webhook struct {
	ID, Type, DeviceID string
	Data               any
}

// MobileAppOff makes the integration absent (404), as on an HA without it.
func (h *HA) MobileAppOff(off bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mobile.off = off
}

// Registrations are the registrations HA received, their bodies.
func (h *HA) Registrations() []map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.mobile.regs)
}

// Webhooks are the commands the registrations' webhooks received.
func (h *HA) Webhooks() []Webhook {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.mobile.calls)
}

// register is POST /api/mobile_app/registrations: a device in the device
// registry (identifiers mobile_app/<device_id>, its config entry), a
// webhook; a device_id registered again replaces its entry. h.mu is held.
func (h *HA) register(w http.ResponseWriter, r *http.Request) {
	if h.mobile.off {
		http.Error(w, "404: Not Found", http.StatusNotFound)
		return
	}
	var reg map[string]any
	json.NewDecoder(r.Body).Decode(&reg)
	for _, k := range []string{"app_id", "app_name", "app_version", "device_name", "manufacturer", "model", "os_name", "os_version"} {
		if s, _ := reg[k].(string); s == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"message": "Message format incorrect: required key not provided @ data['" + k + "']"})
			return
		}
	}
	dev, _ := reg["device_id"].(string)
	for id, hk := range h.mobile.hooks {
		if hk[1] == dev {
			h.removeEntry(hk[0])
			delete(h.mobile.hooks, id)
		}
	}
	h.mobile.next++
	entry, hook := fmt.Sprintf("entry-%d", h.mobile.next), fmt.Sprintf("webhook-%d", h.mobile.next)
	if h.mobile.hooks == nil {
		h.mobile.hooks, h.mobile.deleted = map[string][2]string{}, map[string]bool{}
	}
	h.mobile.regs = append(h.mobile.regs, reg)
	if h.mobile.kaputt {
		if h.mobile.broken == nil {
			h.mobile.broken = map[string]string{}
		}
		h.mobile.broken[entry], _ = reg["device_name"].(string)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"webhook_id": hook})
		return
	}
	h.mobile.hooks[hook] = [2]string{entry, dev}
	h.devices = append(slices.Clone(h.devices), map[string]any{"id": "dev-" + entry, "name": reg["device_name"],
		"identifiers": []any{[]any{"mobile_app", dev}}, "config_entries": []any{entry}})
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"webhook_id": hook, "secret": nil, "cloudhook_url": nil, "remote_ui_url": nil})
}

// removeEntry drops a config entry's devices. h.mu is held.
func (h *HA) removeEntry(entry string) bool {
	n := len(h.devices)
	h.devices = slices.DeleteFunc(slices.Clone(h.devices), func(d map[string]any) bool {
		es, _ := d["config_entries"].([]any)
		return slices.Contains(es, any(entry))
	})
	return len(h.devices) < n
}

// webhook is POST /api/webhook/<id>, no bearer needed: a removed
// registration's 410, an unknown id HA's empty 200, else the command.
// h.mu is held.
func (h *HA) webhook(w http.ResponseWriter, r *http.Request, id string) {
	if h.mobile.deleted[id] {
		w.WriteHeader(http.StatusGone)
		return
	}
	hk, ok := h.mobile.hooks[id]
	if !ok {
		return
	}
	var cmd struct {
		Type string `json:"type"`
		Data any    `json:"data"`
	}
	json.NewDecoder(r.Body).Decode(&cmd)
	h.mobile.calls = append(h.mobile.calls, Webhook{ID: id, Type: cmd.Type, DeviceID: hk[1], Data: cmd.Data})
	switch cmd.Type {
	case "scan_tag": // HA's own scan: tag_scanned with the device
		data, _ := cmd.Data.(map[string]any)
		ev := map[string]any{"tag_id": data["tag_id"], "device_id": "dev-" + hk[0]}
		h.events = append(h.events, Event{Type: "tag_scanned", Data: ev})
		h.laufen(ev)
		w.Write([]byte("{}"))
	case "register_sensor":
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	default:
		w.Write([]byte("{}"))
	}
}

// AddAutomation is an automation HA has: its entity (automation.<slug of
// id>, on, with the attributes id and friendly_name) and its config,
// which /api/config/automation/config/<id> reads and writes. Again with
// the same id replaces the config, as an edit in HA does.
func (h *HA) AddAutomation(id, name string, config map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.autos == nil {
		h.autos = map[string]map[string]any{}
	}
	h.autos[id] = config
	h.set("automation."+strings.ReplaceAll(strings.ToLower(id), "-", "_"), "on", map[string]any{"id": id, "friendly_name": name})
}

// AutomationConfig is one automation's config as it is now (nil: none).
func (h *HA) AutomationConfig(id string) map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, _ := json.Marshal(h.autos[id])
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}

// Event is one event fired on the fake's bus (POST /api/events/<type>).
type Event struct {
	Type string
	Data map[string]any
}

// Events is what was fired, oldest first.
func (h *HA) Events() []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Event(nil), h.events...)
}

// AddTag puts a tag into the tag registry.
func (h *HA) AddTag(id, name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tags = append(h.tags, map[string]any{"id": id, "name": name})
}

// SetDiagnostics sets what a config entry's diagnostics answer under
// "data", the integration's own shape (IPP's: {entry: {data: {host, …}}});
// nil answers 500, as IPP's do while the printer is unreachable.
func (h *HA) SetDiagnostics(entryID string, data map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.diag == nil {
		h.diag = map[string]any{}
	}
	h.diag[entryID] = data
}

// SetEnergyPrefs sets what energy/get_prefs answers, HA's shape
// ({energy_sources: […], device_consumption: […]}); nil answers not_found,
// as HA does without an energy dashboard.
func (h *HA) SetEnergyPrefs(prefs map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.energy = prefs
}

// Stat is what a statistic added in the hour (or the day) from Start.
type Stat struct {
	Start  time.Time
	Change float64
}

// SetStatistics sets a statistic's long-term statistics (haus#65): what it
// added from each Start; HA sums them into hours, days, weeks and months
// in its time zone (Europe/Berlin here).
func (h *HA) SetStatistics(id string, points []Stat) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stats == nil {
		h.stats = map[string][]Stat{}
	}
	h.stats[id] = append([]Stat(nil), points...)
}

// StatisticsDown makes recorder/statistics_during_period fail.
func (h *HA) StatisticsDown(down bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statDown = down
}

// StatisticsAsked are the statistics_during_period commands received.
func (h *HA) StatisticsAsked() []map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]map[string]any(nil), h.statAsks...)
}

// haZone is the fake HA's time zone (/api/config), where it sums days.
var haZone = func() *time.Location {
	l, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC
	}
	return l
}()

// statistics answers recorder/statistics_during_period as HA does
// (recorder/websocket_api.py, 2026-10): {id: [{start, end, change}]},
// start and end in ms since the epoch, a period without rows left out,
// an unknown id absent.
func (h *HA) statistics(m map[string]any) (any, bool) {
	h.statAsks = append(h.statAsks, m)
	if h.statDown {
		return nil, false
	}
	from, err1 := time.Parse(time.RFC3339, fmt.Sprint(m["start_time"]))
	to, err2 := time.Parse(time.RFC3339, fmt.Sprint(m["end_time"]))
	if err1 != nil || err2 != nil {
		return nil, false
	}
	period := fmt.Sprint(m["period"])
	bucket := func(t time.Time) time.Time {
		t = t.In(haZone)
		y, mo, d := t.Date()
		switch period {
		case "hour":
			return t.Truncate(time.Hour)
		case "day":
			return time.Date(y, mo, d, 0, 0, 0, 0, haZone)
		case "week":
			return time.Date(y, mo, d-(int(t.Weekday())+6)%7, 0, 0, 0, 0, haZone)
		default:
			return time.Date(y, mo, 1, 0, 0, 0, 0, haZone)
		}
	}
	next := func(t time.Time) time.Time {
		switch period {
		case "hour":
			return t.Add(time.Hour)
		case "day":
			return t.AddDate(0, 0, 1)
		case "week":
			return t.AddDate(0, 0, 7)
		}
		return t.AddDate(0, 1, 0)
	}
	out := map[string]any{}
	ids, _ := m["statistic_ids"].([]any)
	for _, idv := range ids {
		id := fmt.Sprint(idv)
		pts, ok := h.stats[id]
		if !ok {
			continue
		}
		sums := map[time.Time]float64{}
		var order []time.Time
		for _, p := range pts {
			b := bucket(p.Start)
			if b.Before(bucket(from)) || !b.Before(to) {
				continue
			}
			if _, seen := sums[b]; !seen {
				order = append(order, b)
			}
			sums[b] += p.Change
		}
		sort.Slice(order, func(i, j int) bool { return order[i].Before(order[j]) })
		rows := []any{}
		for _, b := range order {
			rows = append(rows, map[string]any{"start": float64(b.UnixMilli()), "end": float64(next(b).UnixMilli()), "change": sums[b]})
		}
		if len(rows) > 0 {
			out[id] = rows
		}
	}
	return out, true
}

// Speech is one request to HA's speech-to-text.
type Speech struct {
	Provider, Content string // X-Speech-Content
	Body              []byte
}

// SetSTT makes provider a speech-to-text that hears words in any speech
// (haus#63); "" hears nothing (HA's result "error").
func (h *HA) SetSTT(provider, words string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stt == nil {
		h.stt = map[string]string{}
	}
	h.stt[provider] = words
}

// Heard is what the speech-to-text was sent so far.
func (h *HA) Heard() []Speech {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Speech(nil), h.heard...)
}

// Pipeline is one Assist pipeline and the runs HA keeps of it (its debug
// store: the last ten, in memory).
type Pipeline struct {
	ID   string
	Runs []AssistRun
}

// AssistRun is one Assist run as HA's pipeline debug store holds it.
type AssistRun struct {
	ID       string
	Time     time.Time
	Sentence string // intent-start's intent_input
	Local    bool   // intent-end's processed_locally
	Code     string // "" answered; else the error response's data.code (no_intent_match, …)
	Open     bool   // still running: no intent-end yet
}

// SetAssist sets the Assist pipelines and their stored runs.
func (h *HA) SetAssist(ps []Pipeline) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.assist = ps
}

// AssistAdmin(false) makes the token's user no administrator: HA refuses
// the pipeline commands (websocket_api.require_admin, "unauthorized").
func (h *HA) AssistAdmin(admin bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notAdmin = !admin
}

// assistCmd answers the assist_pipeline commands as HA does
// (assist_pipeline/websocket_api.py, pipeline.py, 2026-10).
func (h *HA) assistCmd(m map[string]any) (result any, code string, handled bool) {
	switch m["type"] {
	case "assist_pipeline/pipeline/list", "assist_pipeline/pipeline_debug/list", "assist_pipeline/pipeline_debug/get":
	default:
		return nil, "", false
	}
	if h.notAdmin && m["type"] != "assist_pipeline/pipeline/list" {
		return nil, "unauthorized", true
	}
	var p *Pipeline
	for i := range h.assist {
		if h.assist[i].ID == m["pipeline_id"] {
			p = &h.assist[i]
		}
	}
	switch m["type"] {
	case "assist_pipeline/pipeline/list":
		ps := []any{}
		for _, p := range h.assist {
			ps = append(ps, map[string]any{"id": p.ID, "name": p.ID, "language": "de", "conversation_engine": "conversation.home_assistant"})
		}
		pref := ""
		if len(h.assist) > 0 {
			pref = h.assist[0].ID
		}
		return map[string]any{"pipelines": ps, "preferred_pipeline": pref}, "", true
	case "assist_pipeline/pipeline_debug/list":
		runs := []any{}
		if p != nil {
			for _, r := range p.Runs {
				runs = append(runs, map[string]any{"pipeline_run_id": r.ID, "timestamp": r.Time.UTC().Format("2006-01-02T15:04:05.000000+00:00")})
			}
		}
		return map[string]any{"pipeline_runs": runs}, "", true
	}
	if p == nil {
		return nil, "not_found", true
	}
	for _, r := range p.Runs {
		if r.ID != m["pipeline_run_id"] {
			continue
		}
		ts := r.Time.UTC().Format("2006-01-02T15:04:05.000000+00:00")
		ev := func(t string, data any) map[string]any {
			return map[string]any{"type": t, "data": data, "timestamp": ts}
		}
		events := []any{
			ev("run-start", map[string]any{"pipeline": p.ID, "language": "de", "conversation_id": "01JCONV", "runner_data": map[string]any{"stt_binary_handler_id": 1}}),
			ev("stt-end", map[string]any{"stt_output": map[string]any{"text": r.Sentence}}),
			ev("intent-start", map[string]any{"engine": "conversation.home_assistant", "language": "de", "intent_input": r.Sentence,
				"conversation_id": "01JCONV", "device_id": "dev-kueche-satellit", "satellite_id": "assist_satellite.kueche", "prefer_local_intents": true}),
		}
		if !r.Open {
			resp := map[string]any{"speech": map[string]any{"plain": map[string]any{"speech": "Erledigt"}}, "response_type": "action_done", "data": map[string]any{"targets": []any{}}, "language": "de"}
			if r.Code != "" {
				resp = map[string]any{"speech": map[string]any{"plain": map[string]any{"speech": "Das habe ich nicht verstanden"}}, "response_type": "error", "data": map[string]any{"code": r.Code}, "language": "de"}
			}
			events = append(events,
				ev("intent-end", map[string]any{"processed_locally": r.Local, "intent_output": map[string]any{"response": resp, "conversation_id": "01JCONV", "continue_conversation": false}}),
				ev("run-end", nil))
		}
		return map[string]any{"events": events}, "", true
	}
	return nil, "not_found", true
}

type camera struct {
	ctype string
	body  []byte
}

// SetCamera makes GET /api/camera_proxy/<entityID> answer body as ctype.
func (h *HA) SetCamera(entityID, ctype string, body []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cameras == nil {
		h.cameras = map[string]camera{}
	}
	h.cameras[entityID] = camera{ctype, body}
}

// New starts a fake HA that accepts Bearer token.
func New(token string) *HA {
	h := &HA{Token: token, states: map[string]map[string]any{}, history: map[string][]map[string]any{}, tempUnit: "°C"}
	h.Server = httptest.NewServer(http.HandlerFunc(h.serve))
	return h
}

// Set stores an entity's state (no event).
func (h *HA) Set(entityID, state string, attrs map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.set(entityID, state, attrs)
}

func (h *HA) set(entityID, state string, attrs map[string]any) map[string]any {
	if attrs == nil {
		attrs = map[string]any{}
	}
	if _, ok := h.states[entityID]; !ok {
		h.order = append(h.order, entityID)
	}
	s := map[string]any{"entity_id": entityID, "state": state, "attributes": attrs,
		"last_updated": "2026-10-14T10:00:00.123456+00:00", "last_changed": "2026-10-14T10:00:00.123456+00:00"}
	h.states[entityID] = s
	return s
}

// Change stores a state and sends state_changed to every subscriber.
func (h *HA) Change(entityID, state string, attrs map[string]any) {
	h.mu.Lock()
	s := h.set(entityID, state, attrs)
	subs := append([]*conn(nil), h.subs...)
	h.mu.Unlock()
	for _, c := range subs {
		c.send(map[string]any{"id": c.subID, "type": "event", "event": map[string]any{
			"event_type": "state_changed", "data": map[string]any{"entity_id": entityID, "new_state": s}}})
	}
}

// Subscribed is how many WebSocket clients hold a state_changed subscription.
func (h *HA) Subscribed() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// WaitSubscribed blocks until n clients subscribe, or 5s pass.
func (h *HA) WaitSubscribed(n int) bool {
	for i := 0; i < 500; i++ {
		if h.Subscribed() >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Calls are the service calls received so far.
func (h *HA) Calls() []Call {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Call(nil), h.calls...)
}

// SetHistory sets what /api/history/period answers for an entity.
func (h *HA) SetHistory(entityID string, points []map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.history[entityID] = points
}

// HistoryDown makes /api/history/period answer 500 (the recorder in
// trouble) while the states still answer.
func (h *HA) HistoryDown(down bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.histDown = down
}

// HistoryQueries are the history requests: the start (path) and the query.
func (h *HA) HistoryQueries() ([]string, []url.Values) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.starts...), append([]url.Values(nil), h.queries...)
}

// SetRegistries sets the area, device and entity registries.
func (h *HA) SetRegistries(areas, devices, entities []map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.areas, h.devices, h.entities = areas, devices, entities
}

// SetLabels sets the label registry ({label_id, name} each).
func (h *HA) SetLabels(labels []map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.labels = labels
}

// Entity is one entry of the entity registry as it is now (nil: none).
func (h *HA) Entity(id string) map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	i := slices.IndexFunc(h.entities, func(e map[string]any) bool { return e["entity_id"] == id })
	if i < 0 {
		return nil
	}
	return maps.Clone(h.entities[i])
}

// registryWrite answers a label's creation (its id the name lower case,
// "-" as "_", as HA makes it) and an entity's get and update (labels,
// area_id); ok false for an entity it does not have. h.mu is held.
func (h *HA) registryWrite(m map[string]any) (any, bool) {
	if name := fmt.Sprint(m["name"]); m["type"] == "config/label_registry/create" {
		h.labels = append(h.labels, map[string]any{"label_id": strings.ReplaceAll(strings.ToLower(name), "-", "_"), "name": name})
		return h.labels[len(h.labels)-1], true
	}
	i := slices.IndexFunc(h.entities, func(e map[string]any) bool { return e["entity_id"] == m["entity_id"] })
	if i < 0 {
		return nil, false
	}
	e := maps.Clone(h.entities[i])
	if m["type"] == "config/entity_registry/update" {
		for _, k := range []string{"labels", "area_id"} {
			if v, ok := m[k]; ok {
				e[k] = v
			}
		}
		h.entities = slices.Clone(h.entities) // the test's own slice stays as it set it
		h.entities[i] = e
	}
	return e, true
}

// SetTemperatureUnit is the house's temperature unit in /api/config
// (default °C); "" makes /api/config answer 500 while the rest works.
func (h *HA) SetTemperatureUnit(u string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tempUnit = u
}

// RegistriesDown makes the three registry commands on the WebSocket fail
// (success:false), so the areas cannot be read while the states can.
func (h *HA) RegistriesDown(down bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.regsDown = down
}

// ApplyCalls makes a service call change the state it names, as HA does:
// on, off, toggle, play, pause. For the demo (cmd/homesdemo); the tests
// keep the states as they set them.
func (h *HA) ApplyCalls(on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.apply = on
}

// FailCalls makes service calls answer status (0: succeed again).
func (h *HA) FailCalls(status int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callFail = status
}

// DropAll closes every WebSocket connection, as an HA restart does.
func (h *HA) DropAll() {
	h.mu.Lock()
	subs := append([]*conn(nil), h.subs...)
	h.mu.Unlock()
	for _, c := range subs {
		c.nc.Close()
	}
}

// WaitSubscribedExactly blocks until exactly n clients subscribe, or 5s pass.
func (h *HA) WaitSubscribedExactly(n int) bool {
	for i := 0; i < 500; i++ {
		if h.Subscribed() == n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// Down makes every REST call answer 500.
func (h *HA) Down(down bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.down = down
}

func (h *HA) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/websocket" {
		h.websocket(w, r)
		return
	}
	if id, ok := strings.CutPrefix(r.URL.Path, "/api/webhook/"); ok && r.Method == "POST" {
		h.mu.Lock()
		defer h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		h.webhook(w, r, id)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+h.Token {
		http.Error(w, "401: Unauthorized", http.StatusUnauthorized)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.down {
		http.Error(w, "500: boom", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/":
		w.Write([]byte(`{"message":"API running."}`))
	case r.Method == "GET" && r.URL.Path == "/api/config":
		if h.tempUnit == "" {
			http.Error(w, "500: config broken", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"location_name": "Zuhause", "time_zone": "Europe/Berlin",
			"unit_system": map[string]any{"length": "km", "mass": "g", "temperature": h.tempUnit, "volume": "L"}})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/diagnostics/config_entry/"):
		d, ok := h.diag[strings.TrimPrefix(r.URL.Path, "/api/diagnostics/config_entry/")]
		switch {
		case !ok:
			http.NotFound(w, r)
			return
		case d == nil: // IPP's diagnostics while the printer is unreachable (no coordinator data)
			http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"home_assistant": map[string]any{"version": "2026.11.0"}, "data": d})
	case r.Method == "GET" && r.URL.Path == "/api/states":
		out := []map[string]any{}
		for _, id := range h.order {
			out = append(out, h.states[id])
		}
		json.NewEncoder(w).Encode(out)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/states/"):
		s, ok := h.states[strings.TrimPrefix(r.URL.Path, "/api/states/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Entity not found."}`))
			return
		}
		json.NewEncoder(w).Encode(s)
	case r.Method == "POST" && r.URL.Path == "/api/mobile_app/registrations":
		h.register(w, r)
	case r.Method == "GET" && r.URL.Path == "/api/config/config_entries/entry":
		out := []map[string]any{}
		for _, hk := range h.mobile.hooks {
			out = append(out, map[string]any{"entry_id": hk[0], "domain": "mobile_app", "title": hk[1], "state": "loaded"})
		}
		for e, t := range h.mobile.broken {
			out = append(out, map[string]any{"entry_id": e, "domain": "mobile_app", "title": t, "state": "setup_error"})
		}
		json.NewEncoder(w).Encode(out)
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api/config/config_entries/entry/"):
		entry := strings.TrimPrefix(r.URL.Path, "/api/config/config_entries/entry/")
		if _, ok := h.mobile.broken[entry]; ok {
			delete(h.mobile.broken, entry)
			w.Write([]byte(`{"require_restart":false}`))
			return
		}
		if !h.removeEntry(entry) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Invalid entry specified"}`))
			return
		}
		for id, hk := range h.mobile.hooks {
			if hk[0] == entry {
				delete(h.mobile.hooks, id)
				h.mobile.deleted[id] = true
			}
		}
		w.Write([]byte(`{"require_restart":false}`))
	case strings.HasPrefix(r.URL.Path, "/api/config/automation/config/"):
		h.automation(w, r, strings.TrimPrefix(r.URL.Path, "/api/config/automation/config/"))
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/events/"):
		if h.notAdmin {
			http.Error(w, "401: Unauthorized", http.StatusUnauthorized)
			return
		}
		var data map[string]any
		json.NewDecoder(r.Body).Decode(&data)
		typ := strings.TrimPrefix(r.URL.Path, "/api/events/")
		h.events = append(h.events, Event{Type: typ, Data: data})
		if typ == "tag_scanned" {
			h.laufen(data)
		}
		json.NewEncoder(w).Encode(map[string]any{"message": "Event " + typ + " fired."})
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/services/"):
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/services/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		if h.callFail != 0 {
			http.Error(w, "service failed: some detail", h.callFail)
			return
		}
		var data map[string]any
		json.NewDecoder(r.Body).Decode(&data)
		h.calls = append(h.calls, Call{Domain: parts[0], Service: parts[1], Data: data})
		if id, ok := data["entity_id"].(string); ok && h.apply {
			if s, ok := h.states[id]; ok {
				attrs, _ := s["attributes"].(map[string]any)
				switch parts[1] {
				case "turn_on", "media_play":
					if pct, ok := data["brightness_pct"].(float64); ok { // a lamp turned on at a level, as HA keeps it
						attrs = maps.Clone(attrs)
						attrs["brightness"] = math.Round(pct * 255 / 100)
					}
					h.set(id, map[bool]string{true: "playing", false: "on"}[parts[1] == "media_play"], attrs)
				case "turn_off":
					h.set(id, "off", attrs)
				case "media_pause":
					h.set(id, "paused", attrs)
				case "toggle":
					h.set(id, map[bool]string{true: "off", false: "on"}[s["state"] == "on"], attrs)
				}
				ns, subs := h.states[id], append([]*conn(nil), h.subs...)
				go func() { // like HA: the change goes to every subscriber
					for _, c := range subs {
						c.send(map[string]any{"id": c.subID, "type": "event", "event": map[string]any{
							"event_type": "state_changed", "data": map[string]any{"entity_id": id, "new_state": ns}}})
					}
				}()
			}
		}
		// like HA: the states the call changed, each with the call's context
		changed := []any{}
		if id, ok := data["entity_id"].(string); ok {
			changed = append(changed, map[string]any{"entity_id": id, "context": map[string]any{"id": fmt.Sprintf("ctx-%d", len(h.calls))}})
		}
		json.NewEncoder(w).Encode(changed)
	case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/stt/"):
		provider := strings.TrimPrefix(r.URL.Path, "/api/stt/")
		words, ok := h.stt[provider]
		if !ok {
			http.Error(w, "404: Not Found", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		h.heard = append(h.heard, Speech{Provider: provider, Content: r.Header.Get("X-Speech-Content"), Body: body})
		if !strings.Contains(r.Header.Get("X-Speech-Content"), "format=wav") {
			http.Error(w, "415: Unsupported Media Type", http.StatusUnsupportedMediaType)
			return
		}
		if words == "" {
			w.Write([]byte(`{"text":null,"result":"error"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"text": words, "result": "success"})
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/camera_proxy/"):
		c, ok := h.cameras[strings.TrimPrefix(r.URL.Path, "/api/camera_proxy/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Entity not found."}`))
			return
		}
		w.Header().Set("Content-Type", c.ctype)
		w.Write(c.body)
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/history/period/"):
		h.starts = append(h.starts, strings.TrimPrefix(r.URL.Path, "/api/history/period/"))
		h.queries = append(h.queries, r.URL.Query())
		if h.histDown {
			http.Error(w, "500: recorder boom", http.StatusInternalServerError)
			return
		}
		pts, ok := h.history[r.URL.Query().Get("filter_entity_id")]
		if !ok {
			w.Write([]byte(`[]`))
			return
		}
		json.NewEncoder(w).Encode([][]map[string]any{pts})
	default:
		http.NotFound(w, r)
	}
}

// automation answers HA's config API for one automation, an
// administrator's: GET its config, POST a new one (HA's {"result":"ok"}).
// h.mu is held.
func (h *HA) automation(w http.ResponseWriter, r *http.Request, id string) {
	if h.notAdmin {
		http.Error(w, "401: Unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case "GET":
		c, ok := h.autos[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Resource not found"}`))
			return
		}
		json.NewEncoder(w).Encode(c)
	case "POST":
		var c map[string]any
		if json.NewDecoder(r.Body).Decode(&c) != nil {
			http.Error(w, `{"message":"Message format incorrect"}`, http.StatusBadRequest)
			return
		}
		if h.autos == nil {
			h.autos = map[string]map[string]any{}
		}
		h.autos[id] = c
		w.Write([]byte(`{"result":"ok"}`))
	default:
		http.Error(w, "405", http.StatusMethodNotAllowed)
	}
}

// --- the WebSocket, server side (RFC 6455) ---

type conn struct {
	nc    net.Conn
	br    *bufio.Reader
	wmu   sync.Mutex
	subID any
}

func (c *conn) send(v any) {
	b, _ := json.Marshal(v)
	c.wmu.Lock()
	defer c.wmu.Unlock()
	hdr := []byte{0x81}
	switch n := len(b); {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 1<<16:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	c.nc.Write(append(hdr, b...))
}

// read returns one client text message (clients must mask).
func (c *conn) read() ([]byte, error) {
	var msg []byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(c.br, h[:]); err != nil {
			return nil, err
		}
		op, masked, n := h[0]&0x0f, h[1]&0x80 != 0, uint64(h[1]&0x7f)
		switch n {
		case 126:
			var e [2]byte
			if _, err := io.ReadFull(c.br, e[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(e[:]))
		case 127:
			var e [8]byte
			if _, err := io.ReadFull(c.br, e[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(e[:])
		}
		if !masked {
			return nil, io.ErrUnexpectedEOF // a client frame must be masked
		}
		var key [4]byte
		if _, err := io.ReadFull(c.br, key[:]); err != nil {
			return nil, err
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(c.br, p); err != nil {
			return nil, err
		}
		for i := range p {
			p[i] ^= key[i%4]
		}
		switch op {
		case 0x8:
			return nil, io.EOF
		case 0x9:
			c.wmu.Lock()
			c.nc.Write(append([]byte{0x8a, byte(len(p))}, p...))
			c.wmu.Unlock()
			continue
		case 0xa:
			continue
		}
		msg = append(msg, p...)
		if h[0]&0x80 != 0 {
			return msg, nil
		}
	}
}

func (h *HA) websocket(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || key == "" {
		http.Error(w, "not a websocket", http.StatusBadRequest)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "no hijack", 500)
		return
	}
	nc, brw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer nc.Close()
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
	brw.Flush()
	c := &conn{nc: nc, br: brw.Reader}
	defer func() {
		h.mu.Lock()
		for i, s := range h.subs {
			if s == c {
				h.subs = append(h.subs[:i], h.subs[i+1:]...)
				break
			}
		}
		h.mu.Unlock()
	}()
	c.send(map[string]any{"type": "auth_required", "ha_version": "2026.10.0"})
	authed := false
	for {
		b, err := c.read()
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return
		}
		if !authed {
			if m["type"] != "auth" || m["access_token"] != h.Token {
				c.send(map[string]any{"type": "auth_invalid", "message": "Invalid access token"})
				return
			}
			authed = true
			c.send(map[string]any{"type": "auth_ok", "ha_version": "2026.10.0"})
			continue
		}
		h.mu.Lock()
		if h.regsDown && strings.HasSuffix(fmt.Sprint(m["type"]), "_registry/list") {
			h.mu.Unlock()
			c.send(map[string]any{"id": m["id"], "type": "result", "success": false, "error": map[string]any{"code": "unknown_error", "message": "registry broken"}})
			continue
		}
		if res, code, ok := h.assistCmd(m); ok {
			h.mu.Unlock()
			if code != "" {
				c.send(map[string]any{"id": m["id"], "type": "result", "success": false, "error": map[string]any{"code": code, "message": map[string]string{"unauthorized": "Unauthorized", "not_found": "not found"}[code]}})
			} else {
				c.send(map[string]any{"id": m["id"], "type": "result", "success": true, "result": res})
			}
			continue
		}
		if m["type"] == "energy/get_prefs" {
			prefs := h.energy
			h.mu.Unlock()
			if prefs == nil {
				c.send(map[string]any{"id": m["id"], "type": "result", "success": false, "error": map[string]any{"code": "not_found", "message": "No prefs"}})
			} else {
				c.send(map[string]any{"id": m["id"], "type": "result", "success": true, "result": prefs})
			}
			continue
		}
		if m["type"] == "recorder/statistics_during_period" {
			res, ok := h.statistics(m)
			h.mu.Unlock()
			if !ok {
				c.send(map[string]any{"id": m["id"], "type": "result", "success": false, "error": map[string]any{"code": "invalid_format", "message": "recorder boom"}})
			} else {
				c.send(map[string]any{"id": m["id"], "type": "result", "success": true, "result": res})
			}
			continue
		}
		var result any
		switch m["type"] {
		case "subscribe_events":
			if m["event_type"] == "state_changed" {
				c.subID = m["id"]
				h.subs = append(h.subs, c)
			}
		case "config/area_registry/list":
			result = h.areas
		case "config/device_registry/list":
			result = h.devices
		case "config/entity_registry/list":
			result = h.entities
		case "config/label_registry/list":
			result = h.labels
		case "tag/list":
			result = h.tags
		case "tag/create":
			t := map[string]any{"id": m["tag_id"], "name": m["name"]}
			h.tags = append(h.tags, t)
			result = t
		case "tag/update", "tag/delete":
			i := slices.IndexFunc(h.tags, func(t map[string]any) bool { return t["id"] == m["tag_id"] })
			if i < 0 {
				h.mu.Unlock()
				c.send(map[string]any{"id": m["id"], "type": "result", "success": false, "error": map[string]any{"code": "not_found", "message": "Tag not found"}})
				continue
			}
			if m["type"] == "tag/delete" {
				h.tags = slices.Delete(h.tags, i, i+1)
			} else {
				h.tags[i]["name"] = m["name"]
			}
			result = map[string]any{}
		case "trace/list", "trace/get":
			res, ok := h.traceCmd(m)
			if !ok {
				h.mu.Unlock()
				c.send(map[string]any{"id": m["id"], "type": "result", "success": false, "error": map[string]any{"code": "not_found", "message": "The trace could not be found"}})
				continue
			}
			result = res
		case "config/label_registry/create", "config/entity_registry/get", "config/entity_registry/update":
			res, ok := h.registryWrite(m)
			if !ok {
				h.mu.Unlock()
				c.send(map[string]any{"id": m["id"], "type": "result", "success": false, "error": map[string]any{"code": "not_found", "message": "Entity not found"}})
				continue
			}
			result = res
		}
		h.mu.Unlock()
		if result == nil {
			result = []any{}
		}
		if m["type"] == "subscribe_events" {
			result = nil
		}
		c.send(map[string]any{"id": m["id"], "type": "result", "success": true, "result": result})
	}
}

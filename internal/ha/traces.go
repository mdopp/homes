package ha

import (
	"context"
	"encoding/json"
	"time"
)

// What came of an automation's runs (homes ADR 0004 §9): HA's own traces,
// read over the WebSocket (trace/list, trace/get; an administrator's).

// Lauf is one run of an automation as its trace tells it.
type Lauf struct {
	RunID     string
	Start     time.Time
	Execution string // finished, failed_conditions, failed_single, error, cancelled, aborted, running, …
	LastStep  string // the step it stopped at: condition/0, action/1, …
}

type traceSummary struct {
	RunID           string `json:"run_id"`
	ScriptExecution string `json:"script_execution"`
	State           string `json:"state"`
	LastStep        string `json:"last_step"`
	Timestamp       struct {
		Start string `json:"start"`
	} `json:"timestamp"`
}

func (t traceSummary) lauf() Lauf {
	start, _ := time.Parse(time.RFC3339Nano, t.Timestamp.Start)
	exe := t.ScriptExecution
	if exe == "" && t.State == "running" {
		exe = "running"
	}
	return Lauf{RunID: t.RunID, Start: start, Execution: exe, LastStep: t.LastStep}
}

// Laeufe are the runs HA keeps of the automation with config id id,
// newest first as HA lists them or not: the caller sorts.
func (c *Client) Laeufe(ctx context.Context, id string) ([]Lauf, error) {
	var raw []traceSummary
	if err := c.command(ctx, map[string]any{"type": "trace/list", "domain": "automation", "item_id": id}, &raw); err != nil {
		return nil, err
	}
	out := make([]Lauf, 0, len(raw))
	for _, t := range raw {
		out = append(out, t.lauf())
	}
	return out, nil
}

// Spur is one run in full: the run, what triggered it, the config it ran.
type Spur struct {
	Lauf
	// Trigger is the trigger's variables: platform, event (an event's or a
	// tag's: event_type, data), alias, …; nil when the trace lacks them
	Trigger map[string]any
	Config  map[string]any
	// Ergebnisse is each step's first result by its path
	Ergebnisse map[string]map[string]any
}

// Spur reads one run of the automation id.
func (c *Client) Spur(ctx context.Context, id, runID string) (Spur, error) {
	var raw struct {
		traceSummary
		Config map[string]any `json:"config"`
		Trace  map[string][]struct {
			ChangedVariables struct {
				Trigger map[string]any `json:"trigger"`
			} `json:"changed_variables"`
			Result json.RawMessage `json:"result"`
		} `json:"trace"`
	}
	if err := c.command(ctx, map[string]any{"type": "trace/get", "domain": "automation", "item_id": id, "run_id": runID}, &raw); err != nil {
		return Spur{}, err
	}
	s := Spur{Lauf: raw.lauf(), Config: raw.Config, Ergebnisse: map[string]map[string]any{}}
	for path, steps := range raw.Trace {
		if len(steps) == 0 {
			continue
		}
		if t := steps[0].ChangedVariables.Trigger; t != nil && s.Trigger == nil {
			s.Trigger = t
		}
		var r map[string]any
		if json.Unmarshal(steps[0].Result, &r) == nil {
			s.Ergebnisse[path] = r
		}
	}
	return s, nil
}

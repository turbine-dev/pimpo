// Package trace describes a recorded exploration: what the user asked,
// every capability call the agent made with its result, and what a correct
// outcome looks like. The compiler turns a trace into a routine.
package trace

import (
	"encoding/json"
	"fmt"
	"os"
)

type Trace struct {
	ID      string `json:"id"`
	Request string `json:"request"`
	// Now is the wall clock during the exploration, RFC 3339.
	Now   string `json:"now"`
	Calls []Call `json:"calls"`
	// Judgments are the decisions the agent made while exploring, keyed by
	// judgment name, then by a substring that identifies the item.
	Judgments map[string]map[string]float64 `json:"judgments,omitempty"`
	// Questions holds the wording the agent used for each judgment.
	Questions map[string]string `json:"questions,omitempty"`
	// Outcome is what the user approved at the end of the exploration.
	Outcome string   `json:"outcome"`
	Expect  []Expect `json:"expect"`
	// Event is what started the recorded run, for a routine started by
	// something else than a clock (a webhook's call, an answer).
	Event any `json:"event,omitempty"`
	// Holdout is a second scenario the compiler never sees. A routine that
	// only memorized the recording fails it.
	Holdout *Scenario `json:"holdout,omitempty"`
}

type Call struct {
	Capability string          `json:"capability"`
	Args       json.RawMessage `json:"args"`
	Result     json.RawMessage `json:"result"`
	// Error is why the call failed, such as a service not connected.
	Error string `json:"error,omitempty"`
}

// Scenario is a set of inputs and the calls a correct routine makes with them.
type Scenario struct {
	Now       string                        `json:"now"`
	Responses []Response                    `json:"responses"`
	Judgments map[string]map[string]float64 `json:"judgments,omitempty"`
	Expect    []Expect                      `json:"expect"`
	// Params sets the routine's parameters for this scenario; the rest keep
	// their defaults.
	Params map[string]any `json:"params,omitempty"`
	// Event is what wakes a watching routine in this scenario.
	Event any `json:"event,omitempty"`
	// Writes are canned texts for the routine's writes: name -> identifying
	// substring of the input -> text.
	Writes map[string]map[string]string `json:"writes,omitempty"`
	// State is what the routine kept from earlier runs, and ExpectState
	// the values it must have kept after this one (key -> value).
	State       map[string]any `json:"state,omitempty"`
	ExpectState map[string]any `json:"expect_state,omitempty"`
	// World says the responses are what the service holds, not its answer
	// to the routine's own question: the replay of an exploration and a
	// holdout are, a routine's tests are not. Searches over the world
	// apply every filter the routine asked for, dates and categories
	// included.
	World bool `json:"-"`
}

// Response is a canned result for one call to a read capability. Calls to
// the same capability consume responses in order.
type Response struct {
	Capability string          `json:"capability"`
	Result     json.RawMessage `json:"result"`
}

// Expect checks the write calls a routine makes to one capability.
type Expect struct {
	Capability  string   `json:"capability"`
	Count       *int     `json:"count,omitempty"`
	Contains    []string `json:"contains,omitempty"`
	NotContains []string `json:"not_contains,omitempty"`
}

func Load(path string) (Trace, error) {
	var t Trace
	b, err := os.ReadFile(path)
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return t, fmt.Errorf("parse trace %s: %w", path, err)
	}
	if t.ID == "" || t.Request == "" || len(t.Calls) == 0 || len(t.Expect) == 0 {
		return t, fmt.Errorf("trace %s: id, request, calls and expect are required", path)
	}
	if t.Holdout != nil {
		t.Holdout.World = true
	}
	return t, nil
}

// Replay is the scenario recorded in the trace itself.
func (t Trace) Replay() Scenario {
	s := Scenario{Now: t.Now, Judgments: t.Judgments, Expect: t.Expect, World: true, Event: t.Event}
	for _, c := range t.Calls {
		if len(c.Result) > 0 {
			s.Responses = append(s.Responses, Response{Capability: c.Capability, Result: c.Result})
		}
	}
	return s
}

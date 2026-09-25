// Package host carries out capability calls for routines and explorations.
// Every call passes the policy, reaches the world through a connector, and
// is written to the event log with its arguments and result.
package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/denerFernandes/zodim/internal/budget"
	"github.com/denerFernandes/zodim/internal/capability"
	"github.com/denerFernandes/zodim/internal/connector"
	"github.com/denerFernandes/zodim/internal/event"
	"github.com/denerFernandes/zodim/internal/judge"
	"github.com/denerFernandes/zodim/internal/people"
	"github.com/denerFernandes/zodim/internal/policy"
	"github.com/denerFernandes/zodim/internal/trace"
)

const (
	ActionEvent   = "action.done"
	JudgmentEvent = "judgment.made"
)

// Env is what every host shares; a Host is one run of one routine or one
// exploration on top of it.
type Env struct {
	Router *connector.Router
	Judge  judge.Judge
	Budget *budget.Budget
	Events *event.Store
	Policy policy.Policy
	// Approver asks the owner when a rule says so; nil denies.
	Approver Approver
	// Remember turns an "always" answer into a lasting permission.
	Remember func(ctx context.Context, a policy.Action)
	// Write composes a short text with a small model; nil means routines
	// that write cannot run.
	Write func(ctx context.Context, instruction string, input any) (text string, costUSD float64, err error)
	// RoleOf names a person's role in the house; nil treats everyone as
	// the owner.
	RoleOf func(ctx context.Context, person string) string
}

type Approver interface {
	Ask(ctx context.Context, a policy.Action, reason string) (always bool, err error)
}

// reversibleForm is how an irreversible capability is done when the policy
// asks for a reversible version: delete goes to the trash, sending waits in
// the outbox long enough to be cancelled.
var reversibleForm = map[string]string{
	"gmail.delete": "gmail.trash",
	"gmail.send":   "outbox.send_later",
}

type Host struct {
	Env
	// Source identifies the run in events: "routine:brief#12", "exploration:e1".
	Source string
	// Person is who the run acts for. Connectors see it in the context and
	// use that person's accounts; empty means the owner.
	Person string
	// Destinations are where notify.send delivers for this run; empty
	// means the person's default channel.
	Destinations []string
	// DryRun records changes instead of making them. Explorations run this
	// way: they show what would happen and change nothing but messages to
	// the owner.
	DryRun bool
	// Allowed, when set, is every capability this run may use; an
	// assistant limited to a few tools runs this way.
	Allowed map[string]bool
	// QuietReads leaves successful reads out of the event log; a watch
	// polling every few minutes would otherwise bury the receipts.
	QuietReads bool

	mu        sync.Mutex
	calls     []trace.Call
	judgments map[string]map[string]float64
	questions map[string]string
	costUSD   float64
}

// SetQuestion remembers the wording of a judgment the explorer made.
func (h *Host) SetQuestion(name, q string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.questions == nil {
		h.questions = map[string]string{}
	}
	if q != "" {
		h.questions[name] = q
	}
}

func (h *Host) Questions() map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.questions
}

// ActionRecord is the data of an ActionEvent.
type ActionRecord struct {
	Source     string          `json:"source"`
	Person     string          `json:"person,omitempty"`
	Capability string          `json:"capability"`
	Scope      string          `json:"scope,omitempty"`
	Risk       string          `json:"risk"`
	Args       any             `json:"args"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Verdict    policy.Verdict  `json:"verdict"`
	Reason     string          `json:"reason,omitempty"`
	Rule       string          `json:"rule,omitempty"`
	DryRun     bool            `json:"dry_run,omitempty"`
	// Done is the capability actually called when the policy swapped in a
	// reversible form, e.g. gmail.trash for gmail.delete.
	Done     string `json:"done,omitempty"`
	Approved string `json:"approved,omitempty"`
	Millis   int64  `json:"ms"`
}

var ErrBlocked = errors.New("blocked by a rule")

type destinationsKey struct{}

// DestinationsFrom returns the destinations of the run ctx belongs to.
func DestinationsFrom(ctx context.Context) []string {
	d, _ := ctx.Value(destinationsKey{}).([]string)
	return d
}

func (h *Host) Call(ctx context.Context, name, scope string, args any) (any, error) {
	spec := capability.Catalog[name]
	person := people.Norm(h.Person)
	ctx = people.With(ctx, person)
	ctx = context.WithValue(ctx, destinationsKey{}, h.Destinations)
	rec := ActionRecord{Source: h.Source, Capability: name, Scope: scope, Risk: spec.Risk.String(), Args: args}
	if person != people.OwnerID {
		rec.Person = person
	}
	pol := h.Policy
	if pol == nil {
		pol = policy.Open{}
	}
	act := policy.Action{Capability: name, Scope: scope, Args: args, Risk: spec.Risk, Source: h.Source, Person: person, Role: string(people.Owner)}
	if h.RoleOf != nil {
		act.Role = h.RoleOf(ctx, person)
	}
	d := pol.Decide(ctx, act)
	if h.Allowed != nil && !h.Allowed[name] {
		d = policy.Decision{Verdict: policy.Block, Reason: "this assistant may not use " + name}
	}
	rec.Verdict, rec.Reason, rec.Rule = d.Verdict, d.Reason, d.Rule
	simulated := h.DryRun && spec.Risk >= capability.Reversible

	if d.Verdict == policy.Block {
		rec.Error = d.Reason
		h.record(ctx, rec, nil)
		return nil, fmt.Errorf("%w: %s", ErrBlocked, d.Reason)
	}
	if d.Verdict == policy.Ask && !simulated {
		if h.Approver == nil {
			rec.Error = "needs approval and nobody can answer"
			h.record(ctx, rec, nil)
			return nil, fmt.Errorf("%s needs your approval: %s", name, d.Reason)
		}
		always, err := h.Approver.Ask(ctx, act, d.Reason)
		if err != nil {
			rec.Error, rec.Approved = err.Error(), "no"
			h.record(ctx, rec, nil)
			return nil, fmt.Errorf("%s was not approved: %w", name, err)
		}
		rec.Approved = "yes"
		if always && h.Remember != nil {
			rec.Approved = "always"
			h.Remember(ctx, act)
		}
	}

	start := time.Now()
	var result any
	var err error
	switch {
	case simulated:
		rec.DryRun = true
		result = map[string]any{"ok": true, "dry_run": true}
	case d.Verdict == policy.Reversible && reversibleForm[name] != "":
		rec.Done = reversibleForm[name]
		result, err = h.Router.Call(ctx, rec.Done, scope, args)
	default:
		result, err = h.Router.Call(ctx, name, scope, args)
	}
	rec.Millis = time.Since(start).Milliseconds()
	if err != nil {
		rec.Error = err.Error()
	}
	h.record(ctx, rec, result)
	return result, err
}

func (h *Host) record(ctx context.Context, rec ActionRecord, result any) {
	var raw json.RawMessage
	if result != nil {
		raw, _ = json.Marshal(result)
	}
	h.mu.Lock()
	h.calls = append(h.calls, trace.Call{Capability: rec.Capability, Args: mustJSON(rec.Args), Result: raw, Error: rec.Error})
	h.mu.Unlock()
	rec.Result = truncate(raw, 16<<10)
	if h.QuietReads && rec.Risk == capability.Read.String() && rec.Error == "" {
		return
	}
	if h.Events != nil {
		h.Events.Append(ctx, ActionEvent, h.Source, rec)
	}
}

// JudgeEstimate is the most one judgment may cost; calls that could pass
// the daily limit are refused before they start.
const JudgeEstimate = 0.01

func (h *Host) Judge(ctx context.Context, name, question string, item any) (float64, error) {
	if h.Budget != nil {
		if err := h.Budget.CheckFor(ctx, JudgeEstimate); err != nil {
			return 0, err
		}
	}
	if h.Env.Judge == nil {
		return 0, errors.New("no judgment backend configured; set one in Settings")
	}
	a, err := h.Env.Judge.Ask(ctx, question, item)
	if err != nil {
		return 0, err
	}
	h.addCost(ctx, a.CostUSD, "judgment")
	if h.Events != nil {
		h.Events.Append(ctx, JudgmentEvent, h.Source, map[string]any{"source": h.Source, "judgment": name, "question": question, "p": a.P, "backend": a.Backend, "item": truncateAny(item)})
	}
	return a.P, nil
}

// WriteEstimate is the most one written text may cost.
const WriteEstimate = 0.02

// WriteEvent records a text a routine had a model write.
const WriteEvent = "text.written"

// Write lets a routine have a small model compose text, within budget.
func (h *Host) Write(ctx context.Context, name, instruction string, input any) (string, error) {
	if h.Budget != nil {
		if err := h.Budget.CheckFor(ctx, WriteEstimate); err != nil {
			return "", err
		}
	}
	if h.Env.Write == nil {
		return "", errors.New("no model is set up to write text; see Settings › Models")
	}
	text, cost, err := h.Env.Write(ctx, instruction, input)
	if err != nil {
		return "", err
	}
	h.addCost(ctx, cost, "writing")
	if h.Events != nil {
		h.Events.Append(ctx, WriteEvent, h.Source, map[string]any{"source": h.Source, "write": name, "instruction": instruction, "text": text, "cost_usd": cost})
	}
	return text, nil
}

// Label records a decision the explorer made, keyed by an item reference.
func (h *Host) Label(name, item string, p float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.judgments == nil {
		h.judgments = map[string]map[string]float64{}
	}
	if h.judgments[name] == nil {
		h.judgments[name] = map[string]float64{}
	}
	h.judgments[name][item] = p
}

func (h *Host) addCost(ctx context.Context, usd float64, source string) {
	if usd <= 0 {
		return
	}
	h.mu.Lock()
	h.costUSD += usd
	h.mu.Unlock()
	if h.Budget != nil {
		h.Budget.Record(ctx, budget.Cost{USD: usd, Source: source, Ref: h.Source})
	}
}

// AddCost books a model call made on this run's behalf.
func (h *Host) AddCost(ctx context.Context, usd float64, source string) { h.addCost(ctx, usd, source) }

func (h *Host) Calls() []trace.Call {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]trace.Call(nil), h.calls...)
}

func (h *Host) Judgments() map[string]map[string]float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.judgments
}

func (h *Host) Cost() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.costUSD
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func truncate(raw json.RawMessage, n int) json.RawMessage {
	if len(raw) <= n {
		return raw
	}
	b, _ := json.Marshal(map[string]any{"truncated": true, "bytes": len(raw), "preview": string(raw[:n])})
	return b
}

func truncateAny(v any) any {
	b, _ := json.Marshal(v)
	if len(b) <= 4096 {
		return v
	}
	return string(b[:4096]) + "…"
}

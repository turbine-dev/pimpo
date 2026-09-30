package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Once a week Pimpo learns the owner's preferences from the owner's own
// words and choices: the requests they made, the approvals they denied or
// made permanent, the suggestions they declined. Never from an email, a
// page or anything else the agent read. What it learns goes into memory
// as learned (topic "preferências"), with where it came from; it guides
// answers as learned, never as a reason to pass a rule, and the owner
// confirms or removes each one in Memória. A preference removed is not
// learned again.

const (
	learnLastKey    = "learn.last"
	learnGivenKey   = "learn.given"
	learnTopic      = "preferências"
	learnMaxCost    = 0.03
	learnEvery      = 7 * 24 * time.Hour
	learnMaxPerWeek = 5
)

func (a *App) learnLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if a.dueToLearn(ctx, time.Now()) {
				a.learn(ctx)
			}
		}
	}
}

func (a *App) dueToLearn(ctx context.Context, now time.Time) bool {
	if a.Settings(ctx).LearnOff || a.LLM == nil || a.Memory == nil {
		return false
	}
	last, _ := a.Events.Get(ctx, learnLastKey)
	t, err := time.Parse(time.RFC3339, last)
	return err != nil || now.Sub(t) >= learnEvery
}

var learnSchema = json.RawMessage(`{"type":"object","properties":{"preferences":{"type":"array","maxItems":5,"items":{"type":"object","properties":{"text":{"type":"string"},"evidence":{"type":"string"}},"required":["text","evidence"]}}},"required":["preferences"]}`)

// learn runs one round and returns what it learned.
func (a *App) learn(ctx context.Context) []memory.Fact {
	a.Events.Put(ctx, learnLastKey, time.Now().UTC().Format(time.RFC3339))
	if a.Budget != nil && a.Budget.Check(ctx) != nil {
		return nil
	}
	evidence := a.ownerEvidence(ctx, time.Now().Add(-learnEvery))
	if len(evidence["requests"].([]string))+len(evidence["decisions"].([]string)) < 3 {
		return nil // too little to learn anything real
	}
	known, _ := a.Memory.InstructionsFor("")
	learned, _ := a.Memory.LearnedFor("")
	var have []string
	for _, f := range append(known, learned...) {
		have = append(have, f.Text)
	}
	evidence["already_known"] = have
	evidence["removed_before"] = a.removedLearned(ctx)
	b, _ := json.Marshal(evidence)
	resp, err := a.LLM.Generate(ctx, llm.Request{
		System: "You infer a person's lasting preferences about how their personal agent should work for them, from their own requests and decisions over the last week. " +
			"Only state a preference the evidence clearly shows at least twice, or that the person stated outright (\"always in Portuguese\", \"never before 8\", \"short answers\"). " +
			"Do not repeat already_known, and never state anything in removed_before or close to it. Nothing about other people's private matters, no guesses about health, money or relationships. " +
			"text: one short sentence in the person's language (" + i18n.Of(ctx) + "), as an instruction to the agent; evidence: which requests or decisions show it, briefly. An empty list is a good answer.",
		Prompt: string(b),
		Schema: learnSchema, Model: a.Settings(ctx).JudgeModel, MaxCostUSD: learnMaxCost,
	})
	if a.Budget != nil {
		a.Budget.Record(ctx, budgetCost(resp.CostUSD, "learning"))
	}
	if err != nil {
		a.Events.Append(ctx, "learn.failed", "system", map[string]string{"error": err.Error()})
		return nil
	}
	var out struct {
		Preferences []struct{ Text, Evidence string } `json:"preferences"`
	}
	json.Unmarshal(resp.Structured, &out)
	removed := a.removedLearned(ctx)
	var made []memory.Fact
	for _, p := range out.Preferences {
		text := clip(p.Text, 200)
		if text == "" || len(made) == learnMaxPerWeek || containsFold(removed, text) {
			continue
		}
		f, err := a.Memory.AddFrom(text, learnTopic, "aprendido: "+clip(p.Evidence, 160), memory.Learned, "", memory.Origin{Kind: memory.FromLearned, Label: clip(p.Evidence, 80)})
		if err != nil {
			continue
		}
		made = append(made, f)
		a.rememberGiven(ctx, text)
		a.Events.Append(ctx, "learn.added", "system", map[string]string{"id": f.ID, "text": text, "evidence": clip(p.Evidence, 160)})
	}
	return made
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// ownerEvidence gathers the owner's own words and choices since t.
// Suggestions stay out: their titles were written by a model from email
// metadata, so even the owner's click on one would carry a sender's words.
func (a *App) ownerEvidence(ctx context.Context, since time.Time) map[string]any {
	requests, decisions := []string{}, []string{}
	evs, _ := a.Events.List(ctx, event.Query{Types: []string{"exploration.started", "approval.requested", "approval.resolved"}, Newest: true, Limit: 2000})
	asked := map[string]string{}
	for i := len(evs) - 1; i >= 0; i-- {
		e := evs[i]
		if e.Time.Before(since) {
			continue
		}
		var d struct {
			Request, ID, Answer string
			Action              struct{ Capability string }
		}
		e.Decode(&d)
		switch e.Type {
		case "exploration.started":
			if e.Actor == "human:owner" && d.Request != "" && len(requests) < 60 {
				requests = append(requests, clip(d.Request, 300))
			}
		case "approval.requested":
			asked[d.ID] = d.Action.Capability
		case "approval.resolved":
			if e.Actor == "human:owner" && (d.Answer == "deny" || d.Answer == "always") {
				decisions = append(decisions, d.Answer+" "+asked[d.ID])
			}
		}
	}
	return map[string]any{"requests": requests, "decisions": decisions}
}

// rememberGiven keeps what was learned, so a preference the owner later
// removes from memory is known as removed and not learned again.
func (a *App) rememberGiven(ctx context.Context, text string) {
	raw, _ := a.Events.Get(ctx, learnGivenKey)
	var given []string
	json.Unmarshal([]byte(raw), &given)
	given = append(given, text)
	if len(given) > 200 {
		given = given[len(given)-200:]
	}
	b, _ := json.Marshal(given)
	a.Events.Put(ctx, learnGivenKey, string(b))
}

// removedLearned are preferences once learned that are no longer in
// memory: the owner removed them.
func (a *App) removedLearned(ctx context.Context) []string {
	raw, _ := a.Events.Get(ctx, learnGivenKey)
	var given []string
	json.Unmarshal([]byte(raw), &given)
	facts, _ := a.Memory.List()
	out := []string{}
	for _, g := range given {
		present := false
		for _, f := range facts {
			if strings.EqualFold(f.Text, g) {
				present = true
				break
			}
		}
		if !present {
			out = append(out, g)
		}
	}
	return out
}

func (a *App) learnRoutes() {
	// Now learns at once, for the owner who wants to see it work.
	a.Server.Handle("POST /api/learn", func(w http.ResponseWriter, r *http.Request) {
		if !ownerOnly(w, r) {
			return
		}
		if a.Memory == nil {
			server.WriteError(w, server.StatusError{Status: 503, Msg: "no memory"})
			return
		}
		server.WriteJSON(w, 200, map[string]any{"learned": a.learn(r.Context())})
	})
}

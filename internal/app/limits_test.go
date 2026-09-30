package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
)

const (
	cheapModel  = "openai:gpt-5-mini"
	midModel    = "anthropic:claude-sonnet-5"
	strongModel = "anthropic:claude-opus-5"
)

// priced gives the house three API models, the middle one for tasks.
func priced(t *testing.T, ta *testApp) {
	t.Helper()
	s := ta.Settings(t.Context())
	s.Models = []ModelOption{{ID: cheapModel, PriceIn: 0.25, PriceOut: 2}, {ID: strongModel, PriceIn: 5, PriceOut: 25}, {ID: midModel, PriceIn: 2, PriceOut: 10}}
	s.ExploreModel = midModel
	if code, out := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
}

// A call may use what the house, the person and the assistant all allow.
func TestAllowedModelsAreTheIntersection(t *testing.T) {
	h := newHouse(t)
	priced(t, h.testApp)
	ctx := t.Context()
	if code, out := h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"models": []string{cheapModel, midModel}}); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	ana := people.With(ctx, h.anaID)
	if list, restricted := h.allowedModels(ana); !restricted || !slices.Equal(list, []string{cheapModel, midModel}) {
		t.Fatalf("Ana's models %v %v", list, restricted)
	}
	withRole := withAssistant(ana, []string{midModel, strongModel})
	if list, _ := h.allowedModels(withRole); !slices.Equal(list, []string{midModel}) {
		t.Fatalf("Ana with the assistant %v", list)
	}
	if list, _ := h.allowedModels(withAssistant(ctx, []string{midModel, strongModel})); !slices.Equal(list, []string{midModel, strongModel}) {
		t.Fatalf("the owner with the assistant %v", list)
	}
	if _, restricted := h.allowedModels(ctx); restricted {
		t.Fatal("the owner is limited to less than the house")
	}
	// A model that leaves the house leaves every list.
	s := h.Settings(ctx)
	s.Models = s.Models[1:]
	s.ExploreModel = strongModel
	h.do(t, "PUT", "/api/settings", s)
	if list, _ := h.allowedModels(ana); !slices.Equal(list, []string{midModel}) {
		t.Fatalf("after the house dropped a model %v", list)
	}
	// Nothing left: the call is refused, not run on something else.
	h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"models": []string{midModel}})
	if _, err := h.runAgent(withAssistant(ana, []string{strongModel}), llm.AgentRequest{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "administrador") {
		t.Fatalf("ran with no model allowed: %v", err)
	}
}

// The automatic choice picks only among the allowed models, the cheapest
// when its own pick is not one of them; chains keep to them too.
func TestAutomaticChoiceStaysInsideTheAllowed(t *testing.T) {
	h := newHouse(t)
	priced(t, h.testApp)
	ctx := t.Context()
	defer func(old func(*App) (chooser, bool)) { tierChooser = old }(tierChooser)
	h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"models": []string{cheapModel, strongModel}})
	ana := people.With(ctx, h.anaID)

	tierChooser = func(*App) (chooser, bool) { return fixedChooser{"hard": 0.9}, true }
	if r := h.routeModel(ana, "planeje", "", "", ""); r.Model != strongModel {
		t.Fatalf("hard for Ana %+v", r)
	}
	// Doubt goes to the tasks model, which Ana may not use: the cheapest.
	tierChooser = func(*App) (chooser, bool) { return fixedChooser{"normal": 0.9}, true }
	if r := h.routeModel(ana, "hmm", "", "", ""); r.Model != cheapModel || r.By != "allowed" {
		t.Fatalf("normal for Ana %+v", r)
	}
	if r := h.routeModel(ctx, "hmm", "", "", ""); r.Model != midModel {
		t.Fatalf("the owner was limited %+v", r)
	}
	// A stale fixed choice is kept inside too.
	if r := h.routeModel(ana, "hmm", "", midModel, ""); r.Model != cheapModel {
		t.Fatalf("fixed outside %+v", r)
	}
	if got := h.allowedChain(ana, []string{midModel, strongModel}); !slices.Equal(got, []string{strongModel}) {
		t.Fatalf("chain %v", got)
	}
	if got := h.allowedChain(ana, []string{midModel}); !slices.Equal(got, []string{cheapModel}) {
		t.Fatalf("chain with nothing allowed %v", got)
	}
	// The agent itself runs on an allowed model, whatever was asked.
	var seen []string
	h.Agent = llm.FakeAgent{Script: func(_ context.Context, r llm.AgentRequest) (llm.Response, error) {
		seen = append(seen, r.Model)
		return llm.Response{Text: "ok"}, nil
	}}
	h.runAgent(ana, llm.AgentRequest{Prompt: "x", Model: midModel})
	h.runAgent(ctx, llm.AgentRequest{Prompt: "x", Model: midModel})
	if !slices.Equal(seen, []string{cheapModel, midModel}) {
		t.Fatalf("ran on %v", seen)
	}
}

// A model chosen by hand outside what is allowed is refused, with why.
func TestAnExplicitModelOutsideIsRefused(t *testing.T) {
	h := newHouse(t)
	priced(t, h.testApp)
	h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"models": []string{cheapModel}})
	code, body := h.raw(t, h.ana, "POST", "/api/chats", js(map[string]string{"text": "oi", "model": strongModel}))
	if code != 403 || !strings.Contains(body, "administrador") {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := h.raw(t, h.ana, "POST", "/api/chats", js(map[string]string{"text": "oi", "model": cheapModel})); code != 201 {
		t.Fatalf("%d %s", code, body)
	}
	// An assistant's own list refuses the owner too.
	h.do(t, "PUT", "/api/assistants/escritor", map[string]any{"name": "Escritor", "models": []string{midModel}})
	code, out := h.do(t, "POST", "/api/chats", map[string]string{"text": "oi", "model": strongModel, "assistant": "escritor"})
	if code != 400 || !strings.Contains(out["error"].(string), "assistente") {
		t.Fatalf("%d %v", code, out)
	}
	if code, out := h.do(t, "PUT", "/api/assistants/escritor", map[string]any{"name": "Escritor", "models": []string{"openai:nope"}}); code != 400 {
		t.Fatalf("an assistant took a model the house lacks: %d %v", code, out)
	}
	// A routine's model is chosen by hand as well.
	if code, body := h.raw(t, h.ana, "PUT", "/api/routines/"+h.anas["routine"]+"/settings", js(map[string]string{"model": strongModel})); code != 403 {
		t.Fatalf("Ana's routine took a model she may not use: %d %s", code, body)
	}
	h.Explore.Wait()
}

// A person's own limit stops their calls; the house's still stops
// everyone's; the owner is held only by the house's.
func TestAPersonsLimitBlocksOnlyThem(t *testing.T) {
	h := newHouse(t)
	ctx := t.Context()
	h.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 2})
	if code, out := h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"daily_usd": 3}); code != 400 {
		t.Fatalf("a limit above the house's: %d %v", code, out)
	}
	if code, out := h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"daily_usd": 0.5}); code != 200 || out["daily_limit"] != 0.5 {
		t.Fatalf("%d %v", code, out)
	}
	ana := people.With(ctx, h.anaID)
	h.Budget.Record(ana, budget.Cost{USD: 0.5, Source: "exploration"})
	if err := h.Budget.Check(ana); !errors.Is(err, budget.ErrPersonOverBudget) || !errors.Is(err, budget.ErrOverBudget) {
		t.Fatalf("Ana over her limit: %v", err)
	}
	if err := h.Budget.Check(ctx); err != nil {
		t.Fatalf("the owner held by Ana's limit: %v", err)
	}
	code, body := h.raw(t, h.ana, "POST", "/api/chats", js(map[string]string{"text": "oi"}))
	if code != 400 || !strings.Contains(body, "limite") {
		t.Fatalf("Ana's chat went on: %d %s", code, body)
	}
	// The house's limit still holds for everyone.
	h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"daily_usd": 2})
	h.Budget.Record(ctx, budget.Cost{USD: 1.6, Source: "exploration"})
	if err := h.Budget.Check(ana); !errors.Is(err, budget.ErrOverBudget) || errors.Is(err, budget.ErrPersonOverBudget) {
		t.Fatalf("the house's limit for Ana: %v", err)
	}
	// The owner sees whether Ana's limit was reached, never how much she
	// spent; Ana sees her own.
	h.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 10})
	h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"daily_usd": 0.5})
	_, list := h.raw(t, "tok", "GET", "/api/people", nil)
	if !strings.Contains(list, `"limit_reached":true`) || strings.Contains(list, "spent") {
		t.Fatalf("people %s", list)
	}
	// Her own spending: her chat's exploration and the cost above, not
	// the owner's 1.6.
	_, mine := h.raw(t, h.ana, "GET", "/api/me/limits", nil)
	if !strings.Contains(mine, `"spent_today":0.6`) || !strings.Contains(mine, `"reached":true`) || !strings.Contains(mine, `"daily_usd":0.5`) {
		t.Fatalf("Ana's limits %s", mine)
	}
	if code, _ := h.raw(t, h.ana, "PUT", "/api/people/"+h.anaID+"/limits", js(map[string]any{"daily_usd": 5})); code == 200 {
		t.Fatal("Ana raised her own limit")
	}
	if code, _ := h.do(t, "PUT", "/api/people/owner/limits", map[string]any{"daily_usd": 1}); code != 400 {
		t.Fatal("the owner's limits are the house's")
	}
}

// A guest has a small limit of their own until the owner sets one.
func TestGuestsHaveASmallLimitOfTheirOwn(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := t.Context()
	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 5})
	_, out := ta.do(t, "POST", "/api/people", map[string]string{"name": "Visita", "role": "guest"})
	id := out["id"].(string)
	guest := people.With(ctx, id)
	if l := ta.Budget.LimitFor(ctx, id); l != people.GuestDailyUSD {
		t.Fatalf("guest limit %v", l)
	}
	ta.Budget.Record(guest, budget.Cost{USD: people.GuestDailyUSD, Source: "exploration"})
	if err := ta.Budget.Check(guest); !errors.Is(err, budget.ErrPersonOverBudget) {
		t.Fatalf("guest over the default: %v", err)
	}
	ta.do(t, "PUT", "/api/people/"+id+"/limits", map[string]any{"daily_usd": 1})
	if err := ta.Budget.Check(guest); err != nil {
		t.Fatalf("guest with a higher limit: %v", err)
	}
	// Someone unknown is held like a guest.
	if l := ta.personLimit(ctx, "ninguem"); l != people.GuestDailyUSD {
		t.Fatalf("unknown %v", l)
	}
}

package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/llm"
)

func TestEveryCostHasItsMember(t *testing.T) {
	ta, co := shopApp(t, llm.FakeAgent{Script: func(context.Context, llm.AgentRequest) (llm.Response, error) {
		return llm.Response{Text: "done", CostUSD: 0.02}, nil
	}})
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	w, _ := ta.enqueue(ctx, o, "clara", "Tidy up", nil, "test", 0)
	ta.waitWorkState(t, w.ID, company.WorkDone)
	_, out := ta.do(t, "GET", "/api/companies/"+co+"/costs", nil)
	members := out["members"].(map[string]any)
	if got := members["clara"].(map[string]any)["month"].(float64); got < 0.0199 {
		t.Fatalf("costs = %v", out)
	}
	if got := out["company"].(map[string]any)["day"].(float64); got < 0.0199 {
		t.Fatalf("company spend = %v", out["company"])
	}
}

func TestABudgetReachedHoldsTheWork(t *testing.T) {
	var mu sync.Mutex
	var caps []float64
	ta, co := shopApp(t, llm.FakeAgent{Script: func(_ context.Context, r llm.AgentRequest) (llm.Response, error) {
		mu.Lock()
		caps = append(caps, r.MaxCostUSD)
		mu.Unlock()
		return llm.Response{Text: "done"}, nil
	}})
	ctx := context.Background()
	// The house's own limit stays out of the way; this is about the member's.
	ta.Budget.SetLimit(ctx, 10, "test")
	base := "/api/companies/" + co
	ta.do(t, "PUT", base+"/members/clara", map[string]any{"name": "Clara", "role": "clerk", "reports_to": "ceo", "budget": map[string]any{"month_usd": 1}})
	ta.Budget.Record(ctx, budget.Cost{USD: 0.8, Source: "exploration", Member: co + "/clara"})
	o, _ := ta.Companies.Org(ctx, co)
	first, _ := ta.enqueue(ctx, o, "clara", "first", nil, "test", 0.5)
	ta.waitWorkState(t, first.ID, company.WorkDone)
	mu.Lock()
	if len(caps) != 1 || caps[0] > 0.2001 {
		t.Fatalf("the work was not capped at what is left: %v", caps)
	}
	mu.Unlock()
	ta.Budget.Record(ctx, budget.Cost{USD: 0.2, Source: "exploration", Member: co + "/clara"})
	held, _ := ta.enqueue(ctx, o, "clara", "second", nil, "test", 0)
	ta.pumpWork(ctx)
	if w, _ := ta.Companies.Work(ctx, held.ID); w.State != company.WorkQueued {
		t.Fatalf("work past the budget started: %+v", w)
	}
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"company.budget"}})
	warned := 0
	for _, e := range evs {
		if strings.Contains(string(e.Data), "budget reached") {
			warned++
		}
	}
	ta.pumpWork(ctx)
	if evs2, _ := ta.Events.List(ctx, event.Query{Types: []string{"company.budget"}}); warned != 1 || len(evs2) != len(evs) {
		t.Fatalf("the CEO was warned %d times, then %d events", warned, len(evs2))
	}
	// A company that only warns lets the work go on.
	o, _ = ta.Companies.Org(ctx, co)
	o.Budget.OnLimit = company.OnLimitWarn
	ta.Companies.Update(ctx, o.Company)
	ta.pumpWork(ctx)
	ta.waitWorkState(t, held.ID, company.WorkDone)
}

func TestACompanyCannotOutspendItsPerson(t *testing.T) {
	h := newHouse(t)
	h.do(t, "PUT", "/api/people/"+h.anaID+"/limits", map[string]any{"daily_usd": 1})
	co := h.anas["company"]
	_, body := h.raw(t, h.ana, "GET", "/api/companies/"+co, nil)
	if code, out := h.raw(t, h.ana, "PUT", "/api/companies/"+co, js(map[string]any{"name": "Ana Co", "budget": map[string]any{"day_usd": 5}})); code != 400 || !strings.Contains(out, "own limit") {
		t.Fatalf("a company got more than its person: %d %s (%s)", code, out, body[:60])
	}
	if code, _ := h.raw(t, h.ana, "PUT", "/api/companies/"+co, js(map[string]any{"name": "Ana Co", "budget": map[string]any{"day_usd": 0.5}})); code != 200 {
		t.Fatalf("a budget inside the limit: %d", code)
	}
}

func TestAForecastFromTheSchedules(t *testing.T) {
	ta, co := shopApp(t, weatherAgent)
	ta.do(t, "PUT", "/api/companies/"+co+"/agent-routines/morning", map[string]any{"member": "clara", "name": "Morning", "instructions": "Read the orders", "schedule": "0 8 * * *", "max_usd": 1})
	_, out := ta.do(t, "GET", "/api/companies/"+co+"/costs", nil)
	if f := out["forecast_month"].(float64); f < 29 || f > 31 {
		t.Fatalf("a daily $1 routine forecasts $%.2f a month", f)
	}
	if r := out["per_role"].(map[string]any)["clerk"].(float64); r < 29 {
		t.Fatalf("per role = %v", out["per_role"])
	}
}

func TestSubscriptionWorkCountsWhenTheCompanySays(t *testing.T) {
	ta, co := shopApp(t, weatherAgent)
	ctx := context.Background()
	old := onSubscription
	onSubscription = func(string) bool { return true }
	defer func() { onSubscription = old }()
	if resp := ta.billed(host.WithMember(ctx, co+"/clara"), "explore", "sonnet", llm.Response{CostUSD: 0.9}); resp.CostUSD != 0 {
		t.Fatalf("subscription work cost money: %+v", resp)
	}
	ta.do(t, "PUT", "/api/companies/"+co+"/members/clara", map[string]any{"name": "Clara", "role": "clerk", "reports_to": "ceo", "budget": map[string]any{"month_usd": 0.5}})
	o, _ := ta.Companies.Org(ctx, co)
	s := ta.spendOf(ctx, o)
	if s.Members["clara"].Month != 0 || s.Subscription == nil || s.Subscription.Members["clara"].Month != 0.9 {
		t.Fatalf("spend = %+v", s)
	}
	if ok, _ := ta.budgetGate(ctx, o, company.Work{Member: "clara", MaxUSD: 0.1}, s); !ok {
		t.Fatal("subscription work was counted though the company did not ask")
	}
	o.Budget.Subscription = true
	if ok, _ := ta.budgetGate(ctx, o, company.Work{Member: "clara", MaxUSD: 0.1}, s); ok {
		t.Fatal("subscription work at API prices did not count")
	}
}

func TestAnOutOfTurnsSubscriptionWaitsForItsWindow(t *testing.T) {
	ta, co := shopApp(t, llm.FakeAgent{Script: func(context.Context, llm.AgentRequest) (llm.Response, error) {
		return llm.Response{}, errors.New("Claude AI usage limit reached|1790000000")
	}})
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	w, _ := ta.enqueue(ctx, o, "clara", "Answer the inbox", nil, "test", 0)
	var got company.Work
	ta.waitFor(t, "the work to wait", func() bool {
		got, _ = ta.Companies.Work(ctx, w.ID)
		return got.Retries == 1
	})
	if got.State != company.WorkQueued || time.Until(got.NotBefore) < 50*time.Minute {
		t.Fatalf("work = %+v", got)
	}
	ta.pumpWork(ctx)
	if again, _ := ta.Companies.Work(ctx, w.ID); again.State != company.WorkQueued || again.Retries != 1 {
		t.Fatalf("it ran again before its window: %+v", again)
	}
}

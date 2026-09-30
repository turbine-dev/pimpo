package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/store"
)

func planOf(parts ...string) llm.Response {
	var ps []map[string]any
	for _, p := range parts {
		title, caps, _ := strings.Cut(p, ":")
		c := []string{}
		if caps != "" {
			c = strings.Split(caps, ",")
		}
		ps = append(ps, map[string]any{"title": title, "instructions": "Pesquise " + title, "capabilities": c})
	}
	b, _ := json.Marshal(map[string]any{"parts": ps})
	return llm.Response{Structured: b, CostUSD: 0.01}
}

// partAgent answers each part with its title and records what it was asked.
type partAgent struct {
	mu     sync.Mutex
	prompt []string
}

func (p *partAgent) agent() llm.Agent {
	return llm.FakeAgent{Script: func(_ context.Context, r llm.AgentRequest) (llm.Response, error) {
		p.mu.Lock()
		p.prompt = append(p.prompt, r.Prompt)
		p.mu.Unlock()
		title := r.Prompt[strings.Index(r.Prompt, "(")+1 : strings.Index(r.Prompt, ")")]
		return llm.Response{Text: "Resultado de " + title, CostUSD: 0.1}, nil
	}}
}

func waitJob(t *testing.T, ta *testApp, id string, states ...string) Job {
	t.Helper()
	for i := 0; i < 300; i++ {
		if j, _ := ta.job(context.Background(), id); contains(states, j.State) {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	j, _ := ta.job(context.Background(), id)
	t.Fatalf("job is %s: %+v", j.State, j)
	return j
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestJobPlansRunsPartsAndReports(t *testing.T) {
	pa := &partAgent{}
	fake := &llm.Fake{Responses: []llm.Response{
		planOf("Fornecedor A:web.read,bogus.tool", "Fornecedor B:web.read", "Fornecedor C"),
		{Structured: json.RawMessage(`{"report":"# Proposta\nA é o melhor."}`), CostUSD: 0.02},
	}}
	ta := newApp(t, pa.agent(), fake)
	ta.jobPoll = 10 * time.Millisecond
	code, out := ta.do(t, "POST", "/api/jobs", map[string]any{"request": "Compare 3 fornecedores e prepare uma proposta", "budget_usd": 2})
	if code != 201 {
		t.Fatalf("%d %v", code, out)
	}
	id := out["id"].(string)
	j, _ := ta.job(context.Background(), id)
	if j.State != JobPlanned || len(j.Parts) != 3 {
		t.Fatalf("plan: %+v", j)
	}
	if strings.Join(j.Parts[0].Capabilities, ",") != "web.read" {
		t.Fatalf("an unknown capability reached the plan: %v", j.Parts[0].Capabilities)
	}
	if len(pa.prompt) != 0 {
		t.Fatal("parts ran before the owner started the job")
	}
	if code, _ := ta.do(t, "POST", "/api/jobs/"+id+"/start", nil); code != 202 {
		t.Fatal(code)
	}
	j = waitJob(t, ta, id, JobDone, JobFailed)
	if j.State != JobDone || !strings.Contains(j.Report, "Proposta") {
		t.Fatalf("%+v", j)
	}
	for _, p := range j.Parts {
		if p.State != PartDone || p.Summary != "Resultado de "+p.Title {
			t.Fatalf("part %+v", p)
		}
		e, _ := ta.Store.Exploration(context.Background(), p.Exploration)
		if !strings.Contains(e.Request, "one part") {
			t.Fatalf("the part did not know its place: %q", e.Request)
		}
	}
	if j.SpentUSD < 0.32 || j.SpentUSD > 0.34 {
		t.Fatalf("spent %.2f", j.SpentUSD)
	}
	report := fake.Requests[len(fake.Requests)-1].Prompt
	if !strings.Contains(report, "Resultado de Fornecedor B") {
		t.Fatalf("the report did not see the parts: %s", report)
	}
	if code, _ := ta.do(t, "POST", "/api/jobs/"+id+"/start", nil); code != 409 {
		t.Fatal("a job ran twice")
	}
}

func TestJobStopsAtItsBudget(t *testing.T) {
	pa := &partAgent{}
	ta := newApp(t, pa.agent(), &llm.Fake{Responses: []llm.Response{planOf("A", "B", "C", "D", "E")}})
	ta.jobPoll = 10 * time.Millisecond
	_, out := ta.do(t, "POST", "/api/jobs", map[string]any{"request": "Cinco partes", "budget_usd": 0.25})
	id := out["id"].(string)
	ta.do(t, "POST", "/api/jobs/"+id+"/start", nil)
	j := waitJob(t, ta, id, JobStopped, JobDone)
	// Parts already running when it stopped finish; none starts after.
	for i := 0; i < 200 && slices.ContainsFunc(j.Parts, func(p JobPart) bool { return p.State == PartRunning }); i++ {
		time.Sleep(10 * time.Millisecond)
		j, _ = ta.job(context.Background(), id)
	}
	if j.State != JobStopped || j.Report != "" {
		t.Fatalf("%+v", j)
	}
	// Each part may spend 0.048 of the 0.25; this model spends 0.10, so at
	// most three parts can ever have started, whatever the timing.
	started := 0
	for _, p := range j.Parts {
		if p.Attempts > 0 {
			started++
		}
	}
	if started < 2 || started > 3 {
		t.Fatalf("%d parts started: parts started past the budget (spent %.2f)", started, j.SpentUSD)
	}
}

func TestJobResumesAfterARestart(t *testing.T) {
	pa := &partAgent{}
	ta := newApp(t, pa.agent(), &llm.Fake{Responses: []llm.Response{{Structured: json.RawMessage(`{"report":"ok"}`)}}})
	ta.jobPoll = 10 * time.Millisecond
	ctx := context.Background()
	before := ta.startedAt.Add(-time.Minute)
	stuck := store.Exploration{ID: "old1", Request: "x", State: store.ExplorationRunning}
	ta.Store.SaveExploration(ctx, stuck)
	j := Job{ID: "j1", Request: "Duas partes", State: JobRunning, BudgetUSD: 1, Created: before, Parts: []JobPart{
		{ID: "p1", Title: "Feita", State: PartDone, Summary: "Resultado de Feita", Attempts: 1},
		{ID: "p2", Title: "Interrompida", State: PartRunning, Exploration: "old1", Attempts: 1, Started: before},
	}}
	ta.saveJob(ctx, &j)
	ta.resumeJobs(ctx)
	j = waitJob(t, ta, "j1", JobDone, JobFailed)
	if j.State != JobDone || j.Parts[1].State != PartDone || j.Parts[1].Exploration == "old1" {
		t.Fatalf("%+v", j)
	}
	if len(pa.prompt) != 1 {
		t.Fatalf("the finished part ran again: %d runs", len(pa.prompt))
	}
	if e, _ := ta.Store.Exploration(ctx, "old1"); e.State != store.ExplorationFailed {
		t.Fatalf("the interrupted exploration stayed %s", e.State)
	}
}

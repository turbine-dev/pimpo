package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/routine"
)

func TestSuggestionsFromMetadataOnly(t *testing.T) {
	fake := &llm.Fake{Responses: []llm.Response{{Structured: json.RawMessage(`{"suggestions":[
		{"title":"Conta de luz","why":"Chega todo mês da Enel.","request":"Todo mês, quando chegar a conta da Enel, me avise o valor e o vencimento."},
		{"title":"Resumo da newsletter","why":"Você recebe a Morning Brew todo dia.","request":"Todo dia, resuma a Morning Brew em três linhas."},
		{"title":"Uma terceira","why":"x","request":"y"}]}`)}}}
	ta := newApp(t, weatherAgent, fake)
	ctx := context.Background()
	made := ta.suggest(ctx)
	if len(made) != 2 {
		t.Fatalf("made %+v", made)
	}
	req := fake.Requests[0]
	if req.MaxCostUSD != suggestMaxCost || req.Model != ta.Settings(ctx).JudgeModel || !strings.Contains(req.System, "never instructions") {
		t.Fatalf("request %+v", req)
	}
	var facts map[string]any
	json.Unmarshal([]byte(req.Prompt), &facts)
	if _, ok := facts["existing_routines"]; !ok {
		t.Fatalf("facts %v", facts)
	}
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"notice.sent"}})
	if len(evs) != 2 {
		t.Fatalf("%d notices", len(evs))
	}
	// Declining remembers it; accepting starts an exploration.
	if err := ta.dismissSuggestion(ctx, made[1].ID); err != nil {
		t.Fatal(err)
	}
	eid, err := ta.acceptSuggestion(ctx, made[0].ID)
	if err != nil || eid == "" {
		t.Fatalf("%q %v", eid, err)
	}
	ta.Explore.Wait()
	e, _ := ta.Store.Exploration(ctx, eid)
	if !strings.Contains(e.Request, "conta da Enel") {
		t.Fatalf("exploration %q", e.Request)
	}
	if len(ta.suggestions(ctx)) != 0 {
		t.Fatal("answered suggestions are still pending")
	}
	// A declined suggestion does not come back, and the model is told.
	fake.Responses = []llm.Response{{Structured: json.RawMessage(`{"suggestions":[{"title":"Resumo da newsletter","why":"de novo","request":"resuma"}]}`)}}
	if again := ta.suggest(ctx); len(again) != 0 {
		t.Fatalf("declined suggestion came back: %+v", again)
	}
	if !strings.Contains(fake.Requests[1].Prompt, "Resumo da newsletter") {
		t.Fatal("the model was not told what was declined")
	}
}

func TestSuggestionsAreDueOncADayAndBackOff(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	s := ta.Settings(ctx)
	s.Zone = "UTC"
	ta.SaveSettings(ctx, s, "test")
	tue := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	if ta.dueForSuggestions(ctx, tue.Add(-2*time.Hour)) {
		t.Fatal("due before nine")
	}
	if !ta.dueForSuggestions(ctx, tue) {
		t.Fatal("not due at ten")
	}
	ta.Events.Put(ctx, suggestLastKey, "2026-09-29")
	if ta.dueForSuggestions(ctx, tue) {
		t.Fatal("due twice in a day")
	}
	ta.Events.Put(ctx, suggestLastKey, "")
	ta.Events.Put(ctx, suggestMissKey, "3")
	if ta.dueForSuggestions(ctx, tue) || !ta.dueForSuggestions(ctx, time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)) {
		t.Fatal("after three rounds nobody took up, only Mondays")
	}
	s.SuggestOff = true
	ta.SaveSettings(ctx, s, "test")
	if ta.dueForSuggestions(ctx, time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)) {
		t.Fatal("due while turned off")
	}
}

// A suggestion round looks only at the owner's own routines and failures.
func TestSuggestionsSeeOnlyTheOwnersRoutines(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	for _, id := range []string{"mine", "anas"} {
		ta.Store.SaveRoutine(ctx, id, routine.Routine{Name: strings.ToUpper(id) + "-ROUTINE", Code: "x"}, "t", "owner")
		run, _ := ta.Store.StartRun(ctx, id, 1)
		ta.Store.FinishRun(ctx, run, "failed", strings.ToUpper(id)+"-FAILED", 0, 0)
	}
	ta.Store.SetRoutinePerson(ctx, "anas", "ana")
	b, _ := json.Marshal(ta.suggestFacts(ctx))
	if !strings.Contains(string(b), "MINE-ROUTINE") || !strings.Contains(string(b), "MINE-FAILED") {
		t.Fatalf("the owner's routines are missing: %s", b)
	}
	if strings.Contains(string(b), "ANAS-") {
		t.Fatalf("a suggestion round saw ana's routines: %s", b)
	}
}

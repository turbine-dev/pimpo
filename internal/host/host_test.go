package host

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/vigia/internal/budget"
	"github.com/denerFernandes/vigia/internal/connector"
	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/judge"
	"github.com/denerFernandes/vigia/internal/policy"
)

type fakeMail struct{ archived []string }

func (f *fakeMail) Capabilities() []string { return []string{"gmail.search", "gmail.archive"} }
func (f *fakeMail) Call(_ context.Context, name, _ string, args any) (any, error) {
	if name == "gmail.archive" {
		f.archived = append(f.archived, args.(map[string]any)["id"].(string))
		return map[string]any{"ok": true}, nil
	}
	return []map[string]string{{"id": "m1"}}, nil
}

type blockArchive struct{}

func (blockArchive) Decide(_ context.Context, a policy.Action) policy.Decision {
	if a.Capability == "gmail.archive" {
		return policy.Decision{Verdict: policy.Block, Reason: "no archiving on Sundays", Rule: "r1"}
	}
	return policy.Decision{Verdict: policy.Allow}
}

type fixedJudge struct{ cost float64 }

func (f fixedJudge) Ask(context.Context, string, any) (judge.Answer, error) {
	return judge.Answer{P: 0.8, CostUSD: f.cost, Backend: "llm"}, nil
}

func env(t *testing.T, mail *fakeMail) Env {
	t.Helper()
	ev, err := event.Open(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	return Env{Router: connector.NewRouter(mail), Events: ev, Budget: &budget.Budget{Events: ev}, Judge: fixedJudge{cost: 0.01}}
}

func TestCallsAreRecordedAndPolicyApplies(t *testing.T) {
	mail := &fakeMail{}
	h := &Host{Env: env(t, mail), Source: "routine:triage#1"}
	ctx := context.Background()
	if _, err := h.Call(ctx, "gmail.search", "", map[string]any{"query": "is:unread"}); err != nil {
		t.Fatal(err)
	}
	h.Policy = blockArchive{}
	_, err := h.Call(ctx, "gmail.archive", "", map[string]any{"id": "m1"})
	if !errors.Is(err, ErrBlocked) || len(mail.archived) != 0 {
		t.Fatalf("blocked call went through: %v %v", err, mail.archived)
	}
	evs, _ := h.Events.List(ctx, event.Query{Types: []string{ActionEvent}})
	if len(evs) != 2 {
		t.Fatalf("events %d", len(evs))
	}
	var rec ActionRecord
	evs[1].Decode(&rec)
	if rec.Verdict != policy.Block || rec.Rule != "r1" || rec.Risk != "reversible" || evs[1].Actor != "routine:triage#1" {
		t.Fatalf("record %+v", rec)
	}
	if len(h.Calls()) != 2 {
		t.Fatalf("trace calls %d", len(h.Calls()))
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	mail := &fakeMail{}
	h := &Host{Env: env(t, mail), Source: "exploration:e1", DryRun: true}
	res, err := h.Call(context.Background(), "gmail.archive", "", map[string]any{"id": "m1"})
	if err != nil || len(mail.archived) != 0 || res.(map[string]any)["dry_run"] != true {
		t.Fatalf("dry run touched the mailbox: %v %v %v", res, err, mail.archived)
	}
	if _, err := h.Call(context.Background(), "gmail.search", "", map[string]any{}); err != nil {
		t.Fatal("reads must still work in a dry run")
	}
}

func TestJudgmentsCostAndRespectTheBudget(t *testing.T) {
	h := &Host{Env: env(t, &fakeMail{}), Source: "routine:brief#3"}
	ctx := context.Background()
	h.Budget.SetLimit(ctx, 0.02, "owner")
	for range 2 {
		if _, err := h.Judge(ctx, "important", "Is it important?", map[string]string{"id": "m1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.Judge(ctx, "important", "?", nil); !errors.Is(err, budget.ErrOverBudget) {
		t.Fatalf("over budget: %v", err)
	}
	if h.Cost() < 0.019 {
		t.Fatalf("cost %v", h.Cost())
	}
	h.Label("important", "m1", 0.9)
	if h.Judgments()["important"]["m1"] != 0.9 {
		t.Fatal("label lost")
	}
}

func TestUnconnectedCapabilityExplainsItself(t *testing.T) {
	h := &Host{Env: env(t, &fakeMail{}), Source: "routine:x#1"}
	_, err := h.Call(context.Background(), "telegram.send", "", map[string]any{"text": "hi"})
	if err == nil || !strings.Contains(err.Error(), "Connections") {
		t.Fatalf("got %v", err)
	}
}

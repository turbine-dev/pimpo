package host

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/pimpo/internal/budget"
	"github.com/denerFernandes/pimpo/internal/connector"
	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/judge"
	"github.com/denerFernandes/pimpo/internal/policy"
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

type askAll struct{}

func (askAll) Decide(_ context.Context, a policy.Action) policy.Decision {
	if a.Capability == "gmail.archive" {
		return policy.Decision{Verdict: policy.Ask, Reason: "ask before archiving"}
	}
	return policy.Decision{Verdict: policy.Allow}
}

type answer struct {
	always bool
	err    error
	asked  int
}

func (a *answer) Ask(context.Context, policy.Action, string) (bool, error) {
	a.asked++
	return a.always, a.err
}

func TestApprovalGatesTheCall(t *testing.T) {
	mail := &fakeMail{}
	ans := &answer{err: errors.New("the owner said no")}
	remembered := 0
	h := &Host{Env: env(t, mail), Source: "routine:triage#2"}
	h.Policy, h.Approver = askAll{}, ans
	h.Remember = func(context.Context, policy.Action) { remembered++ }
	ctx := context.Background()
	if _, err := h.Call(ctx, "gmail.archive", "", map[string]any{"id": "m1"}); err == nil || len(mail.archived) != 0 {
		t.Fatalf("denied call went through: %v", err)
	}
	ans.err, ans.always = nil, true
	if _, err := h.Call(ctx, "gmail.archive", "", map[string]any{"id": "m1"}); err != nil || len(mail.archived) != 1 || remembered != 1 {
		t.Fatalf("approved call: %v archived=%v remembered=%d", err, mail.archived, remembered)
	}
	// Simulated calls in an exploration never wait for the owner.
	x := &Host{Env: env(t, &fakeMail{}), Source: "exploration:e1", DryRun: true}
	x.Policy, x.Approver = askAll{}, ans
	asked := ans.asked
	if _, err := x.Call(ctx, "gmail.archive", "", map[string]any{"id": "m1"}); err != nil || ans.asked != asked {
		t.Fatalf("exploration asked for a simulated action: %v", err)
	}
}

type reversibleAll struct{}

func (reversibleAll) Decide(context.Context, policy.Action) policy.Decision {
	return policy.Decision{Verdict: policy.Reversible}
}

type trashCan struct{ trashed []string }

func (t *trashCan) Capabilities() []string { return []string{"gmail.trash", "gmail.delete"} }
func (t *trashCan) Call(_ context.Context, name, _ string, args any) (any, error) {
	if name == "gmail.delete" {
		return nil, errors.New("permanent delete must not be called")
	}
	t.trashed = append(t.trashed, args.(map[string]any)["id"].(string))
	return map[string]bool{"ok": true}, nil
}

func TestIrreversibleBecomesReversible(t *testing.T) {
	can := &trashCan{}
	e := env(t, &fakeMail{})
	e.Router.Add(can)
	h := &Host{Env: e, Source: "routine:cleanup#1"}
	h.Policy = reversibleAll{}
	if _, err := h.Call(context.Background(), "gmail.delete", "", map[string]any{"id": "m9"}); err != nil {
		t.Fatal(err)
	}
	if len(can.trashed) != 1 {
		t.Fatal("delete was not turned into trash")
	}
	evs, _ := h.Events.List(context.Background(), event.Query{Types: []string{ActionEvent}})
	var rec ActionRecord
	evs[0].Decode(&rec)
	if rec.Done != "gmail.trash" || rec.Capability != "gmail.delete" {
		t.Fatalf("record %+v", rec)
	}
}

func TestWriteIsBudgetedAndRecorded(t *testing.T) {
	e := env(t, &fakeMail{})
	ctx := context.Background()
	e.Write = func(_ context.Context, instruction string, input any) (string, float64, error) {
		return "Pede a assinatura hoje.", 0.004, nil
	}
	h := &Host{Env: e, Source: "routine:ana#3"}
	text, err := h.Write(ctx, "pedido", "Diga o que o e-mail pede", map[string]any{"subject": "Contrato"})
	if err != nil || text != "Pede a assinatura hoje." || h.Cost() != 0.004 {
		t.Fatalf("%q %v %v", text, err, h.Cost())
	}
	evs, _ := e.Events.List(ctx, event.Query{Types: []string{WriteEvent}})
	if len(evs) != 1 || !strings.Contains(string(evs[0].Data), "Pede a assinatura hoje.") {
		t.Fatalf("%v", evs)
	}
	e.Budget.SetLimit(ctx, 0.01, "test")
	if _, err := (&Host{Env: e, Source: "routine:ana#4"}).Write(ctx, "pedido", "x", nil); err == nil {
		t.Fatal("wrote past the budget")
	}
	e.Write = nil
	if _, err := (&Host{Env: e}).Write(ctx, "pedido", "x", nil); err == nil {
		t.Fatal("wrote with no model")
	}
}

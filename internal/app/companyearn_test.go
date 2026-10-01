package app

import (
	"context"
	"testing"

	"github.com/turbine-dev/pimpo/internal/approval"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
)

func TestAutonomyIsEarnedAndTakenBack(t *testing.T) {
	ctx := context.Background()
	ta, co := clerk(t)
	o, _ := ta.Companies.Org(ctx, co)
	o.EarnAfter = 3
	ta.Companies.Update(ctx, o.Company)
	act := sendAs(co)
	if d := ta.decide(ctx, act); d.Verdict != policy.Ask {
		t.Fatalf("the action should ask first: %+v", d)
	}
	answer := func(ans approval.Answer, err error) { ta.deliveryAnswered(ctx, act, ans, err) }
	answer(approval.Once, nil)
	answer(approval.Deny, approval.ErrDenied)
	answer(approval.Once, nil)
	answer(approval.Run, nil)
	answer(approval.Deny, approval.ErrExpired)
	answer(approval.Once, nil)
	if needs, _ := ta.needs(people.With(ctx, people.OwnerID)); containsKind(needs, "company_autonomy") {
		t.Fatal("suggested before three approvals in a row")
	}
	answer(approval.Always, nil)
	needs, _ := ta.needs(people.With(ctx, people.OwnerID))
	if !containsKind(needs, "company_autonomy") {
		t.Fatalf("no suggestion after three in a row: %+v", needs)
	}
	answer(approval.Once, nil)
	if needs, _ := ta.needs(people.With(ctx, people.OwnerID)); countKind(needs, "company_autonomy") != 1 {
		t.Fatal("suggested twice")
	}

	base := "/api/companies/" + co
	if code, out := ta.do(t, "POST", base+"/members/clara/earn", map[string]any{"capability": act.Capability, "accept": true}); code != 200 {
		t.Fatalf("earn: %d %v", code, out)
	}
	if d := ta.decide(ctx, act); d.Verdict != policy.Allow {
		t.Fatalf("an earned kind still asks: %+v", d)
	}
	o, _ = ta.Companies.Org(ctx, co)
	if m, _ := o.Member("clara"); len(m.Autonomy) == 0 || !m.Autonomy[0].Earned {
		t.Fatalf("autonomy = %+v", m.Autonomy)
	}
	if code, _ := ta.do(t, "POST", base+"/members/clara/earn", map[string]any{"capability": "gmail.delete", "accept": true}); code != 400 {
		t.Fatal("autonomy given for a kind nobody suggested")
	}

	// A no afterwards, here from a request a person still answered,
	// takes it back to approval.
	answer(approval.Deny, approval.ErrDenied)
	if d := ta.decide(ctx, act); d.Verdict != policy.Ask {
		t.Fatalf("a kind taken back is still done alone: %+v", d)
	}
	streaks, _ := ta.Companies.Streaks(ctx, co)
	if len(streaks) != 1 || streaks[0].Count != 0 || streaks[0].Earned {
		t.Fatalf("streaks = %+v", streaks)
	}

	o, _ = ta.Companies.Org(ctx, co)
	o.EarnOff = true
	ta.Companies.Update(ctx, o.Company)
	for range 5 {
		answer(approval.Once, nil)
	}
	if needs, _ := ta.needs(people.With(ctx, people.OwnerID)); containsKind(needs, "company_autonomy") {
		t.Fatal("suggested with suggestions off")
	}
	o.EarnOff, o.EarnAfter = false, 101
	if _, err := ta.Companies.Update(ctx, o.Company); err == nil {
		t.Fatal("101 approvals in a row were accepted")
	}
}

func countKind(needs []need, kind string) int {
	n := 0
	for _, x := range needs {
		if x.Kind == kind {
			n++
		}
	}
	return n
}

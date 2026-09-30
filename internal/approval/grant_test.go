package approval

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/policy"
)

func send(to, subject string) policy.Action {
	return policy.Action{Capability: "gmail.send", Risk: 3, Source: "routine:brief#7", Person: "owner",
		Args: map[string]any{"to": to, "subject": subject, "body": "Hoje: " + subject}}
}

func grantFor(t *testing.T, a policy.Action, person string, limit *float64) Grant {
	t.Helper()
	g, err := NewGrant(Request{Action: a}, person, 3, limit)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// A daily email with new words is the same operation; a new recipient,
// routine, version or person is not.
func TestGrantMatchesOnlyTheSameOperation(t *testing.T) {
	g := grantFor(t, send("ana@x.com, Bia@x.com", "segunda"), "owner", nil)
	if !g.Allows(send(" bia@x.com;ANA@x.com", "terça"), 3) {
		t.Fatal("the same recipients with a new subject and body should match")
	}
	for name, a := range map[string]policy.Action{
		"another recipient": send("ana@x.com, eve@evil.com", "terça"),
		"fewer recipients":  send("ana@x.com", "terça"),
		"another routine":   func() policy.Action { a := send("ana@x.com, bia@x.com", "x"); a.Source = "routine:other#8"; return a }(),
		"another person":    func() policy.Action { a := send("ana@x.com, bia@x.com", "x"); a.Person = "ana"; return a }(),
		"another action":    func() policy.Action { a := send("ana@x.com, bia@x.com", "x"); a.Capability = "gmail.draft"; return a }(),
		"an extra argument": func() policy.Action {
			a := send("ana@x.com, bia@x.com", "x")
			a.Args.(map[string]any)["cc"] = "eve@evil.com"
			return a
		}(),
	} {
		if g.Allows(a, 3) {
			t.Errorf("%s matched", name)
		}
	}
	if g.Allows(send("ana@x.com, bia@x.com", "x"), 4) {
		t.Error("a new version of the routine matched")
	}
}

func TestGrantHostsAndAmounts(t *testing.T) {
	pay := func(url string, amount float64) policy.Action {
		return policy.Action{Capability: "shop.pay", Risk: 3, Source: "routine:bills#1", Scope: "Pay.example",
			Args: map[string]any{"url": url, "amount": amount, "note": "conta de luz"}}
	}
	g := grantFor(t, pay("https://pay.example/invoice/1", 80), "owner", nil)
	if !g.Allows(pay("https://PAY.example/invoice/2", 75.5), 3) {
		t.Fatal("the same host with a smaller amount should match")
	}
	if g.Allows(pay("https://pay.example/invoice/2", 80.01), 3) || g.Allows(pay("https://pay.example/x", -5), 3) {
		t.Fatal("an amount above the limit, or a negative one, matched")
	}
	if g.Allows(pay("https://evil.example/invoice/2", 10), 3) {
		t.Fatal("another host matched")
	}
	limit := 120.0
	if g := grantFor(t, pay("https://pay.example/1", 80), "owner", &limit); !g.Allows(pay("https://pay.example/2", 119), 3) {
		t.Fatal("the person's own limit was not used")
	}
	low := 50.0
	if _, err := NewGrant(Request{Action: pay("https://pay.example/1", 80)}, "owner", 3, &low); err == nil {
		t.Fatal("a limit below the approved amount was accepted")
	}
}

func TestGrantsBelongToWhoAnswersAndSkipAlwaysAsk(t *testing.T) {
	if _, err := NewGrant(Request{Action: send("a@x.com", "s")}, "ana", 3, nil); err == nil {
		t.Fatal("someone else approved the owner's routine for it")
	}
	wa := policy.Action{Capability: "whatsapp.send_to", Risk: 3, Source: "routine:r#1", Args: map[string]any{"to": "+5511"}}
	if _, err := NewGrant(Request{Action: wa}, "owner", 3, nil); err == nil {
		t.Fatal("whatsapp.send_to got a grant")
	}
	// Even a grant written by hand never lets it through.
	forged := Grant{Person: "owner", Routine: "r", Version: 3, Capability: "whatsapp.send_to", Match: OperationOf("whatsapp.send_to", wa.Args).Fields}
	if forged.Allows(wa, 3) {
		t.Fatal("a grant let whatsapp.send_to through")
	}
	exp := send("a@x.com", "s")
	exp.Source = "exploration:e1"
	if Grantable(exp, "owner") {
		t.Fatal("an exploration may be approved for a routine")
	}
}

func TestGrantStoreIsPerPerson(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	t.Cleanup(func() { ev.Close() })
	ctx := context.Background()
	s := &Grants{Events: ev}
	g := grantFor(t, send("a@x.com", "s"), "owner", nil)
	s.Add(ctx, g, "human:owner")
	// Answering again for the same operation replaces, not piles up.
	s.Add(ctx, grantFor(t, send("a@x.com", "t"), "owner", nil), "human:owner")
	mine := s.Mine(ctx, "owner")
	if len(mine) != 1 || len(s.Mine(ctx, "ana")) != 0 {
		t.Fatalf("owner %v", mine)
	}
	if err := s.Revoke(ctx, mine[0].ID, "ana", "human:ana"); !errors.Is(err, ErrNoGrant) {
		t.Fatalf("someone else revoked it: %v", err)
	}
	if _, ok := s.Find(ctx, send("a@x.com", "u"), 3); !ok {
		t.Fatal("not found")
	}
	if err := s.Revoke(ctx, mine[0].ID, "owner", "human:owner"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Find(ctx, send("a@x.com", "u"), 3); ok {
		t.Fatal("a revoked grant still applies")
	}
}

// The choice is offered for a routine's own actions only, and answering
// with it lets the waiting action go on.
func TestRoutineChoiceIsOfferedWhereItApplies(t *testing.T) {
	m, n := manager(t, time.Minute)
	offered := func(a policy.Action) bool {
		done := make(chan error, 1)
		go func() { _, err := m.Ask(context.Background(), a, "r"); done <- err }()
		<-n.got
		note := n.list[len(n.list)-1]
		m.Resolve(context.Background(), idOf(note), Routine, "human:owner")
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(note.Actions, func(x explore.Action) bool { return strings.HasPrefix(x.Data, "grant:") })
	}
	if !offered(send("a@x.com", "s")) {
		t.Fatal("not offered for a routine")
	}
	wa := policy.Action{Capability: "whatsapp.send_to", Risk: 3, Source: "routine:r#1"}
	exp := send("a@x.com", "s")
	exp.Source = "exploration:e1"
	if offered(wa) || offered(exp) {
		t.Fatal("offered where it cannot apply")
	}
}

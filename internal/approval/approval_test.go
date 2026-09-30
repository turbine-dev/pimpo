package approval

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/pause"
	"github.com/turbine-dev/pimpo/internal/policy"
)

type notes struct {
	mu   sync.Mutex
	list []explore.Notice
	got  chan struct{}
}

func (n *notes) Notify(_ context.Context, x explore.Notice) error {
	n.mu.Lock()
	n.list = append(n.list, x)
	n.mu.Unlock()
	n.got <- struct{}{}
	return nil
}

func manager(t *testing.T, timeout time.Duration) (*Manager, *notes) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	t.Cleanup(func() { ev.Close() })
	n := &notes{got: make(chan struct{}, 4)}
	return &Manager{Events: ev, Notify: n, Timeout: timeout, Describe: func(_ context.Context, a policy.Action) string { return "Enviar e-mail para cliente@acme.com" }}, n
}

func idOf(n explore.Notice) string { return strings.TrimPrefix(n.Actions[0].Data, "approve:") }

func TestOwnerApproves(t *testing.T) {
	m, n := manager(t, time.Minute)
	done := make(chan error, 1)
	go func() {
		_, err := m.Ask(context.Background(), policy.Action{Capability: "gmail.send", Risk: 3, Source: "routine:x#1"}, "Sempre me pergunte")
		done <- err
	}()
	<-n.got
	if len(m.Open()) != 1 || !strings.Contains(n.list[0].Text, "cliente@acme.com") {
		t.Fatalf("open %+v notice %+v", m.Open(), n.list)
	}
	if !m.Resolve(context.Background(), idOf(n.list[0]), Once, "human:owner") {
		t.Fatal("resolve failed")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if m.Resolve(context.Background(), idOf(n.list[0]), Once, "human:owner") {
		t.Fatal("resolved twice")
	}
}

func TestDenyAndExpiry(t *testing.T) {
	m, n := manager(t, time.Minute)
	go func() {
		<-n.got
		m.Resolve(context.Background(), idOf(n.list[0]), Deny, "human:owner")
	}()
	if _, err := m.Ask(context.Background(), policy.Action{Capability: "gmail.delete"}, "r"); !errors.Is(err, ErrDenied) {
		t.Fatalf("deny: %v", err)
	}
	quick, qn := manager(t, 20*time.Millisecond)
	go func() { <-qn.got }()
	if _, err := quick.Ask(context.Background(), policy.Action{Capability: "gmail.delete"}, "r"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expiry: %v", err)
	}
	if len(quick.Open()) != 0 {
		t.Fatal("expired request still open")
	}
}

func TestRunAnswerCoversTheRestOfTheRun(t *testing.T) {
	m, n := manager(t, time.Minute)
	act := policy.Action{Capability: "gmail.archive", Risk: 2, Source: "routine:triage#7"}
	go func() {
		<-n.got
		m.Resolve(context.Background(), idOf(n.list[0]), Run, "human:owner")
	}()
	for i := 0; i < 200; i++ {
		if _, err := m.Ask(context.Background(), act, "ask before archiving"); err != nil {
			t.Fatal(err)
		}
	}
	if len(n.list) != 1 {
		t.Fatalf("asked %d times for one run", len(n.list))
	}
	next := policy.Action{Capability: "gmail.archive", Risk: 2, Source: "routine:triage#8"}
	go func() {
		<-n.got
		m.Resolve(context.Background(), idOf(n.list[1]), Deny, "human:owner")
	}()
	if _, err := m.Ask(context.Background(), next, "r"); err == nil {
		t.Fatal("a run answer leaked into the next run")
	}
}

func TestSuggestsAlwaysAfterThreeApprovals(t *testing.T) {
	m, n := manager(t, time.Minute)
	for i := 0; i < 4; i++ {
		act := policy.Action{Capability: "gmail.send", Risk: 3, Source: fmt.Sprintf("routine:followup#%d", i)}
		go func() {
			<-n.got
			n.mu.Lock()
			last := n.list[len(n.list)-1]
			n.mu.Unlock()
			m.Resolve(context.Background(), idOf(last), Once, "human:owner")
		}()
		m.Ask(context.Background(), act, "r")
	}
	if !strings.Contains(n.list[3].Text, "3 vezes") || strings.Contains(n.list[2].Text, "3 vezes") {
		t.Fatalf("suggestion missing or early:\n%s\n%s", n.list[2].Text, n.list[3].Text)
	}
}

// A scheduled run has 15 minutes of its own; waiting for the owner does
// not use them up, and a request whose run ended cannot be answered.
func TestWaitingForTheOwnerStopsTheRunClock(t *testing.T) {
	m, n := manager(t, time.Minute)
	ctx, cancel := pause.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	go func() {
		<-n.got
		time.Sleep(150 * time.Millisecond) // longer than the run's own limit
		m.Resolve(context.Background(), idOf(n.list[0]), Once, "human:owner")
	}()
	if _, err := m.Ask(ctx, policy.Action{Capability: "gmail.send", Risk: 3}, "r"); err != nil {
		t.Fatalf("the run ran out of time while waiting: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("the run's clock kept counting")
	}

	m2, n2 := manager(t, time.Minute)
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() { <-n2.got; cancel2() }()
	if _, err := m2.Ask(ctx2, policy.Action{Capability: "gmail.send", Risk: 3}, "r"); err == nil {
		t.Fatal("a cancelled run was approved")
	}
	if len(m2.Open()) != 0 || m2.Resolve(context.Background(), idOf(n2.list[0]), Once, "human:owner") {
		t.Fatal("the request of an ended run can still be answered")
	}
}

// Only the one responsible answers a request: a member's own requests are
// theirs, and the owner cannot answer them.
func TestOnlyTheResponsibleAnswers(t *testing.T) {
	m, n := manager(t, time.Minute)
	m.Responsible = func(_ context.Context, person string) (string, string) { return person, person }
	go m.Ask(context.Background(), policy.Action{Capability: "gmail.send", Risk: 3, Source: "routine:x#1", Person: "ana"}, "")
	<-n.got
	id := idOf(n.list[0])
	if m.MayAnswer(id, "owner") || m.MayAnswer(id, "") || m.MayAnswer(id, "bia") || !m.MayAnswer(id, "ana") {
		t.Fatal("someone other than ana may answer her request")
	}
	go m.Ask(context.Background(), policy.Action{Capability: "gmail.send", Risk: 3, Source: "routine:y#1"}, "")
	<-n.got
	own := idOf(n.list[1])
	if !m.MayAnswer(own, "owner") || m.MayAnswer(own, "ana") {
		t.Fatal("the owner's own request")
	}
}

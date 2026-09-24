package approval

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/explore"
	"github.com/denerFernandes/vigia/internal/policy"
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
	return &Manager{Events: ev, Notify: n, Timeout: timeout, Describe: func(a policy.Action) string { return "Enviar e-mail para cliente@acme.com" }}, n
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

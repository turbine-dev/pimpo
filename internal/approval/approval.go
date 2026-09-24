// Package approval holds an action until the owner answers. The routine
// waits at that step, as the owner would expect from a person asking.
package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/explore"
	"github.com/denerFernandes/vigia/internal/policy"
)

const (
	EventRequested = "approval.requested"
	EventResolved  = "approval.resolved"
)

type Answer string

const (
	Once   Answer = "once"
	Always Answer = "always"
	Deny   Answer = "deny"
)

var ErrDenied = errors.New("the owner said no")
var ErrExpired = errors.New("no answer in time")

type Request struct {
	ID      string        `json:"id"`
	Action  policy.Action `json:"action"`
	Text    string        `json:"text"`
	Reason  string        `json:"reason"`
	Created time.Time     `json:"created"`
}

type Manager struct {
	Events *event.Store
	Notify explore.Notifier
	// Timeout is how long a run waits; the Jev-backed decision was 30 minutes.
	Timeout time.Duration
	// Describe turns an action into the sentence shown to the owner.
	Describe func(policy.Action) string

	mu      sync.Mutex
	pending map[string]chan Answer
	open    map[string]Request
}

func newID() string {
	b := make([]byte, 5)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Ask blocks until the owner answers, the timeout passes or ctx ends.
func (m *Manager) Ask(ctx context.Context, a policy.Action, reason string) (Answer, error) {
	id := newID()
	text := fmt.Sprintf("%s (%s)", a.Capability, a.Source)
	if m.Describe != nil {
		text = m.Describe(a)
	}
	req := Request{ID: id, Action: a, Text: text, Reason: reason, Created: time.Now()}
	ch := make(chan Answer, 1)
	m.mu.Lock()
	if m.pending == nil {
		m.pending, m.open = map[string]chan Answer{}, map[string]Request{}
	}
	m.pending[id], m.open[id] = ch, req
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.pending, id)
		delete(m.open, id)
		m.mu.Unlock()
	}()
	m.Events.Append(ctx, EventRequested, a.Source, req)
	icon := "🟠"
	if a.Risk >= 3 {
		icon = "🔴"
	}
	m.Notify.Notify(ctx, explore.Notice{
		Text:    fmt.Sprintf("%s Posso fazer isto?\n%s\n\nRegra: %s", icon, text, reason),
		Actions: []explore.Action{{Label: "Permitir", Data: "approve:" + id}, {Label: "Sempre", Data: "always:" + id}, {Label: "Negar", Data: "deny:" + id}},
	})
	timeout := m.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case ans := <-ch:
		if ans == Deny {
			return ans, ErrDenied
		}
		return ans, nil
	case <-timer.C:
		m.Events.Append(context.WithoutCancel(ctx), EventResolved, "system", map[string]string{"id": id, "answer": "expired"})
		return Deny, ErrExpired
	case <-ctx.Done():
		return Deny, ctx.Err()
	}
}

// Resolve records the owner's answer. It reports false if the request is
// no longer waiting (answered, expired, or from before a restart).
func (m *Manager) Resolve(ctx context.Context, id string, ans Answer, actor string) bool {
	m.mu.Lock()
	ch, ok := m.pending[id]
	if ok {
		delete(m.pending, id)
	}
	m.mu.Unlock()
	if !ok {
		return false
	}
	m.Events.Append(ctx, EventResolved, actor, map[string]string{"id": id, "answer": string(ans)})
	ch <- ans
	return true
}

// Open lists the requests still waiting, oldest first.
func (m *Manager) Open() []Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Request, 0, len(m.open))
	for id, r := range m.open {
		if _, waiting := m.pending[id]; waiting {
			out = append(out, r)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Created.Before(out[j-1].Created); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

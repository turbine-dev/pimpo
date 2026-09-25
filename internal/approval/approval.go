// Package approval holds an action until the owner answers. The routine
// waits at that step, as the owner would expect from a person asking.
package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/zodim/internal/event"
	"github.com/denerFernandes/zodim/internal/explore"
	"github.com/denerFernandes/zodim/internal/policy"
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
	// Run allows the same capability for the rest of this run, so a loop
	// over 200 emails asks once, not 200 times.
	Run Answer = "run"
)

var ErrDenied = errors.New("the owner said no")
var ErrExpired = errors.New("no answer in time")

type Request struct {
	ID     string        `json:"id"`
	Action policy.Action `json:"action"`
	Text   string        `json:"text"`
	Reason string        `json:"reason"`
	// Responsible is the person who may answer; empty is the owner.
	Responsible string    `json:"responsible,omitempty"`
	Created     time.Time `json:"created"`
}

type Manager struct {
	Events *event.Store
	Notify explore.Notifier
	// Timeout is how long a run waits; the Jev-backed decision was 30 minutes.
	Timeout time.Duration
	// Describe turns an action into the sentence shown to the owner.
	Describe func(policy.Action) string
	// Responsible names who answers for a person's requests; nil means
	// the owner answers everything.
	Responsible func(ctx context.Context, person string) (id, name string)

	mu      sync.Mutex
	pending map[string]chan Answer
	open    map[string]Request
	// runs holds "allow for the rest of this run" answers by run and capability.
	runs map[string]bool
	// onces counts one-off approvals per routine and capability, to suggest
	// making them permanent.
	onces map[string]int
}

func runKey(a policy.Action) string { return a.Source + "|" + a.Capability }

func routineKey(a policy.Action) string {
	src := a.Source
	if i := strings.IndexByte(src, '#'); i >= 0 {
		src = src[:i]
	}
	return src + "|" + a.Capability
}

func newID() string {
	b := make([]byte, 5)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Ask blocks until the owner answers, the timeout passes or ctx ends.
func (m *Manager) Ask(ctx context.Context, a policy.Action, reason string) (Answer, error) {
	m.mu.Lock()
	if m.runs[runKey(a)] {
		m.mu.Unlock()
		return Run, nil
	}
	suggest := m.onces[routineKey(a)] >= 3
	m.mu.Unlock()
	id := newID()
	text := fmt.Sprintf("%s (%s)", a.Capability, a.Source)
	if m.Describe != nil {
		text = m.Describe(a)
	}
	req := Request{ID: id, Action: a, Text: text, Reason: reason, Created: time.Now()}
	asker := ""
	if m.Responsible != nil && a.Person != "" && a.Person != "owner" {
		req.Responsible, asker = m.Responsible(ctx, a.Person)
		if req.Responsible == "owner" {
			req.Responsible = ""
		}
	}
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
	body := fmt.Sprintf("%s Posso fazer isto?\n%s\n\nRegra: %s", icon, text, reason)
	if asker != "" {
		body = fmt.Sprintf("%s Pedido de %s. Posso fazer isto?\n%s\n\nRegra: %s", icon, asker, text, reason)
	}
	if suggest {
		body += "\n\nVocê já permitiu isto 3 vezes. Quer que eu não pergunte mais?"
	}
	m.Notify.Notify(ctx, explore.Notice{
		Text: body,
		Actions: []explore.Action{{Label: "Permitir", Data: "approve:" + id}, {Label: "Todos desta vez", Data: "batch:" + id},
			{Label: "Sempre", Data: "always:" + id}, {Label: "Negar", Data: "deny:" + id}},
		To: req.Responsible,
	})
	timeout := m.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case ans := <-ch:
		m.mu.Lock()
		if m.runs == nil {
			m.runs, m.onces = map[string]bool{}, map[string]int{}
		}
		switch ans {
		case Run:
			m.runs[runKey(a)] = true
		case Once:
			m.onces[routineKey(a)]++
		}
		m.mu.Unlock()
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

// MayAnswer reports whether a person may answer a waiting request: the
// owner always, anyone else only for requests they are responsible for.
func (m *Manager) MayAnswer(id, person string) bool {
	if person == "" || person == "owner" {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.open[id]
	return ok && r.Responsible == person
}

// Suggest reports whether the owner keeps approving this kind of action.
func (m *Manager) Suggest(a policy.Action) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.onces[routineKey(a)] >= 3
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

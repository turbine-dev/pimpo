package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
)

// Routine allows this exact operation for this routine from now on: the
// same capability with the same recipients, hosts and other identifying
// arguments, and amounts up to the approved one. Anything else asks again.
const Routine Answer = "routine"

const (
	EventGranted = "approval.granted"
	EventRevoked = "approval.revoked"
)

var ErrNoGrant = errors.New("no such approval")

// Grant is one "approve for this routine" answer. It belongs to the person
// who answered, applies only to that person's runs of that routine at that
// code version, and only to calls whose operation matches.
type Grant struct {
	ID         string `json:"id"`
	Person     string `json:"person"`
	Routine    string `json:"routine"`
	Version    int    `json:"version"`
	Capability string `json:"capability"`
	Scope      string `json:"scope,omitempty"`
	// Match holds the identifying arguments in canonical form.
	Match map[string]string `json:"match"`
	// Limits are the most each amount argument may be.
	Limits  map[string]float64 `json:"limits,omitempty"`
	Created time.Time          `json:"created"`
}

// Operation is what identifies a call: its identifying arguments and its
// amounts. Free text (bodies, subjects, messages) is left out, so a daily
// email with new words is the same operation while a new recipient is not.
type Operation struct {
	Fields  map[string]string
	Amounts map[string]float64
}

// freeText are arguments that change from one run to the next without
// changing what the action does or to whom.
var freeText = []string{"body", "html", "subject", "text", "message", "content", "title", "description", "note", "notes", "caption"}

// freeTextFor adds, per capability, arguments that are free text there.
var freeTextFor = map[string][]string{
	"reminder.set":  {"at", "in"},
	"ask.owner":     {"question", "options", "key"},
	"sheets.append": {"values"},
	"todoist.add":   {"due", "due_string"},
}

// recipients are compared as a set of addresses, whatever their order or case.
var recipients = []string{"to", "cc", "bcc", "recipients", "email", "phone", "number"}

// amounts may grow up to the approved limit without asking again.
var amounts = []string{"amount", "total", "price", "sum"}

// OperationOf reduces a call's arguments to what identifies it.
func OperationOf(capability string, args any) Operation {
	op := Operation{Fields: map[string]string{}, Amounts: map[string]float64{}}
	b, _ := json.Marshal(args)
	var v any
	json.Unmarshal(b, &v)
	m, ok := v.(map[string]any)
	if !ok {
		if v != nil {
			op.Fields[""] = canonical("", v)
		}
		return op
	}
	for k, val := range m {
		key := strings.ToLower(k)
		if slices.Contains(freeText, key) || slices.Contains(freeTextFor[capability], key) {
			continue
		}
		if n, isNum := val.(float64); isNum && slices.Contains(amounts, key) {
			op.Amounts[k] = n
			continue
		}
		op.Fields[k] = canonical(key, val)
	}
	return op
}

func canonical(key string, v any) string {
	if slices.Contains(recipients, key) {
		var list []string
		switch x := v.(type) {
		case string:
			list = strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == ';' })
		case []any:
			for _, e := range x {
				list = append(list, fmt.Sprint(e))
			}
		}
		if list != nil {
			for i := range list {
				list[i] = strings.ToLower(strings.TrimSpace(list[i]))
			}
			slices.Sort(list)
			return strings.Join(slices.Compact(list), ",")
		}
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		// A web address is the same operation on the same host.
		if key == "url" {
			if u, err := url.Parse(s); err == nil && u.Host != "" {
				return strings.ToLower(u.Hostname())
			}
		}
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// RoutineOf is the routine id of a "routine:<id>#<run>" source, or "".
func RoutineOf(source string) string {
	id, ok := strings.CutPrefix(source, "routine:")
	if !ok {
		return ""
	}
	id, _, _ = strings.Cut(id, "#")
	return id
}

// Grantable reports whether "approve for this routine" may answer this
// request for person: a routine's action, answered by the person it runs
// for, and not one of those that always ask.
func Grantable(a policy.Action, person string) bool {
	return RoutineOf(a.Source) != "" && !policy.AlwaysAsks(a.Capability) && a.Role != "guest" &&
		people.Norm(a.Person) == people.Norm(person)
}

// NewGrant is the grant for a request answered by person, for the
// routine's code at version. limit, when set, replaces the approved amount
// as the most later calls may use; it may not be below it.
func NewGrant(req Request, person string, version int, limit *float64) (Grant, error) {
	a := req.Action
	if !Grantable(a, person) {
		return Grant{}, errors.New("only the person a routine works for can approve it for that routine, and not this action")
	}
	op := OperationOf(a.Capability, a.Args)
	g := Grant{ID: newID(), Person: people.Norm(person), Routine: RoutineOf(a.Source), Version: version, Capability: a.Capability,
		Scope: strings.ToLower(a.Scope), Match: op.Fields, Created: time.Now()}
	if len(op.Amounts) > 0 {
		g.Limits = map[string]float64{}
		for k, n := range op.Amounts {
			g.Limits[k] = n
			if limit != nil {
				if *limit < n {
					return Grant{}, fmt.Errorf("the limit %v is below the amount being approved (%v)", *limit, n)
				}
				g.Limits[k] = *limit
			}
		}
	}
	return g, nil
}

// Allows reports whether the grant covers an action of a run at version.
func (g Grant) Allows(a policy.Action, version int) bool {
	if g.Person != people.Norm(a.Person) || g.Routine != RoutineOf(a.Source) || g.Version != version ||
		g.Capability != a.Capability || g.Scope != strings.ToLower(a.Scope) || a.Role == "guest" || policy.AlwaysAsks(a.Capability) {
		return false
	}
	op := OperationOf(a.Capability, a.Args)
	if !maps.Equal(g.Match, op.Fields) || len(g.Limits) != len(op.Amounts) {
		return false
	}
	for k, n := range op.Amounts {
		limit, ok := g.Limits[k]
		if !ok || n > limit || (limit >= 0 && n < 0) {
			return false
		}
	}
	return true
}

// same reports whether two grants cover the same operation, so a second
// answer replaces the first instead of piling up.
func (g Grant) same(o Grant) bool {
	return g.Person == o.Person && g.Routine == o.Routine && g.Capability == o.Capability && g.Scope == o.Scope && maps.Equal(g.Match, o.Match)
}

// Grants keeps the "approve for this routine" answers in the event store,
// so a change is logged and survives restarts.
type Grants struct {
	Events *event.Store
	mu     sync.Mutex
}

const grantsKey = "approval.grants"

func (s *Grants) all(ctx context.Context) []Grant {
	raw, _ := s.Events.Get(ctx, grantsKey)
	var list []Grant
	json.Unmarshal([]byte(raw), &list)
	return list
}

func (s *Grants) save(ctx context.Context, list []Grant) error {
	b, _ := json.Marshal(list)
	return s.Events.Put(ctx, grantsKey, string(b))
}

// Add saves a grant, replacing an earlier one for the same operation.
func (s *Grants) Add(ctx context.Context, g Grant, actor string) error {
	s.mu.Lock()
	list := slices.DeleteFunc(s.all(ctx), g.same)
	err := s.save(ctx, append(list, g))
	s.mu.Unlock()
	if err != nil {
		return err
	}
	_, err = s.Events.Append(ctx, EventGranted, actor, g)
	return err
}

// Mine lists one person's grants, oldest first.
func (s *Grants) Mine(ctx context.Context, person string) []Grant {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Grant{}
	for _, g := range s.all(ctx) {
		if g.Person == people.Norm(person) {
			out = append(out, g)
		}
	}
	return out
}

// Revoke removes one of person's grants; anyone else's does not exist for them.
func (s *Grants) Revoke(ctx context.Context, id, person, actor string) error {
	s.mu.Lock()
	list := s.all(ctx)
	i := slices.IndexFunc(list, func(g Grant) bool { return g.ID == id && g.Person == people.Norm(person) })
	if i < 0 {
		s.mu.Unlock()
		return ErrNoGrant
	}
	g := list[i]
	err := s.save(ctx, slices.Delete(list, i, i+1))
	s.mu.Unlock()
	if err != nil {
		return err
	}
	_, err = s.Events.Append(ctx, EventRevoked, actor, map[string]any{"id": g.ID, "person": g.Person, "routine": g.Routine, "capability": g.Capability})
	return err
}

// Prune drops the grants keep rejects, such as those of an older version
// of their routine; they would never apply again.
func (s *Grants) Prune(ctx context.Context, keep func(Grant) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.all(ctx)
	next := slices.DeleteFunc(slices.Clone(list), func(g Grant) bool { return !keep(g) })
	if len(next) != len(list) {
		s.save(ctx, next)
	}
}

// Find is the grant that allows an action of a run at version, if any.
func (s *Grants) Find(ctx context.Context, a policy.Action, version int) (Grant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range s.all(ctx) {
		if g.Allows(a, version) {
			return g, true
		}
	}
	return Grant{}, false
}

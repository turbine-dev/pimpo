package company

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/policy"
)

// Layers, from the most specific. Context is given in the opposite order,
// the company first; rules are decided by the most specific layer that has
// one for the action.
const (
	ScopeMember     = "member"
	ScopeRole       = "role"
	ScopeDepartment = "department"
	ScopeCompany    = "company"
)

var scopes = []string{ScopeMember, ScopeRole, ScopeDepartment, ScopeCompany}

// keptVersions is how many earlier texts a context keeps.
const keptVersions = 10

// A Context is something members need to know to work well, written by a
// person: who the company is, its products, its voice, its policies.
type Context struct {
	ID       string    `json:"id"`
	Scope    string    `json:"scope"`
	Of       string    `json:"of,omitempty"`
	Title    string    `json:"title"`
	Body     string    `json:"body"`
	Version  int       `json:"version"`
	Updated  time.Time `json:"updated"`
	Previous []string  `json:"previous,omitempty"`
}

// A Rule is one of the house's kind of rules with a scope in the company.
// A rule that allows what a broader layer forbids is an Exception, and
// Overrides names the rules it goes against.
type Rule struct {
	ID        string         `json:"id"`
	Scope     string         `json:"scope"`
	Of        string         `json:"of,omitempty"`
	Text      string         `json:"text"`
	When      policy.When    `json:"when"`
	Then      policy.Verdict `json:"then"`
	Off       bool           `json:"off,omitempty"`
	Exception bool           `json:"exception,omitempty"`
	Overrides []string       `json:"overrides,omitempty"`
}

// ErrException is a rule that goes against a broader one without saying
// it is an exception.
var ErrException = errors.New("this rule allows what a broader rule forbids")

func (o Org) scopeExists(scope, of string) bool {
	switch scope {
	case ScopeCompany:
		return of == ""
	case ScopeDepartment:
		_, ok := o.Department(of)
		return ok
	case ScopeRole:
		_, ok := o.Role(of)
		return ok
	case ScopeMember:
		_, ok := o.Member(of)
		return ok
	}
	return false
}

func (o Org) checkLayers() error {
	seen := map[string]bool{}
	for _, c := range o.Contexts {
		switch {
		case !ValidID(c.ID) || seen[c.ID]:
			return fmt.Errorf("context %q needs a unique id", c.ID)
		case !o.scopeExists(c.Scope, c.Of):
			return fmt.Errorf("context %q is for a %s that does not exist", c.Title, c.Scope)
		case strings.TrimSpace(c.Title) == "":
			return fmt.Errorf("context %q needs a title", c.ID)
		}
		seen[c.ID] = true
	}
	seen = map[string]bool{}
	for _, r := range o.Rules {
		switch {
		case !ValidID(r.ID) || seen[r.ID]:
			return fmt.Errorf("rule %q needs a unique id", r.ID)
		case !o.scopeExists(r.Scope, r.Of):
			return fmt.Errorf("rule %q is for a %s that does not exist", r.Text, r.Scope)
		case len(r.When.People) > 0 || len(r.When.Roles) > 0:
			return fmt.Errorf("rule %q: a company's rules are about its members, not the house's people", r.Text)
		}
		if err := (policy.Rule{ID: r.ID, Text: r.Text, When: r.When, Then: r.Then}).Validate(); err != nil {
			return fmt.Errorf("rule %q: %w", r.Text, err)
		}
		seen[r.ID] = true
	}
	return nil
}

// layers are the scopes that apply to a member, the most specific first.
func (o Org) layers(m Member) [][2]string {
	out := [][2]string{{ScopeMember, m.ID}}
	if m.Role != "" {
		out = append(out, [2]string{ScopeRole, m.Role})
	}
	if m.Department != "" {
		out = append(out, [2]string{ScopeDepartment, m.Department})
	}
	return append(out, [2]string{ScopeCompany, ""})
}

// Decide is the company's verdict on an action of one of its members: the
// most specific layer with a rule for it decides, the strictest of its
// rules if several match. False when no rule of the company covers it.
func (o Org) Decide(member string, a policy.Action) (policy.Decision, bool) {
	m, ok := o.Member(member)
	if !ok {
		return policy.Decision{}, false
	}
	for _, l := range o.layers(m) {
		var hit *Rule
		for i, r := range o.Rules {
			if r.Scope != l[0] || r.Of != l[1] || r.Off || !r.When.Matches(a) {
				continue
			}
			if hit == nil || policy.Stricter(hit.Then, r.Then) != hit.Then {
				hit = &o.Rules[i]
			}
		}
		if hit != nil {
			return policy.Decision{Verdict: hit.Then, Reason: hit.Text, Rule: "company:" + hit.ID}, true
		}
	}
	return policy.Decision{}, false
}

// Overridden are the broader rules r would let members get around: rules
// in a layer above r's, for some of the same members and capabilities,
// that are stricter than r.
func (o Org) Overridden(r Rule) []string {
	order := slices.Index(scopes, r.Scope)
	var out []string
	for _, b := range o.Rules {
		if b.ID == r.ID || b.Off || slices.Index(scopes, b.Scope) <= order {
			continue
		}
		if policy.Stricter(r.Then, b.Then) == r.Then || !overlaps(r.When, b.When) || !o.share(r, b) {
			continue
		}
		out = append(out, b.ID)
	}
	return out
}

// share says whether some member falls under both rules' scopes. A rule
// whose scope has no members yet is assumed to share them, so an
// exception is never missed for being early.
func (o Org) share(r, b Rule) bool {
	if b.Scope == ScopeCompany {
		return true
	}
	under := func(x Rule, m Member) bool {
		switch x.Scope {
		case ScopeMember:
			return m.ID == x.Of
		case ScopeRole:
			return m.Role == x.Of
		case ScopeDepartment:
			return m.Department == x.Of
		}
		return true
	}
	any := false
	for _, m := range o.Members {
		if under(r, m) {
			any = true
			if under(b, m) {
				return true
			}
		}
	}
	return !any
}

// overlaps says whether one action could meet both conditions, judged by
// capabilities and risk only, which errs towards seeing an overlap.
func overlaps(a, b policy.When) bool {
	if len(a.Capabilities) > 0 && len(b.Capabilities) > 0 && !slices.ContainsFunc(a.Capabilities, func(c string) bool { return slices.Contains(b.Capabilities, c) }) {
		return false
	}
	return true
}

// Brief is what a member is told about the company and its job, the
// broadest layer first.
func (o Org) Brief(member string) string {
	m, ok := o.Member(member)
	if !ok {
		return ""
	}
	var b strings.Builder
	section := func(title, body string) {
		if body = strings.TrimSpace(body); body != "" {
			fmt.Fprintf(&b, "## %s\n\n%s\n\n", title, body)
		}
	}
	about := o.Name
	if o.Industry != "" {
		about += " (" + o.Industry + ")"
	}
	section("The company", about+"\n\n"+o.Mission)
	layers := o.layers(m)
	slices.Reverse(layers)
	for _, l := range layers {
		if l[0] == ScopeRole {
			if r, ok := o.Role(l[1]); ok {
				job := r.Function
				if len(r.Responsibilities) > 0 {
					job += "\n\nResponsibilities:\n- " + strings.Join(r.Responsibilities, "\n- ")
				}
				if len(r.Deliverables) > 0 {
					job += "\n\nDeliverables:\n- " + strings.Join(r.Deliverables, "\n- ")
				}
				section("Your role: "+r.Title, job)
			}
		}
		if l[0] == ScopeMember {
			section("You", m.Name+"\n\n"+m.Persona)
		}
		for _, c := range o.Contexts {
			if c.Scope == l[0] && c.Of == l[1] {
				section(c.Title, c.Body)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// putContext writes a context, keeping the text it had before.
func (o *Org) putContext(c Context, now time.Time) {
	c.Updated, c.Version, c.Previous = now, 1, nil
	if old, ok := o.Context(c.ID); ok {
		c.Version, c.Previous = old.Version, old.Previous
		if old.Body != c.Body {
			c.Version++
			c.Previous = append([]string{old.Body}, old.Previous...)
			if len(c.Previous) > keptVersions {
				c.Previous = c.Previous[:keptVersions]
			}
		}
	}
	o.Contexts = upsert(o.Contexts, c, func(x Context) string { return x.ID })
}

func (o Org) Context(id string) (Context, bool) {
	i := slices.IndexFunc(o.Contexts, func(c Context) bool { return c.ID == id })
	if i < 0 {
		return Context{}, false
	}
	return o.Contexts[i], true
}

// putRule writes a rule, refusing one that goes against a broader rule
// unless it says it is an exception. Rules it makes others go against
// are marked as exceptions too, since they were there first.
func (o *Org) putRule(r Rule) error {
	r.Overrides = o.Overridden(r)
	if len(r.Overrides) > 0 && !r.Exception {
		return fmt.Errorf("%w (%s); save it as an exception", ErrException, strings.Join(r.Overrides, ", "))
	}
	o.Rules = upsert(o.Rules, r, func(x Rule) string { return x.ID })
	o.markExceptions()
	return nil
}

func (o *Org) markExceptions() {
	for i := range o.Rules {
		o.Rules[i].Overrides = o.Overridden(o.Rules[i])
		o.Rules[i].Exception = len(o.Rules[i].Overrides) > 0
	}
}

// dropScoped removes the contexts and rules of something that is gone.
func (o *Org) dropScoped(scope, of string) {
	o.Contexts = slices.DeleteFunc(o.Contexts, func(c Context) bool { return c.Scope == scope && c.Of == of })
	o.Rules = slices.DeleteFunc(o.Rules, func(r Rule) bool { return r.Scope == scope && r.Of == of })
	o.markExceptions()
}

package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"strings"
	"sync"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/protect"
)

// Rule is one decision the owner made, in a form the engine can check
// without a language model. Text is what the owner wrote.
type Rule struct {
	ID   string  `json:"id"`
	Text string  `json:"text"`
	When When    `json:"when"`
	Then Verdict `json:"then"`
	// Off keeps a rule without enforcing it.
	Off bool `json:"off,omitempty"`
}

// When matches actions. Empty fields match everything.
type When struct {
	Capabilities []string `json:"capabilities,omitempty"`
	// MinRisk matches actions at this risk or above: read, notify, reversible, irreversible.
	MinRisk string `json:"min_risk,omitempty"`
	// Source matches "routine:<id>" or "exploration"; a trailing * is a prefix.
	Source string `json:"source,omitempty"`
	// ArgsContain matches when any argument text contains one of these, case-insensitive.
	ArgsContain []string `json:"args_contain,omitempty"`
	// Hosts matches http calls to these hosts.
	Hosts []string `json:"hosts,omitempty"`
	// People and Roles match who the run acts for ("ana"; "member", "guest").
	People []string `json:"people,omitempty"`
	Roles  []string `json:"roles,omitempty"`
	// Except leaves these capabilities out, e.g. every irreversible action
	// but one another rule handles.
	Except []string `json:"except,omitempty"`
}

func (w When) matches(a Action) bool {
	if len(w.Capabilities) > 0 && !contains(w.Capabilities, a.Capability) {
		return false
	}
	if contains(w.Except, a.Capability) {
		return false
	}
	if w.MinRisk != "" && a.Risk < parseRisk(w.MinRisk) {
		return false
	}
	if w.Source != "" {
		src := a.Source
		if i := strings.IndexByte(src, '#'); i >= 0 {
			src = src[:i]
		}
		if strings.HasSuffix(w.Source, "*") {
			if !strings.HasPrefix(src, strings.TrimSuffix(w.Source, "*")) {
				return false
			}
		} else if src != w.Source && !strings.HasPrefix(src, w.Source+":") {
			return false
		}
	}
	if len(w.People) > 0 && !contains(w.People, person(a)) {
		return false
	}
	if len(w.Roles) > 0 && !contains(w.Roles, role(a)) {
		return false
	}
	if len(w.Hosts) > 0 && !contains(w.Hosts, strings.ToLower(a.Scope)) {
		return false
	}
	if len(w.ArgsContain) > 0 {
		b, _ := json.Marshal(a.Args)
		text := strings.ToLower(string(b))
		hit := false
		for _, s := range w.ArgsContain {
			hit = hit || strings.Contains(text, strings.ToLower(s))
		}
		if !hit {
			return false
		}
	}
	return true
}

func person(a Action) string {
	if a.Person == "" {
		return "owner"
	}
	return a.Person
}

func role(a Action) string {
	if a.Role == "" {
		return "owner"
	}
	return a.Role
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func parseRisk(s string) capability.Risk {
	switch s {
	case "notify":
		return capability.Notify
	case "reversible":
		return capability.Reversible
	case "irreversible":
		return capability.Irreversible
	}
	return capability.Read
}

// Validate rejects rules the engine could not enforce as written.
func (r Rule) Validate() error {
	switch r.Then {
	case Allow, Reversible, Ask, Block:
	default:
		return fmt.Errorf("rule decision must be allow, reversible, ask or block, not %q", r.Then)
	}
	for _, c := range append(append([]string{}, r.When.Capabilities...), r.When.Except...) {
		if _, ok := capability.Catalog[c]; !ok {
			return fmt.Errorf("unknown capability %q", c)
		}
	}
	switch r.When.MinRisk {
	case "", "read", "notify", "reversible", "irreversible":
	default:
		return fmt.Errorf("unknown risk %q", r.When.MinRisk)
	}
	for _, role := range r.When.Roles {
		if role != "owner" && role != "member" && role != "guest" {
			return fmt.Errorf("unknown role %q", role)
		}
	}
	return nil
}

// Preset is the balanced starting point chosen for new installs: reads and
// messages to the owner go through, reversible changes go through and stay
// undoable, anything irreversible asks first, and new web hosts ask once.
func Preset() []Rule {
	return []Rule{
		{ID: "preset-irreversible", Text: "Sempre me pergunte antes de qualquer coisa que não dá para desfazer.", When: When{MinRisk: "irreversible"}, Then: Ask},
	}
}

// Presets are the three starting points offered at setup.
func Presets() map[string][]Rule {
	return map[string][]Rule{
		"conservative": {
			{ID: "preset-changes", Text: "Me pergunte antes de qualquer mudança, mesmo as reversíveis.", When: When{MinRisk: "reversible"}, Then: Ask},
		},
		"balanced": Preset(),
		"liberal": {liberalAsk,
			{ID: "preset-delete", Text: "Apagar sempre vira mover para a lixeira.", When: When{Capabilities: []string{"gmail.delete"}}, Then: Reversible},
		},
	}
}

// liberalAsk keeps the liberal level's promise: sending to other people,
// and anything else that cannot be undone, asks first. Only deleting email,
// which becomes moving it to the trash, goes through.
var liberalAsk = Rule{ID: "preset-liberal", Text: "Me pergunte antes de enviar algo para outras pessoas ou de qualquer coisa que não dá para desfazer.",
	When: When{MinRisk: "irreversible", Except: []string{"gmail.delete"}}, Then: Ask}

// upgrade replaces the liberal level's first rule, which asked only
// before gmail.send and so let other irreversible actions through
// (messages on other services, clearing a sheet, imported tools). A rule
// the owner edited is left alone.
func upgrade(rules []Rule) []Rule {
	for i, r := range rules {
		if r.ID == "preset-others" && r.Then == Ask && len(r.When.Capabilities) == 1 && r.When.Capabilities[0] == "gmail.send" &&
			r.When.MinRisk == "" && r.When.Source == "" && len(r.When.People) == 0 && len(r.When.Roles) == 0 && len(r.When.ArgsContain) == 0 {
			rules[i] = liberalAsk
			rules[i].Off = r.Off
		}
	}
	return rules
}

// alwaysAsk lists capabilities no rule may let through unasked.
var alwaysAsk = map[string]string{
	"whatsapp.send_to": "messages to other people on WhatsApp always wait for approval",
	"ha.critical":      "locks, alarms, covers and valves always wait for approval",
}

// strength orders verdicts: when several rules match, the strictest wins.
var strength = map[Verdict]int{Allow: 0, Reversible: 1, Ask: 2, Block: 3}

// Engine applies the owner's rules. Rules are kept in the event store so a
// change is logged and survives restarts.
type Engine struct {
	Events *event.Store
	// Protect is the shared protection list; its entries block, unless the
	// owner chose to ignore one.
	Protect *protect.Guard

	mu    sync.Mutex
	cache []Rule
}

const rulesKey = "policy.rules"
const hostsKey = "policy.hosts"

func (e *Engine) Rules(ctx context.Context) []Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cache != nil {
		return e.cache
	}
	raw, _ := e.Events.Get(ctx, rulesKey)
	var rules []Rule
	if raw == "" {
		rules = Preset()
	} else {
		json.Unmarshal([]byte(raw), &rules)
		rules = upgrade(rules)
	}
	e.cache = rules
	return rules
}

func (e *Engine) SaveRules(ctx context.Context, rules []Rule, actor string) error {
	for _, r := range rules {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("rule %q: %w", r.Text, err)
		}
	}
	b, _ := json.Marshal(rules)
	if err := e.Events.Put(ctx, rulesKey, string(b)); err != nil {
		return err
	}
	e.mu.Lock()
	e.cache = rules
	e.mu.Unlock()
	_, err := e.Events.Append(ctx, "rules.changed", actor, map[string]any{"rules": rules})
	return err
}

const ignoredKey = "protect.ignored"

func (e *Engine) ignored(ctx context.Context, id string) bool {
	raw, _ := e.Events.Get(ctx, ignoredKey)
	return strings.Contains(","+raw+",", ","+id+",")
}

// IgnoreProtection stops one protection entry from blocking here, for a
// false positive the owner contests.
func (e *Engine) IgnoreProtection(ctx context.Context, id, actor string) error {
	raw, _ := e.Events.Get(ctx, ignoredKey)
	if !e.ignored(ctx, id) {
		raw = strings.Trim(raw+","+id, ",")
	}
	if err := e.Events.Put(ctx, ignoredKey, raw); err != nil {
		return err
	}
	_, err := e.Events.Append(ctx, "protect.ignored", actor, map[string]string{"id": id})
	return err
}

// AllowHost remembers the owner's answer about a web host.
func (e *Engine) AllowHost(ctx context.Context, host string, allowed bool, actor string) error {
	hosts := e.hosts(ctx)
	hosts[strings.ToLower(host)] = allowed
	b, _ := json.Marshal(hosts)
	if err := e.Events.Put(ctx, hostsKey, string(b)); err != nil {
		return err
	}
	_, err := e.Events.Append(ctx, "policy.host", actor, map[string]any{"host": host, "allowed": allowed})
	return err
}

func (e *Engine) hosts(ctx context.Context) map[string]bool {
	raw, _ := e.Events.Get(ctx, hostsKey)
	m := map[string]bool{}
	json.Unmarshal([]byte(raw), &m)
	return m
}

// Decide checks the rules; with no match, the risk decides: reversible
// changes are allowed but made undoable, everything else is allowed. A
// guest's request never changes anything without the responsible person.
func (e *Engine) Decide(ctx context.Context, a Action) Decision {
	if e.Protect != nil {
		if hit, ok := e.Protect.Check(a.Capability, a.Scope, a.Args); ok && !e.ignored(ctx, hit.ID) {
			return Decision{Verdict: Block, Reason: i18n.T(ctx, "policy.protect", "reason", hit.Reason, "count", hit.Reports), Rule: "protect:" + hit.ID}
		}
	}
	d := e.decide(ctx, a)
	if why := alwaysAsk[a.Capability]; why != "" && strength[d.Verdict] < strength[Ask] {
		return Decision{Verdict: Ask, Reason: why}
	}
	if role(a) == "guest" && a.Risk >= capability.Reversible && strength[d.Verdict] < strength[Ask] {
		return Decision{Verdict: Ask, Reason: "guests' requests wait for the person responsible"}
	}
	return d
}

func (e *Engine) decide(ctx context.Context, a Action) Decision {
	d := Decision{Verdict: Allow}
	if a.Risk == capability.Reversible {
		d = Decision{Verdict: Reversible, Reason: "reversible changes stay undoable"}
	}
	if a.Scope != "" {
		switch allowed, known := e.hosts(ctx)[strings.ToLower(a.Scope)]; {
		case known && !allowed:
			return Decision{Verdict: Block, Reason: "you said no to " + a.Scope}
		case !known && strings.HasPrefix(a.Source, "exploration"):
			d = Decision{Verdict: Ask, Reason: "first time reaching " + a.Scope}
		}
	}
	// Blocks always win. A rule naming both a source and capabilities (what
	// "always allow" creates) is specific and overrides general rules;
	// among rules of the same kind the strictest wins.
	var general, specific *Decision
	for _, r := range e.Rules(ctx) {
		if r.Off || !r.When.matches(a) {
			continue
		}
		if r.Then == Block {
			return Decision{Verdict: Block, Reason: r.Text, Rule: r.ID}
		}
		slot := &general
		if r.When.Source != "" && len(r.When.Capabilities) > 0 {
			slot = &specific
		}
		if *slot == nil || strength[r.Then] > strength[(*slot).Verdict] {
			*slot = &Decision{Verdict: r.Then, Reason: r.Text, Rule: r.ID}
		}
	}
	switch {
	case specific != nil:
		// An allow still keeps reversible changes undoable.
		if specific.Verdict == Allow && d.Verdict == Reversible {
			specific.Verdict = Reversible
		}
		return *specific
	case general != nil && strength[general.Verdict] >= strength[d.Verdict]:
		return *general
	}
	return d
}

// AllowAlways is the rule an "always" answer creates: this capability, for
// this routine, without asking again.
func AllowAlways(a Action) Rule {
	src := a.Source
	if i := strings.IndexByte(src, '#'); i >= 0 {
		src = src[:i]
	}
	return Rule{
		ID:   "always-" + strings.NewReplacer(":", "-", ".", "-").Replace(src+"-"+a.Capability),
		Text: fmt.Sprintf("%s pode usar %s sem perguntar", src, a.Capability),
		When: When{Capabilities: []string{a.Capability}, Source: src},
		Then: Allow,
	}
}

// Matches reports whether a rule applies to an action, for previews.
func (e *Engine) Matches(r Rule, a Action) bool { return !r.Off && r.When.matches(a) }

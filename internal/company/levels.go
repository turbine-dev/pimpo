package company

import (
	"fmt"
	"slices"
	"strings"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/policy"
)

// Who decides at a level.
const (
	LevelSelf = "self"
	LevelBoss = "boss"
	LevelHead = "head"
	LevelCEO  = "ceo"
)

// How a level's decision reaches the CEO: at once, or up the tree with
// each boss adding a recommendation and none deciding.
const (
	RouteDirect   = "direct"
	RouteOpinions = "opinions"
)

// A Level is one of a company's decision levels: who decides at it and
// what puts a decision there. A decision is at the highest level any of
// whose triggers fire, and at the first level when none does.
type Level struct {
	Level   int      `json:"level" yaml:"level"`
	Name    string   `json:"name" yaml:"name"`
	Decides string   `json:"decides" yaml:"decides"`
	Route   string   `json:"route,omitempty" yaml:"route,omitempty"`
	When    Triggers `json:"when" yaml:"when"`
}

// Triggers put a decision at a level: an amount over OverUSD, a risk at
// MinRisk or above, one of Kinds (price, contract, hiring...), being
// public, or one of Words in what is decided.
type Triggers struct {
	OverUSD float64  `json:"over_usd,omitempty" yaml:"over_usd,omitempty"`
	MinRisk string   `json:"min_risk,omitempty" yaml:"min_risk,omitempty"`
	Kinds   []string `json:"kinds,omitempty" yaml:"kinds,omitempty"`
	Public  bool     `json:"public,omitempty" yaml:"public,omitempty"`
	Words   []string `json:"words,omitempty" yaml:"words,omitempty"`
}

// Levels are a company's decision levels and how unclear cases are
// classified.
type Levels struct {
	List []Level `json:"list,omitempty" yaml:"list,omitempty"`
	// Unsure is how sure the classifier must be that a decision is not
	// for the top level; below it, the decision goes up a level.
	Unsure float64 `json:"unsure,omitempty" yaml:"unsure,omitempty"`
}

// SuggestedLevels are the four levels the RFC suggests.
func SuggestedLevels() Levels {
	return Levels{Unsure: 0.8, List: []Level{
		{Level: 1, Name: "Operational", Decides: LevelSelf},
		{Level: 2, Name: "Tactical", Decides: LevelBoss, When: Triggers{OverUSD: 0, MinRisk: "irreversible"}},
		{Level: 3, Name: "Managerial", Decides: LevelHead, When: Triggers{OverUSD: 20, Kinds: []string{"roadmap", "architecture"}}},
		{Level: 4, Name: "Strategic", Decides: LevelCEO, Route: RouteOpinions, When: Triggers{OverUSD: 100, Public: true,
			Kinds: []string{"price", "contract", "partnership", "hiring", "budget", "legal"}}},
	}}
}

func (l Levels) check() error {
	if len(l.List) == 0 {
		return nil
	}
	for i, x := range l.List {
		switch {
		case x.Level != i+1:
			return fmt.Errorf("levels are numbered 1, 2, 3… in order")
		case strings.TrimSpace(x.Name) == "":
			return fmt.Errorf("level %d needs a name", x.Level)
		case !slices.Contains([]string{LevelSelf, LevelBoss, LevelHead, LevelCEO}, x.Decides):
			return fmt.Errorf("level %d is decided by self, boss, head or ceo", x.Level)
		case x.Route != "" && x.Route != RouteDirect && x.Route != RouteOpinions:
			return fmt.Errorf("level %d reaches the CEO directly or with opinions", x.Level)
		case x.When.OverUSD < 0:
			return fmt.Errorf("level %d: amounts are not negative", x.Level)
		}
		if err := (policy.Rule{When: policy.When{MinRisk: x.When.MinRisk}, Then: policy.Ask}).Validate(); err != nil {
			return fmt.Errorf("level %d: %w", x.Level, err)
		}
	}
	if top := l.List[len(l.List)-1]; top.Decides != LevelCEO {
		return fmt.Errorf("the highest level is the CEO's")
	}
	if l.Unsure != 0 && (l.Unsure < 0.5 || l.Unsure >= 1) {
		return fmt.Errorf("the classifier's certainty is 0.5 to 0.99")
	}
	return nil
}

// A Matter is what a level is found for: an action or a question.
type Matter struct {
	Capability string
	Risk       capability.Risk
	AmountUSD  float64
	Kind       string
	Public     bool
	Text       string
}

// Classify finds a matter's level by the triggers alone, with what set
// it. Zero when the company has no levels.
func (l Levels) Classify(m Matter) (int, string) {
	if len(l.List) == 0 {
		return 0, ""
	}
	text := strings.ToLower(fold(m.Text))
	for i := len(l.List) - 1; i > 0; i-- {
		x := l.List[i]
		w := x.When
		switch {
		case w.OverUSD > 0 && m.AmountUSD > w.OverUSD:
			return x.Level, fmt.Sprintf("more than $%.0f", w.OverUSD)
		case w.MinRisk != "" && m.Capability != "" && (policy.When{MinRisk: w.MinRisk}).Matches(policy.Action{Capability: m.Capability, Risk: m.Risk}):
			return x.Level, "risk " + w.MinRisk
		case m.Kind != "" && slices.ContainsFunc(w.Kinds, func(k string) bool { return same(k, m.Kind) }):
			return x.Level, "kind " + m.Kind
		case w.Public && m.Public:
			return x.Level, "public"
		}
		for _, word := range w.Words {
			if word != "" && strings.Contains(text, strings.ToLower(fold(word))) {
				return x.Level, "mentions " + word
			}
		}
	}
	return 1, ""
}

// Top is the highest level, the CEO's.
func (l Levels) Top() (Level, bool) {
	if len(l.List) == 0 {
		return Level{}, false
	}
	return l.List[len(l.List)-1], true
}

func (l Levels) At(level int) (Level, bool) {
	if level < 1 || level > len(l.List) {
		return Level{}, false
	}
	return l.List[level-1], true
}

// Head is the member at the top of member's department: the highest boss
// still in it, or the boss when the member has no department.
func (o Org) Head(member string) (Member, bool) {
	m, ok := o.Member(member)
	if !ok {
		return Member{}, false
	}
	head, found := o.Boss(member)
	if m.Department == "" {
		return head, found
	}
	for _, up := range o.Chain(member) {
		if up.Department == m.Department && up.Kind == Agent {
			head, found = up, true
		}
	}
	return head, found
}

// DeciderAt is who decides a member's matter at a level, as a member of
// the company: the member itself, its boss, the head of its department or
// the CEO seat.
func (o Org) DeciderAt(member string, l Level) (Member, bool) {
	switch l.Decides {
	case LevelSelf:
		return o.Member(member)
	case LevelBoss:
		return o.Boss(member)
	case LevelHead:
		return o.Head(member)
	}
	return o.Member(CEO)
}

// Package company keeps companies of agents: departments, roles and the
// members who hold them, in a tree whose top is a person, the CEO. A
// company belongs to the person who created it; partners are other people
// of the house it is shared with.
package company

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Grants a partner may have, weakest first.
const (
	View      = "view"
	Approve   = "approve"
	Configure = "configure"
)

// Kinds of member.
const (
	Agent  = "agent"
	Person = "person"
)

// Member states.
const (
	Active = "active"
	Paused = "paused"
)

// CEO is the id of the seat every company starts with: its person, at the top.
const CEO = "ceo"

type Company struct {
	ID       string `json:"id"`
	Person   string `json:"person,omitempty"`
	Name     string `json:"name"`
	Industry string `json:"industry,omitempty"`
	Mission  string `json:"mission,omitempty"`
	Zone     string `json:"zone,omitempty"`
	Hours    Hours  `json:"hours"`
	Paused   bool   `json:"paused,omitempty"`
	// Decider is who decides, by default, what a member's action asks
	// first; none means a person.
	Decider Decider `json:"decider,omitzero"`
	// Levels say which decisions go up to whom, the highest to the CEO.
	Levels Levels `json:"levels,omitzero"`
	// Budget is what the company may spend; past it, members stop or are
	// only warned, as OnLimit says.
	Budget Budget `json:"budget,omitzero"`
	// Accounts are the company's shared accounts and who may use them.
	Accounts []SharedAccount `json:"accounts,omitempty"`
	// Memory says how each scope of memory is written.
	Memory MemoryPolicy `json:"memory,omitzero"`
	// Lateral lets members hand work to others of their department, not
	// only to the people below them.
	Lateral  bool      `json:"lateral,omitempty"`
	Partners []Partner `json:"partners,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
}

type Partner struct {
	Person string `json:"person"`
	Grant  string `json:"grant"`
}

type Department struct {
	ID     string `json:"id" yaml:"id"`
	Name   string `json:"name" yaml:"name"`
	Color  string `json:"color,omitempty" yaml:"color,omitempty"`
	Paused bool   `json:"paused,omitempty" yaml:"-"`
	// MonthUSD is what the department may spend a month; 0 is no limit.
	MonthUSD float64 `json:"month_usd,omitempty" yaml:"month_usd,omitempty"`
}

// A Role is a function in the company that members hold: what it is for,
// and what a member holding it starts with.
type Role struct {
	ID               string     `json:"id" yaml:"id"`
	Title            string     `json:"title" yaml:"title"`
	Function         string     `json:"function,omitempty" yaml:"function,omitempty"`
	Responsibilities []string   `json:"responsibilities,omitempty" yaml:"responsibilities,omitempty"`
	Deliverables     []string   `json:"deliverables,omitempty" yaml:"deliverables,omitempty"`
	Capabilities     []string   `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Models           []string   `json:"models,omitempty" yaml:"models,omitempty"`
	AccountKinds     []string   `json:"account_kinds,omitempty" yaml:"account_kinds,omitempty"`
	Autonomy         []Autonomy `json:"autonomy,omitempty" yaml:"autonomy,omitempty"`
}

// A Member is an agent holding a role, or a person holding a seat (the
// CEO). An agent's empty Capabilities or Models mean its role's.
type Member struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Person       string   `json:"person,omitempty"`
	Title        string   `json:"title,omitempty"`
	Role         string   `json:"role,omitempty"`
	Department   string   `json:"department,omitempty"`
	ReportsTo    string   `json:"reports_to,omitempty"`
	Name         string   `json:"name"`
	Avatar       string   `json:"avatar,omitempty"`
	Persona      string   `json:"persona,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Models       []string `json:"models,omitempty"`
	State        string   `json:"state,omitempty"`
	// Hours, when set, replace the company's for this member.
	Hours *Hours `json:"hours,omitempty"`
	// Autonomy is who decides what this member's actions ask first, before
	// its role's matrix and the company's default.
	Autonomy []Autonomy `json:"autonomy,omitempty"`
	// Budget is the member's own limit, its salary.
	Budget  Budget    `json:"budget,omitzero"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// An Org is a company with everything in it.
type Org struct {
	Company
	Departments   []Department   `json:"departments"`
	Roles         []Role         `json:"roles"`
	Members       []Member       `json:"members"`
	Contexts      []Context      `json:"contexts"`
	Rules         []Rule         `json:"rules"`
	AgentRoutines []AgentRoutine `json:"agent_routines"`
}

var (
	ErrNotFound = errors.New("no such company")
	idPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
)

// ValidID says whether id may name a department, role or member.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Slug turns a name into an id: "Ana Lima" is "ana-lima".
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(fold(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	return s
}

func fold(s string) string {
	out, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), s)
	if err != nil {
		return s
	}
	return out
}

// Grant is what person may do in the company: its own person configures
// it, a partner has the grant they were given, anyone else nothing.
func (c Company) Grant(person string) string {
	if person == c.Person {
		return Configure
	}
	for _, p := range c.Partners {
		if p.Person == person {
			return p.Grant
		}
	}
	return ""
}

// Allows says whether a grant is at least need.
func Allows(grant, need string) bool {
	rank := map[string]int{View: 1, Approve: 2, Configure: 3}
	return grant != "" && rank[grant] >= rank[need]
}

func (o Org) Member(id string) (Member, bool) {
	i := slices.IndexFunc(o.Members, func(m Member) bool { return m.ID == id })
	if i < 0 {
		return Member{}, false
	}
	return o.Members[i], true
}

func (o Org) Role(id string) (Role, bool) {
	i := slices.IndexFunc(o.Roles, func(r Role) bool { return r.ID == id })
	if i < 0 {
		return Role{}, false
	}
	return o.Roles[i], true
}

func (o Org) Department(id string) (Department, bool) {
	i := slices.IndexFunc(o.Departments, func(d Department) bool { return d.ID == id })
	if i < 0 {
		return Department{}, false
	}
	return o.Departments[i], true
}

// Reports are the members who report to id.
func (o Org) Reports(id string) []Member {
	var out []Member
	for _, m := range o.Members {
		if m.ReportsTo == id {
			out = append(out, m)
		}
	}
	return out
}

// Chain is id's bosses, nearest first, up to the top.
func (o Org) Chain(id string) []Member {
	var out []Member
	m, ok := o.Member(id)
	for ok && m.ReportsTo != "" && len(out) <= len(o.Members) {
		if m, ok = o.Member(m.ReportsTo); ok {
			out = append(out, m)
		}
	}
	return out
}

// Check says what is wrong with the company, if anything: the CEO seat is
// its person and the only member without a boss, every boss exists, there
// are no loops, and every agent holds a role that exists.
func (o Org) Check() error {
	if strings.TrimSpace(o.Name) == "" {
		return errors.New("a company needs a name")
	}
	for _, p := range o.Partners {
		if p.Person == "" || p.Person == o.Person || !slices.Contains([]string{View, Approve, Configure}, p.Grant) {
			return fmt.Errorf("partner %q: the grant is view, approve or configure", p.Person)
		}
	}
	seen := map[string]bool{}
	for _, d := range o.Departments {
		if !ValidID(d.ID) || seen["d:"+d.ID] || strings.TrimSpace(d.Name) == "" {
			return fmt.Errorf("department %q needs a unique id and a name", d.ID)
		}
		seen["d:"+d.ID] = true
	}
	for _, r := range o.Roles {
		if !ValidID(r.ID) || seen["r:"+r.ID] || strings.TrimSpace(r.Title) == "" {
			return fmt.Errorf("role %q needs a unique id and a title", r.ID)
		}
		seen["r:"+r.ID] = true
	}
	ceo, ok := o.Member(CEO)
	if !ok || ceo.Kind != Person || ceo.Person != o.Person || ceo.ReportsTo != "" {
		return errors.New("the CEO seat is the company's person, at the top")
	}
	for _, m := range o.Members {
		switch {
		case !ValidID(m.ID) || seen["m:"+m.ID]:
			return fmt.Errorf("member %q needs a unique id of lowercase letters, digits and dashes", m.ID)
		case strings.TrimSpace(m.Name) == "":
			return fmt.Errorf("member %q needs a name", m.ID)
		case m.ID != CEO && m.ReportsTo == "":
			return fmt.Errorf("%s needs someone to report to", m.Name)
		case m.ReportsTo != "" && !o.has(m.ReportsTo):
			return fmt.Errorf("%s reports to %q, who is not in the company", m.Name, m.ReportsTo)
		case m.Department != "" && !seen["d:"+m.Department]:
			return fmt.Errorf("%s is in department %q, which does not exist", m.Name, m.Department)
		case m.Kind == Agent && !seen["r:"+m.Role]:
			return fmt.Errorf("%s needs a role that exists", m.Name)
		case m.Kind == Person && m.Person == "":
			return fmt.Errorf("%s is a seat for a person, and needs one", m.Name)
		case m.Kind != Agent && m.Kind != Person:
			return fmt.Errorf("%s is an agent or a person", m.Name)
		case m.State != "" && m.State != Active && m.State != Paused:
			return fmt.Errorf("%s is active or paused", m.Name)
		}
		seen["m:"+m.ID] = true
	}
	for _, m := range o.Members {
		if len(o.Chain(m.ID)) > len(o.Members) || !o.reachesTop(m.ID) {
			return fmt.Errorf("%s is in a loop of bosses", m.Name)
		}
	}
	if err := o.checkLayers(); err != nil {
		return err
	}
	return o.checkWork()
}

func (o Org) has(id string) bool { _, ok := o.Member(id); return ok }

func (o Org) reachesTop(id string) bool {
	if id == CEO {
		return true
	}
	chain := o.Chain(id)
	return len(chain) > 0 && chain[len(chain)-1].ID == CEO
}

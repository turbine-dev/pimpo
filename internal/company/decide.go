package company

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/policy"
)

// Kinds of decider.
const (
	DecideSelf      = "self"
	DecideJev       = "jev"
	DecideLaya      = "laya"
	DecideModel     = "model"
	DecideBoss      = "boss"
	DecidePerson    = "person"
	DecideCommittee = "committee"
	DecideCascade   = "cascade"
)

// A Decider is who answers a decision: the member itself, Jev or Laya
// (Jev's open model, on this computer) with a threshold, a model, the boss, a person, a committee of members voting,
// or a cascade that goes on to the next step while a step is unsure.
type Decider struct {
	Kind      string    `json:"kind" yaml:"kind"`
	Threshold float64   `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	Model     string    `json:"model,omitempty" yaml:"model,omitempty"`
	Members   []string  `json:"members,omitempty" yaml:"members,omitempty"`
	Unanimous bool      `json:"unanimous,omitempty" yaml:"unanimous,omitempty"`
	Steps     []Decider `json:"steps,omitempty" yaml:"steps,omitempty"`
}

func (d Decider) check(o Org) error {
	switch d.Kind {
	case DecideSelf, DecideBoss, DecidePerson:
	case DecideJev, DecideLaya:
		if d.Threshold < 0.5 || d.Threshold >= 1 {
			return fmt.Errorf("%s decides with a threshold from 0.5 to 0.99", map[string]string{DecideJev: "Jev", DecideLaya: "Laya"}[d.Kind])
		}
	case DecideModel:
	case DecideCommittee:
		if len(d.Members) < 2 {
			return fmt.Errorf("a committee has at least two members")
		}
		for _, m := range d.Members {
			if err := o.CheckWork(m); err != nil {
				return fmt.Errorf("committee member %q: %w", m, err)
			}
		}
	case DecideCascade:
		if len(d.Steps) < 2 {
			return fmt.Errorf("a cascade has at least two steps")
		}
		for _, s := range d.Steps {
			if s.Kind == DecideCascade {
				return fmt.Errorf("a cascade's steps are not cascades")
			}
			if err := s.check(o); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("a decider is self, jev, model, boss, person, committee or cascade, not %q", d.Kind)
	}
	return nil
}

// Autonomy is one line of a member's autonomy matrix: for a capability,
// or for every capability at a risk or above, who decides when an action
// would ask first.
type Autonomy struct {
	Capability string  `json:"capability,omitempty" yaml:"capability,omitempty"`
	MinRisk    string  `json:"min_risk,omitempty" yaml:"min_risk,omitempty"`
	Decider    Decider `json:"decider" yaml:"decider"`
	// Earned says a person gave it after a run of approvals.
	Earned bool `json:"earned,omitempty" yaml:"earned,omitempty"`
}

func (a Autonomy) matches(act policy.Action) bool {
	if a.Capability != "" {
		return a.Capability == act.Capability
	}
	return policy.When{MinRisk: a.MinRisk}.Matches(act)
}

func checkAutonomy(o Org, list []Autonomy) error {
	for _, a := range list {
		if a.Capability != "" {
			if _, ok := capability.Catalog[a.Capability]; !ok {
				return fmt.Errorf("unknown capability %q", a.Capability)
			}
		}
		if err := (policy.Rule{When: policy.When{MinRisk: a.MinRisk}, Then: policy.Ask}).Validate(); err != nil {
			return err
		}
		if err := a.Decider.check(o); err != nil {
			return err
		}
	}
	return nil
}

// DeciderFor is who decides an action of a member that would ask first:
// the member's own matrix, then its role's, then the company's default,
// and a person when none says. A line naming the capability beats one
// for a risk.
func (o Org) DeciderFor(member string, act policy.Action) Decider {
	m, _ := o.Member(member)
	r, _ := o.Role(m.Role)
	for _, list := range [][]Autonomy{m.Autonomy, r.Autonomy} {
		for _, a := range list {
			if a.Capability == act.Capability {
				return a.Decider
			}
		}
		for _, a := range list {
			if a.Capability == "" && a.matches(act) {
				return a.Decider
			}
		}
	}
	if o.Decider.Kind != "" {
		return o.Decider
	}
	return Decider{Kind: DecidePerson}
}

// A Decision is one answer a decider gave, kept to be audited.
type Decision struct {
	ID       string    `json:"id"`
	Company  string    `json:"company"`
	Member   string    `json:"member"`
	Question string    `json:"question"`
	Decider  string    `json:"decider"`
	Answer   string    `json:"answer"`
	P        float64   `json:"p,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	CostUSD  float64   `json:"cost_usd,omitempty"`
	Level    int       `json:"level,omitempty"`
	Why      string    `json:"why,omitempty"`
	Created  time.Time `json:"created"`
}

const decisionSchema = `
CREATE TABLE IF NOT EXISTS company_decisions (
  id         TEXT PRIMARY KEY,
  company    TEXT NOT NULL,
  data       TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS company_decisions_company ON company_decisions (company, created_at);`

func (s *Store) SaveDecision(ctx context.Context, d Decision) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO company_decisions (id, company, data, created_at) VALUES (?, ?, ?, ?)`, d.ID, d.Company, string(b), d.Created.UTC().Format(time.RFC3339Nano))
	return err
}

// Decisions are a company's latest decisions, newest first.
func (s *Store) Decisions(ctx context.Context, company string, limit int) ([]Decision, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM company_decisions WHERE company = ? ORDER BY created_at DESC LIMIT ?`, company, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Decision{}
	for rows.Next() {
		var data string
		var d Decision
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Label says a decider in a few words, for the log.
func (d Decider) Label() string {
	switch d.Kind {
	case DecideJev, DecideLaya:
		return fmt.Sprintf("%s ≥ %.2f", d.Kind, d.Threshold)
	case DecideModel:
		if d.Model != "" {
			return "model " + d.Model
		}
	case DecideCommittee:
		return fmt.Sprintf("committee of %d", len(d.Members))
	case DecideCascade:
		out := ""
		for i, s := range d.Steps {
			if i > 0 {
				out += " → "
			}
			out += s.Label()
		}
		return out
	}
	return d.Kind
}

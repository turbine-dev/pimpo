package company

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

// A finance member proposes budgets; a person decides. Budgets are
// limits on what the company spends with Pimpo: no money moves.

// Budget proposals' scopes and states.
const (
	ProposalProposed = "proposed"
	ProposalAccepted = "accepted"
	ProposalDeclined = "declined"
)

type Proposal struct {
	ID       string    `json:"id"`
	Company  string    `json:"company"`
	By       string    `json:"by"`
	Scope    string    `json:"scope"`
	Of       string    `json:"of,omitempty"`
	MonthUSD float64   `json:"month_usd"`
	Was      float64   `json:"was"`
	Reason   string    `json:"reason"`
	State    string    `json:"state"`
	Decided  string    `json:"decided,omitempty"`
	Created  time.Time `json:"created"`
}

// CheckProposal says whether a proposal names a budget the company has.
func (o Org) CheckProposal(p Proposal) error {
	if p.MonthUSD <= 0 || p.MonthUSD > 100000 || strings.TrimSpace(p.Reason) == "" {
		return errors.New("a proposal gives a monthly amount and why")
	}
	switch p.Scope {
	case ScopeCompany:
		if p.Of != "" {
			return errors.New("the company's budget has no of")
		}
	case ScopeDepartment:
		if _, ok := o.Department(p.Of); !ok {
			return ErrNotFound
		}
	case ScopeMember:
		if m, ok := o.Member(p.Of); !ok || m.Kind != Agent {
			return ErrNotFound
		}
	default:
		return errors.New("a budget is the company's, a department's or a member's")
	}
	return nil
}

// MonthBudget is the monthly limit a proposal would change.
func (o Org) MonthBudget(scope, of string) float64 {
	switch scope {
	case ScopeDepartment:
		d, _ := o.Department(of)
		return d.MonthUSD
	case ScopeMember:
		m, _ := o.Member(of)
		return m.Budget.MonthUSD
	}
	return o.Budget.MonthUSD
}

func (s *Store) SaveProposal(ctx context.Context, p Proposal) error {
	return s.saveRow(ctx, "company_proposals", p.ID, p.Company, p, p.Created)
}

func (s *Store) Proposals(ctx context.Context, company string) ([]Proposal, error) {
	return rows[Proposal](ctx, s, `SELECT data FROM company_proposals WHERE company = ? ORDER BY created_at DESC`, company)
}

// Pending are a company's proposals waiting for a person.
func Pending(ps []Proposal) []Proposal {
	return slices.DeleteFunc(slices.Clone(ps), func(p Proposal) bool { return p.State != ProposalProposed })
}

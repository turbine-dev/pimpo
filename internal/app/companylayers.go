package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/server"
)

const (
	maxContexts    = 200
	maxContextText = 64 << 10
	maxRules       = 200
)

// companyDecides puts a company's rules on top of the house's for an
// action of one of its members. Its rules may restrict and allow among
// themselves, but never past the house's: the stricter verdict wins.
func (a *App) companyDecides(ctx context.Context, act policy.Action, house policy.Decision) policy.Decision {
	id, member, _ := strings.Cut(act.Member, "/")
	o, err := a.Companies.Org(ctx, id)
	if err != nil {
		return policy.Decision{Verdict: policy.Block, Reason: "the member's company is gone"}
	}
	if _, ok := o.Member(member); !ok {
		return policy.Decision{Verdict: policy.Block, Reason: "no longer in the company"}
	}
	if d, ok := o.Decide(member, act); ok && policy.Stricter(house.Verdict, d.Verdict) != house.Verdict {
		return d
	}
	return house
}

type ruleView struct {
	Capability string         `json:"capability"`
	Risk       string         `json:"risk"`
	Verdict    policy.Verdict `json:"verdict"`
	Reason     string         `json:"reason,omitempty"`
	Rule       string         `json:"rule,omitempty"`
}

// previewMember is what a member receives: its brief, and the verdict on
// each capability it may use, with the rule that gives it.
func (a *App) previewMember(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
	ctx := people.With(r.Context(), o.Person)
	m, ok := o.Member(r.PathValue("part"))
	if !ok {
		return nil, company.ErrNotFound
	}
	caps := m.Capabilities
	if role, ok := o.Role(m.Role); ok && len(caps) == 0 {
		caps = role.Capabilities
	}
	views := []ruleView{}
	for _, c := range caps {
		spec := capability.Catalog[c]
		act := policy.Action{Capability: c, Risk: spec.Risk, Source: "member:" + o.ID, Person: o.Person, Role: string(a.roleOf(ctx)), Member: o.ID + "/" + m.ID}
		d := a.decide(ctx, act)
		views = append(views, ruleView{c, spec.Risk.String(), d.Verdict, d.Reason, d.Rule})
	}
	return map[string]any{"brief": o.Brief(m.ID), "rules": views}, nil
}

func (a *App) putContext(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
	var c company.Context
	if err := server.Decode(r, &c); err != nil {
		return nil, err
	}
	c.ID, c.Title = r.PathValue("part"), strings.TrimSpace(c.Title)
	_, exists := o.Context(c.ID)
	switch {
	case len(c.Body) > maxContextText || len([]rune(c.Title)) > 80:
		return nil, server.StatusError{Status: 400, Msg: "keep the title under 80 characters and the text under 64 KB"}
	case !exists && len(o.Contexts) >= maxContexts:
		return nil, server.StatusError{Status: 400, Msg: "a company keeps up to 200 contexts"}
	}
	return a.Companies.SaveContext(r.Context(), o.ID, c)
}

func (a *App) putCompanyRule(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
	var rule company.Rule
	if err := server.Decode(r, &rule); err != nil {
		return nil, err
	}
	rule.ID, rule.Text = r.PathValue("part"), strings.TrimSpace(rule.Text)
	exists := false
	for _, x := range o.Rules {
		exists = exists || x.ID == rule.ID
	}
	switch {
	case rule.Text == "" || len([]rune(rule.Text)) > 500:
		return nil, server.StatusError{Status: 400, Msg: "say what the rule is, in up to 500 characters"}
	case !exists && len(o.Rules) >= maxRules:
		return nil, server.StatusError{Status: 400, Msg: "a company keeps up to 200 rules"}
	}
	return a.Companies.SaveRule(r.Context(), o.ID, rule)
}

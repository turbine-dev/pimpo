package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/turbine-dev/pimpo/internal/approval"
	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Decision levels put a decision where it belongs. The triggers decide
// first, without a model; for a question they leave below the top, a
// classifier says whether it is the CEO's after all, and when it is
// unsure the decision goes up to the CEO. A decision is never put lower.

// levelOf is a matter's level, with what put it there.
func (a *App) levelOf(ctx context.Context, o company.Org, m company.Matter) (int, string) {
	level, why := o.Levels.Classify(m)
	top, ok := o.Levels.Top()
	if !ok || level == top.Level || o.Levels.Unsure == 0 || strings.TrimSpace(m.Text) == "" {
		return level, why
	}
	ans, err := a.judge(people.With(ctx, o.Person), "Is this a decision for the company's "+top.Name+" level, the CEO's, by these triggers: "+triggers(top.When)+"?",
		map[string]string{"decision": m.Text, "kind": m.Kind})
	if err != nil {
		return top.Level, "no judge answered, so up to the CEO"
	}
	a.Budget.Record(people.With(ctx, o.Person), budget.Cost{USD: ans.CostUSD, Source: "judgment", Ref: "company:" + o.ID, Member: o.ID + "/"})
	switch {
	case ans.P >= 0.5:
		return top.Level, fmt.Sprintf("judged the CEO's (%.0f%%)", ans.P*100)
	case 1-ans.P < o.Levels.Unsure:
		// Unsure whether it is the CEO's: it is, since sending one too many
		// costs a question and missing one costs the decision.
		return top.Level, fmt.Sprintf("unsure (%.0f%%), so up to the CEO", ans.P*100)
	}
	return level, why
}

func triggers(t company.Triggers) string {
	var parts []string
	if t.OverUSD > 0 {
		parts = append(parts, fmt.Sprintf("more than $%.0f", t.OverUSD))
	}
	if len(t.Kinds) > 0 {
		parts = append(parts, "about "+strings.Join(t.Kinds, ", "))
	}
	if t.Public {
		parts = append(parts, "public")
	}
	if len(t.Words) > 0 {
		parts = append(parts, "mentioning "+strings.Join(t.Words, ", "))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "; ")
}

// actionMatter is an action as a matter to classify: its capability, its
// risk and the largest amount it moves.
func actionMatter(act policy.Action) company.Matter {
	m := company.Matter{Capability: act.Capability, Risk: act.Risk, Text: actionText(act)}
	for _, n := range approval.OperationOf(act.Capability, act.Args).Amounts {
		m.AmountUSD = max(m.AmountUSD, n)
	}
	return m
}

// rank orders who decides, the least careful first.
func rank(d company.Decider) int {
	switch d.Kind {
	case company.DecideSelf:
		return 0
	case company.DecideJev, company.DecideModel, company.DecideCommittee:
		return 1
	case company.DecideBoss:
		return 2
	case company.DecideCascade:
		r := 0
		for _, s := range d.Steps {
			r = max(r, rank(s))
		}
		return r
	}
	return 4
}

// levelDecider is who a level asks to decide an action, as a decider.
func levelDecider(l company.Level) (company.Decider, int) {
	switch l.Decides {
	case company.LevelSelf:
		return company.Decider{Kind: company.DecideSelf}, 0
	case company.LevelBoss:
		return company.Decider{Kind: company.DecideBoss}, 2
	case company.LevelHead:
		return company.Decider{Kind: "head"}, 3
	}
	return company.Decider{Kind: company.DecidePerson}, 4
}

// raise is the decider for an action once its level is known: the
// member's own, unless the level asks for someone more careful.
func (o companyLevels) raise(member string, act policy.Action, dec company.Decider) (company.Decider, int, string) {
	level, why := o.Levels.Classify(actionMatter(act))
	l, ok := o.Levels.At(level)
	if !ok {
		return dec, level, why
	}
	if ld, r := levelDecider(l); r > rank(dec) {
		return ld, level, why
	}
	return dec, level, why
}

type companyLevels struct{ company.Org }

func (a *App) companyLevelRoutes() {
	a.Server.Handle("POST /api/companies/{id}/levels/simulate", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var in struct {
			Member    string  `json:"member"`
			Text      string  `json:"text"`
			Kind      string  `json:"kind"`
			AmountUSD float64 `json:"amount_usd"`
			Public    bool    `json:"public"`
			Action    string  `json:"capability"`
		}
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		m := company.Matter{Kind: in.Kind, AmountUSD: in.AmountUSD, Public: in.Public, Text: in.Text, Capability: in.Action}
		if spec, ok := capability.Catalog[in.Action]; ok {
			m.Risk = spec.Risk
		}
		level, why := a.levelOf(r.Context(), o, m)
		out := map[string]any{"level": level, "why": why}
		if l, ok := o.Levels.At(level); ok {
			out["name"], out["decides"] = l.Name, l.Decides
			if d, ok := o.DeciderAt(in.Member, l); ok && in.Member != "" {
				out["decider"] = d.Name
			}
		}
		return out, nil
	}))
}

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/turbine-dev/pimpo/internal/approval"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/server"
)

// deliveryAnswered counts a person's answer to a company member's action:
// approvals in a row lead to a suggestion of autonomy for that kind, and
// a no takes an earned one back to approval.
func (a *App) deliveryAnswered(ctx context.Context, act policy.Action, ans approval.Answer, err error) {
	co, me, ok := strings.Cut(act.Member, "/")
	if !ok || act.Rehearsal {
		return
	}
	o, oerr := a.Companies.Org(ctx, co)
	if oerr != nil {
		return
	}
	switch {
	case err == nil && (ans == approval.Once || ans == approval.Always):
		st, suggest, serr := a.Companies.Approved(ctx, co, me, act.Capability, o.Earning())
		if serr != nil || !suggest {
			return
		}
		m, _ := o.Member(me)
		a.Events.Append(ctx, "company.autonomy.suggested", "system", map[string]any{"company": co, "member": me, "capability": act.Capability, "count": st.Count, "person": o.Person})
		a.Channel.Notify(people.With(ctx, o.Person), explore.Notice{Text: fmt.Sprintf("%s · %s: %d %s approved in a row. Let %s do it alone?", o.Name, m.Name, st.Count, act.Capability, m.Name), To: o.Person, Kind: "task"})
	case errors.Is(err, approval.ErrDenied):
		was, derr := a.Companies.Denied(ctx, co, me, act.Capability)
		if derr == nil && was {
			a.takeBack(ctx, o, me, act.Capability)
		}
	}
}

// takeBack removes an earned autonomy line: that kind asks again.
func (a *App) takeBack(ctx context.Context, o company.Org, me, capability string) {
	m, ok := o.Member(me)
	if !ok {
		return
	}
	m.Autonomy = slices.DeleteFunc(m.Autonomy, func(l company.Autonomy) bool { return l.Earned && l.Capability == capability })
	if _, err := a.Companies.SaveMember(ctx, o.ID, m); err == nil {
		a.Events.Append(ctx, "company.autonomy.taken_back", "system", map[string]any{"company": o.ID, "member": me, "capability": capability, "person": o.Person})
	}
}

// earn answers a suggestion: yes gives the member that kind to do alone,
// no starts the count again. On an earned kind, no takes it back.
func (a *App) earn(ctx context.Context, o company.Org, me, capability string, yes bool) (company.Org, error) {
	m, ok := o.Member(me)
	if !ok || m.Kind != company.Agent {
		return o, company.ErrNotFound
	}
	streaks, _ := a.Companies.Streaks(ctx, o.ID)
	i := slices.IndexFunc(streaks, func(s company.Streak) bool { return s.Member == me && s.Capability == capability })
	if i < 0 || !streaks[i].Suggested && !streaks[i].Earned {
		return o, server.StatusError{Status: 400, Msg: "that kind was not suggested for this member"}
	}
	if !yes {
		if streaks[i].Earned {
			a.takeBack(ctx, o, me, capability)
		}
		if err := a.Companies.Decline(ctx, o.ID, me, capability); err != nil {
			return o, err
		}
		return a.Companies.Org(ctx, o.ID)
	}
	if !slices.ContainsFunc(m.Autonomy, func(l company.Autonomy) bool {
		return l.Capability == capability && l.Decider.Kind == company.DecideSelf
	}) {
		// First, so it decides before broader lines; the decision levels and
		// what always asks still go above it.
		m.Autonomy = append([]company.Autonomy{{Capability: capability, Decider: company.Decider{Kind: company.DecideSelf}, Earned: true}}, m.Autonomy...)
	}
	saved, err := a.Companies.SaveMember(ctx, o.ID, m)
	if err != nil {
		return o, err
	}
	if err := a.Companies.Earn(ctx, o.ID, me, capability); err != nil {
		return o, err
	}
	a.Events.Append(ctx, "company.autonomy.earned", actor(ctx), map[string]any{"company": o.ID, "member": me, "capability": capability, "person": o.Person})
	return saved, nil
}

// earnNeeds are the suggestions of autonomy waiting for the person.
func (a *App) earnNeeds(ctx context.Context) []need {
	me := people.From(ctx)
	out := []need{}
	all, _ := a.Companies.List(ctx)
	for _, c := range all {
		o, err := a.Companies.Org(ctx, c.ID)
		if err != nil || !company.Allows(o.Grant(me), company.Configure) {
			continue
		}
		streaks, _ := a.Companies.Streaks(ctx, o.ID)
		for _, s := range streaks {
			if !s.Suggested {
				continue
			}
			m, _ := o.Member(s.Member)
			out = append(out, need{ID: "earn:" + o.ID + ":" + s.Member + ":" + s.Capability, Title: fmt.Sprintf("%s: %s", o.Name, m.Name), Detail: s.Capability, Count: s.Count,
				Created: s.Updated, Urgency: urgencyWhenFree, Link: "/companies/" + o.ID, Actions: []string{"accept", "dismiss"}})
		}
	}
	return out
}

func (a *App) companyEarnRoutes() {
	a.Server.Handle("GET /api/companies/{id}/streaks", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.Streaks(r.Context(), o.ID)
	}))
	a.Server.Handle("POST /api/companies/{id}/members/{part}/earn", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var in struct {
			Capability string `json:"capability"`
			Accept     bool   `json:"accept"`
		}
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		return a.earn(r.Context(), o, r.PathValue("part"), in.Capability, in.Accept)
	}))
}

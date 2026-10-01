package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// A finance member reads what the company spends with Pimpo (and, with
// the Stripe and GitHub connectors, what comes in), writes the monthly
// report, points out what looks wrong and proposes budgets. Nothing it
// does moves money.

const (
	anomalyTimes = 2.0
	anomalyFloor = 1.0
	paceTimes    = 1.5
	paceFloor    = 5.0
	financeKey   = "finance.report."
)

func init() {
	for _, s := range []capability.Spec{
		{Name: "costs.read", Risk: capability.Read, Signature: "costs.read({month})",
			Returns: "{month, total, members, departments, by_day, subscription, previous_total, budget, forecast_month, anomalies} what the company spent with Pimpo in a month (YYYY-MM, this one by default), by member and department, in dollars",
			Schema:  `{"type":"object","properties":{"month":{"type":"string","description":"YYYY-MM"}}}`},
		{Name: "company.budget_propose", Risk: capability.Notify, Signature: "company.budget_propose({scope, of, month_usd, reason})",
			Returns: "{proposal}; proposes a monthly budget for the company, a department (of: its id) or a member (of: its id) for the CEO to decide; it changes a limit, never moves money",
			Schema:  `{"type":"object","properties":{"scope":{"type":"string","enum":["company","department","member"]},"of":{"type":"string"},"month_usd":{"type":"number"},"reason":{"type":"string"}},"required":["scope","month_usd","reason"]}`},
	} {
		capability.Register(s)
	}
}

type financeCap struct{ a *App }

func (financeCap) Capabilities() []string { return []string{"costs.read", "company.budget_propose"} }

func (c financeCap) Call(ctx context.Context, name, _ string, args any) (any, error) {
	a := c.a
	o, me, _, err := a.caller(ctx)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(args)
	switch name {
	case "costs.read":
		var in struct {
			Month string `json:"month"`
		}
		json.Unmarshal(b, &in)
		return a.costsRead(ctx, o, in.Month)
	case "company.budget_propose":
		var in company.Proposal
		json.Unmarshal(b, &in)
		p := company.Proposal{ID: newTeamID("p_"), Company: o.ID, By: me, Scope: in.Scope, Of: in.Of, MonthUSD: in.MonthUSD, Reason: clip(strings.TrimSpace(in.Reason), 2000), State: company.ProposalProposed, Created: time.Now().UTC()}
		if err := o.CheckProposal(p); err != nil {
			return nil, err
		}
		p.Was = o.MonthBudget(p.Scope, p.Of)
		if err := a.Companies.SaveProposal(ctx, p); err != nil {
			return nil, err
		}
		a.Events.Append(ctx, "company.budget.proposed", actorFor(o, me), map[string]any{"company": o.ID, "proposal": p.ID, "scope": p.Scope, "of": p.Of, "month_usd": p.MonthUSD, "person": o.Person})
		return map[string]string{"proposal": p.ID}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

type monthCosts struct {
	Month         string             `json:"month"`
	Total         float64            `json:"total"`
	Members       map[string]float64 `json:"members"`
	Departments   map[string]float64 `json:"departments"`
	ByDay         map[string]float64 `json:"by_day"`
	Subscription  float64            `json:"subscription"`
	PreviousTotal float64            `json:"previous_total"`
	Budget        company.Budget     `json:"budget"`
	ForecastMonth float64            `json:"forecast_month,omitempty"`
	Anomalies     []string           `json:"anomalies"`
}

// costsRead is a month of a company's spending, with what looks wrong.
func (a *App) costsRead(ctx context.Context, o company.Org, month string) (monthCosts, error) {
	zone := companyLevels{o}.zone()
	now := time.Now().In(zone)
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, zone)
	if month != "" {
		t, err := time.ParseInLocation("2006-01", month, zone)
		if err != nil {
			return monthCosts{}, errors.New("month is YYYY-MM")
		}
		start = t
	}
	end := start.AddDate(0, 1, 0)
	prev := start.AddDate(0, -1, 0)
	out := monthCosts{Month: start.Format("2006-01"), Members: map[string]float64{}, Departments: map[string]float64{}, ByDay: map[string]float64{}, Budget: o.Budget, Anomalies: []string{}}
	// The two weeks before the month too, for the anomalies of its first days.
	daily := map[string]float64{}
	lastMonth := map[string]float64{}
	costs, at, _ := a.Budget.Since(ctx, prev)
	for i, c := range costs {
		co, member, ok := strings.Cut(c.Member, "/")
		if !ok || co != o.ID {
			continue
		}
		when := at[i].In(zone)
		daily[when.Format(time.DateOnly)] += c.USD
		switch {
		case when.Before(start):
			out.PreviousTotal += c.USD
			lastMonth[member] += c.USD
		case when.Before(end):
			out.Total += c.USD
			out.ByDay[when.Format(time.DateOnly)] += c.USD
			out.Members[nameIn(o, member)] += c.USD
			if m, ok := o.Member(member); ok && m.Department != "" {
				d, _ := o.Department(m.Department)
				out.Departments[d.Name] += c.USD
			}
		}
	}
	evs, _ := a.Events.List(ctx, event.Query{Types: []string{"subscription.used"}})
	for _, e := range evs {
		var u struct {
			USD    float64 `json:"usd"`
			Member string  `json:"member"`
		}
		if e.Decode(&u) == nil && strings.HasPrefix(u.Member, o.ID+"/") && !e.Time.Before(start) && e.Time.Before(end) {
			out.Subscription += u.USD
		}
	}
	for d := start; d.Before(end) && !d.After(now); d = d.AddDate(0, 0, 1) {
		if why := dayAnomaly(daily, d); why != "" {
			out.Anomalies = append(out.Anomalies, why)
		}
	}
	if start.Equal(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, zone)) {
		out.ForecastMonth = a.costs(ctx, o).ForecastMonth
		// A member on pace to spend much more than last month.
		elapsed := now.Sub(start).Hours() / 24
		days := end.Sub(start).Hours() / 24
		for _, m := range o.Members {
			spent := out.Members[m.Name]
			if m.Kind != company.Agent || elapsed < 3 || spent == 0 {
				continue
			}
			pace := spent / elapsed * days
			if was := lastMonth[m.ID]; pace > paceFloor && pace > paceTimes*was {
				out.Anomalies = append(out.Anomalies, fmt.Sprintf("%s is on pace for $%.2f this month, against $%.2f last month", m.Name, pace, was))
			}
		}
	}
	return out, nil
}

// dayAnomaly says why a day's spending stands out against the median of
// the two weeks before it, if it does.
func dayAnomaly(daily map[string]float64, d time.Time) string {
	spent := daily[d.Format(time.DateOnly)]
	if spent < anomalyFloor {
		return ""
	}
	var before []float64
	for i := 1; i <= 14; i++ {
		before = append(before, daily[d.AddDate(0, 0, -i).Format(time.DateOnly)])
	}
	sort.Float64s(before)
	median := (before[6] + before[7]) / 2
	if spent > anomalyTimes*median {
		return fmt.Sprintf("%s: $%.2f spent, against a usual $%.2f a day", d.Format(time.DateOnly), spent, median)
	}
	return ""
}

func nameIn(o company.Org, member string) string {
	if m, ok := o.Member(member); ok {
		return m.Name
	}
	return member
}

// decideProposal applies or declines a budget proposal; a budget is never
// given past what the company's person may spend.
func (a *App) decideProposal(ctx context.Context, o company.Org, id string, accept bool) (company.Proposal, error) {
	ps, _ := a.Companies.Proposals(ctx, o.ID)
	i := slices.IndexFunc(ps, func(p company.Proposal) bool { return p.ID == id })
	if i < 0 {
		return company.Proposal{}, company.ErrNotFound
	}
	p := ps[i]
	if p.State != company.ProposalProposed {
		return p, server.StatusError{Status: 400, Msg: "this proposal was decided already"}
	}
	p.State, p.Decided = company.ProposalDeclined, actor(ctx)
	if accept {
		if limit := a.Budget.LimitFor(ctx, o.Person); limit > 0 && p.MonthUSD > 31*limit {
			return p, server.StatusError{Status: 400, Msg: fmt.Sprintf("the company may spend at most its person's own limit, $%.2f a day", limit)}
		}
		var err error
		switch p.Scope {
		case company.ScopeCompany:
			c := o.Company
			c.Budget.MonthUSD = p.MonthUSD
			_, err = a.Companies.Update(ctx, c)
		case company.ScopeDepartment:
			d, _ := o.Department(p.Of)
			d.MonthUSD = p.MonthUSD
			_, err = a.Companies.SaveDepartment(ctx, o.ID, d)
		case company.ScopeMember:
			m, _ := o.Member(p.Of)
			m.Budget.MonthUSD = p.MonthUSD
			_, err = a.Companies.SaveMember(ctx, o.ID, m)
		}
		if err != nil {
			return p, err
		}
		p.State = company.ProposalAccepted
	}
	if err := a.Companies.SaveProposal(ctx, p); err != nil {
		return p, err
	}
	a.Events.Append(ctx, "company.budget."+p.State, actor(ctx), map[string]any{"company": o.ID, "proposal": p.ID, "person": o.Person})
	return p, nil
}

// askForReports has each finance member write last month's report once,
// from the first of the month.
func (a *App) askForReports(ctx context.Context, now time.Time) {
	all, _ := a.Companies.List(ctx)
	for _, c := range all {
		o, err := a.Companies.Org(ctx, c.ID)
		if err != nil {
			continue
		}
		local := now.In(companyLevels{o}.zone())
		if local.Day() != 1 || local.Hour() < 9 {
			continue
		}
		last := local.AddDate(0, -1, 0).Format("2006-01")
		for _, m := range o.Members {
			if m.Kind != company.Agent || !slices.Contains(capsOf(o, m), "costs.read") {
				continue
			}
			key := financeKey + o.ID + "." + m.ID + "." + last
			if done, _ := a.Events.Get(ctx, key); done != "" {
				continue
			}
			a.Events.Put(ctx, key, "asked")
			request := fmt.Sprintf("Write the company's report for %s: what it spent with Pimpo by member and department, against its budget and the month before (costs.read with month %s), what came in if you can read it, what looked wrong, and any budget you would propose (company.budget_propose). Keep the report with company.remember, kind report.", last, last)
			a.enqueue(people.With(ctx, o.Person), o, m.ID, request, nil, "finance:"+last, 0)
		}
	}
}

func capsOf(o company.Org, m company.Member) []string {
	if len(m.Capabilities) > 0 {
		return m.Capabilities
	}
	r, _ := o.Role(m.Role)
	return r.Capabilities
}

// proposalNeeds are the budget proposals waiting for the person.
func (a *App) proposalNeeds(ctx context.Context) []need {
	me := people.From(ctx)
	out := []need{}
	all, _ := a.Companies.List(ctx)
	for _, c := range all {
		o, err := a.Companies.Org(ctx, c.ID)
		if err != nil || !company.Allows(o.Grant(me), company.Configure) {
			continue
		}
		ps, _ := a.Companies.Proposals(ctx, o.ID)
		for _, p := range company.Pending(ps) {
			what := o.Name
			if p.Of != "" {
				what += " · " + nameIn(o, p.Of)
				if d, ok := o.Department(p.Of); ok {
					what = o.Name + " · " + d.Name
				}
			}
			out = append(out, need{ID: "budget:" + o.ID + ":" + p.ID, Title: fmt.Sprintf("%s: $%.2f a month (was $%.2f)", what, p.MonthUSD, p.Was), Detail: p.Reason,
				Created: p.Created, Urgency: urgencyWhenFree, Link: "/companies/" + o.ID + "?tab=costs", Actions: []string{"accept", "dismiss"}})
		}
	}
	return out
}

func (a *App) companyFinanceRoutes() {
	a.Server.Handle("GET /api/companies/{id}/proposals", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.Proposals(r.Context(), o.ID)
	}))
	a.Server.Handle("POST /api/companies/{id}/proposals/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var in struct {
			Accept bool `json:"accept"`
		}
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		return a.decideProposal(r.Context(), o, r.PathValue("part"), in.Accept)
	}))
	a.Server.Handle("GET /api/companies/{id}/month", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.costsRead(r.Context(), o, r.URL.Query().Get("month"))
	}))
}

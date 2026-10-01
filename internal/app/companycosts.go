package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/people"
)

// Every cost of a company is booked for one of its members. Budgets sit
// in layers (the company, a department, a member's salary, each piece of
// work), and all of them inside the person's own limit and the house's.

const budgetWarnKey = "company.budget."

type companySpend struct {
	Company     company.Spend            `json:"company"`
	Members     map[string]company.Spend `json:"members"`
	Departments map[string]company.Spend `json:"departments"`
	ByDay       map[string]float64       `json:"by_day"`
	// Subscription is what work paid by a subscription would have cost on
	// the API, kept apart from money spent.
	Subscription *companySpend `json:"subscription,omitempty"`
}

func newSpend() companySpend {
	return companySpend{Members: map[string]company.Spend{}, Departments: map[string]company.Spend{}, ByDay: map[string]float64{}}
}

func (o companyLevels) zone() *time.Location {
	if z, err := time.LoadLocation(o.Zone); err == nil {
		return z
	}
	return time.Local
}

// add books usd spent at when for a member of o.
func (s *companySpend) add(o company.Org, member string, usd float64, when, day, month time.Time) {
	s.ByDay[when.Format("2006-01-02")] += usd
	if when.Before(month) {
		return
	}
	put := func(x company.Spend) company.Spend {
		x.Month += usd
		if !when.Before(day) {
			x.Day += usd
		}
		return x
	}
	s.Company = put(s.Company)
	s.Members[member] = put(s.Members[member])
	if m, ok := o.Member(member); ok && m.Department != "" {
		s.Departments[m.Department] = put(s.Departments[m.Department])
	}
}

// spendOf is what a company spent today and this month, by member and
// department, and each of the last 30 days, with what its subscription
// work would have cost apart.
func (a *App) spendOf(ctx context.Context, o company.Org) companySpend {
	zone := companyLevels{o}.zone()
	now := time.Now().In(zone)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, zone)
	since := day.AddDate(0, 0, -29)
	if month.Before(since) {
		since = month
	}
	out, sub := newSpend(), newSpend()
	costs, at, _ := a.Budget.Since(ctx, since)
	for i, c := range costs {
		if co, member, ok := strings.Cut(c.Member, "/"); ok && co == o.ID {
			out.add(o, member, c.USD, at[i].In(zone), day, month)
		}
	}
	evs, _ := a.Events.List(ctx, event.Query{Types: []string{"subscription.used"}})
	for _, e := range evs {
		var u struct {
			USD    float64 `json:"usd"`
			Member string  `json:"member"`
		}
		if e.Time.Before(since) || e.Decode(&u) != nil {
			continue
		}
		if co, member, ok := strings.Cut(u.Member, "/"); ok && co == o.ID {
			sub.add(o, member, u.USD, e.Time.In(zone), day, month)
		}
	}
	if sub.Company.Month > 0 || len(sub.ByDay) > 0 {
		out.Subscription = &sub
	}
	return out
}

// counted is the spend a company's limits look at: money, and, when the
// company says so, its subscription work at API prices.
func (s companySpend) counted(o company.Org) companySpend {
	if !o.Budget.Subscription || s.Subscription == nil {
		return s
	}
	out := newSpend()
	out.Company = s.Company.Plus(s.Subscription.Company)
	for _, src := range []companySpend{s, *s.Subscription} {
		for k, v := range src.Members {
			out.Members[k] = out.Members[k].Plus(v)
		}
		for k, v := range src.Departments {
			out.Departments[k] = out.Departments[k].Plus(v)
		}
	}
	return out
}

// overBudget says which limit a member's work would go past, or "".
func overBudget(o company.Org, member string, s companySpend) string {
	m, _ := o.Member(member)
	if why := m.Budget.Over(s.Members[member]); why != "" {
		return m.Name + ": " + why
	}
	if d, ok := o.Department(m.Department); ok && d.MonthUSD > 0 {
		if why := (company.Budget{MonthUSD: d.MonthUSD}).Over(s.Departments[d.ID]); why != "" {
			return d.Name + ": " + why
		}
	}
	if why := o.Budget.Over(s.Company); why != "" {
		return o.Name + ": " + why
	}
	return ""
}

// leftFor is the most a member's next piece of work may spend, or -1.
func leftFor(o company.Org, member string, s companySpend) float64 {
	m, _ := o.Member(member)
	left := -1.0
	take := func(l float64) {
		if l >= 0 && (left < 0 || l < left) {
			left = l
		}
	}
	take(m.Budget.Left(s.Members[member]))
	if d, ok := o.Department(m.Department); ok && d.MonthUSD > 0 {
		take((company.Budget{MonthUSD: d.MonthUSD}).Left(s.Departments[d.ID]))
	}
	take(o.Budget.Left(s.Company))
	return left
}

// warnBudget tells the CEO once a day that a limit was reached or is near.
func (a *App) warnBudget(ctx context.Context, o company.Org, scope, text string) {
	key := budgetWarnKey + o.ID + "." + scope + "." + time.Now().In(companyLevels{o}.zone()).Format("2006-01-02")
	if v, _ := a.Events.Get(ctx, key); v != "" {
		return
	}
	a.Events.Put(ctx, key, "1")
	a.Events.Append(ctx, "company.budget", "system", map[string]string{"company": o.ID, "scope": scope, "text": text, "person": o.Person})
	a.Channel.Notify(people.With(ctx, o.Person), explore.Notice{Text: o.Name + " · " + text, To: o.Person, Kind: "task"})
}

// budgetGate is whether a member's work may start now, and what it may
// spend; it warns the CEO near and at the limits.
func (a *App) budgetGate(ctx context.Context, o company.Org, w company.Work, s companySpend) (bool, float64) {
	s = s.counted(o)
	if why := overBudget(o, w.Member, s); why != "" {
		a.warnBudget(ctx, o, "over."+w.Member, "budget reached: "+why)
		if o.Budget.OnLimit != company.OnLimitWarn {
			return false, 0
		}
		return true, w.MaxUSD
	}
	m, _ := o.Member(w.Member)
	if m.Budget.Near(s.Members[w.Member]) || o.Budget.Near(s.Company) {
		a.warnBudget(ctx, o, "near."+w.Member, "80% of the budget spent by "+m.Name)
	}
	if left := leftFor(o, w.Member, s); left >= 0 && left < w.MaxUSD {
		return left > 0.005, left
	}
	return true, w.MaxUSD
}

type costView struct {
	companySpend
	ForecastMonth float64                       `json:"forecast_month"`
	PerMember     map[string]memberCost         `json:"per_member"`
	PerRole       map[string]float64            `json:"per_role"`
	Budget        company.Budget                `json:"budget"`
	Limits        map[string]company.Budget     `json:"limits"`
	Outcomes      map[string]map[string]float64 `json:"outcomes"`
}

type memberCost struct {
	Done        int     `json:"done"`
	CostPerDone float64 `json:"cost_per_done"`
	Forecast    float64 `json:"forecast_month"`
}

// costs are a company's spending, forecast and cost per outcome.
func (a *App) costs(ctx context.Context, o company.Org) costView {
	s := a.spendOf(ctx, o)
	v := costView{companySpend: s, PerMember: map[string]memberCost{}, PerRole: map[string]float64{}, Budget: o.Budget, Limits: map[string]company.Budget{}, Outcomes: map[string]map[string]float64{}}
	works, _ := a.Companies.Works(ctx, o.ID, 1000)
	recent := time.Now().AddDate(0, 0, -14)
	byRoutine := map[string][]float64{}
	adhoc := map[string]float64{}
	for _, w := range works {
		if w.State == company.WorkDone {
			mc := v.PerMember[w.Member]
			mc.Done++
			mc.CostPerDone += w.CostUSD
			v.PerMember[w.Member] = mc
		}
		if strings.HasPrefix(w.From, "routine:") {
			byRoutine[strings.TrimPrefix(w.From, "routine:")] = append(byRoutine[strings.TrimPrefix(w.From, "routine:")], w.CostUSD)
		} else if w.Queued.After(recent) {
			adhoc[w.Member] += w.CostUSD
		}
	}
	for id, mc := range v.PerMember {
		if mc.Done > 0 {
			mc.CostPerDone /= float64(mc.Done)
		}
		v.PerMember[id] = mc
	}
	for _, r := range o.AgentRoutines {
		if r.Off || r.Schedule == "" {
			continue
		}
		sched, err := company.ParseSchedule(r.Schedule)
		if err != nil {
			continue
		}
		per := r.MaxUSD
		if per == 0 {
			per = workDefaultUSD
		}
		if past := byRoutine[r.ID]; len(past) > 0 {
			per = 0
			for _, c := range past {
				per += c
			}
			per /= float64(len(past))
		}
		mc := v.PerMember[r.Member]
		mc.Forecast += float64(runsInMonth(sched)) * per
		v.PerMember[r.Member] = mc
	}
	for member, spent := range adhoc {
		mc := v.PerMember[member]
		mc.Forecast += spent / 14 * 30
		v.PerMember[member] = mc
	}
	roles := map[string][]float64{}
	for _, m := range o.Members {
		mc := v.PerMember[m.ID]
		v.ForecastMonth += mc.Forecast
		if m.Kind == company.Agent {
			roles[m.Role] = append(roles[m.Role], max(mc.Forecast, s.Members[m.ID].Month))
			v.Limits[m.ID] = m.Budget
		}
	}
	for role, list := range roles {
		total := 0.0
		for _, x := range list {
			total += x
		}
		v.PerRole[role] = total / float64(len(list))
	}
	tasks, _ := a.Companies.Tasks(ctx, o.ID)
	done, spentOnTasks := 0, 0.0
	for _, w := range works {
		if w.Task != "" {
			spentOnTasks += w.CostUSD
		}
	}
	for _, t := range tasks {
		if t.State == company.TaskDone {
			done++
		}
	}
	if done > 0 {
		v.Outcomes["task"] = map[string]float64{"count": float64(done), "cost_each": spentOnTasks / float64(done)}
	}
	return v
}

// runsInMonth is how many times a schedule fires in the next 30 days.
func runsInMonth(s interface{ Next(time.Time) time.Time }) int {
	now := time.Now()
	end := now.AddDate(0, 0, 30)
	n := 0
	for t := s.Next(now); t.Before(end) && n < 3000; t = s.Next(t) {
		n++
	}
	return n
}

func (a *App) companyCostRoutes() {
	a.Server.Handle("GET /api/companies/{id}/costs", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.costs(r.Context(), o), nil
	}))
}

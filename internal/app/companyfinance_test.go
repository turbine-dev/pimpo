package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/people"
)

// spendAt books a member's cost as if it happened at when.
func (ta *testApp) spendAt(t *testing.T, member string, usd float64, when time.Time) {
	t.Helper()
	ta.Events.SetClock(func() time.Time { return when })
	defer ta.Events.SetClock(time.Now)
	if err := ta.Budget.Record(people.With(context.Background(), people.OwnerID), budget.Cost{USD: usd, Source: "work", Member: member}); err != nil {
		t.Fatal(err)
	}
}

func TestFinanceReadsTheMonthAndWhatLooksWrong(t *testing.T) {
	ta, co := clerk(t)
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	zone := companyLevels{o}.zone()
	now := time.Now().In(zone)
	start := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, zone)
	for i := 1; i <= 20; i++ {
		ta.spendAt(t, co+"/clara", 0.5, start.AddDate(0, 0, -i))
	}
	ta.spendAt(t, co+"/clara", 9, start)
	ta.spendAt(t, "other/x", 50, start)
	got, err := ta.costsRead(ctx, o, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 9 || got.Members["Clara"] != 9 || got.PreviousTotal < 7 || len(got.Anomalies) == 0 || !strings.Contains(got.Anomalies[0], "$9.00 spent") {
		t.Fatalf("this month = %+v", got)
	}
	prev, _ := ta.costsRead(ctx, o, start.AddDate(0, -1, 0).Format("2006-01"))
	if prev.Total < 7 || prev.Total > 10 || len(prev.Anomalies) != 0 {
		t.Fatalf("last month = %+v", prev)
	}
	if _, err := ta.costsRead(ctx, o, "September"); err == nil {
		t.Fatal("a month that is not YYYY-MM")
	}
}

func TestABudgetIsProposedAndAPersonDecides(t *testing.T) {
	ta, co := clerk(t)
	ctx := context.Background()
	base := "/api/companies/" + co
	ta.do(t, "PUT", base+"/roles/manager", map[string]any{"title": "Manager", "capabilities": []string{"costs.read", "company.budget_propose"}})
	fin := financeCap{ta.App}
	as := asMember(co+"/bia", "company.budget_propose")
	if _, err := fin.Call(as, "company.budget_propose", "", map[string]any{"scope": "member", "of": "clara", "month_usd": 30, "reason": "Clara answers twice as many customers"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]any{
		{"scope": "member", "of": "ceo", "month_usd": 30, "reason": "x"},
		{"scope": "department", "of": "nowhere", "month_usd": 30, "reason": "x"},
		{"scope": "company", "month_usd": -1, "reason": "x"},
		{"scope": "company", "month_usd": 10},
	} {
		if _, err := fin.Call(as, "company.budget_propose", "", bad); err == nil {
			t.Errorf("%v was proposed", bad)
		}
	}
	needs, _ := ta.needs(people.With(ctx, people.OwnerID))
	if !containsKind(needs, "company_budget") {
		t.Fatalf("the CEO was not asked: %+v", needs)
	}
	ps, _ := ta.Companies.Proposals(ctx, co)
	if code, out := ta.do(t, "POST", base+"/proposals/"+ps[0].ID, map[string]bool{"accept": true}); code != 200 || out["state"] != company.ProposalAccepted {
		t.Fatalf("accept: %d %v", code, out)
	}
	o, _ := ta.Companies.Org(ctx, co)
	if m, _ := o.Member("clara"); m.Budget.MonthUSD != 30 {
		t.Fatalf("Clara's salary = %+v", m.Budget)
	}
	if code, _ := ta.do(t, "POST", base+"/proposals/"+ps[0].ID, map[string]bool{"accept": false}); code != 400 {
		t.Fatal("a proposal was decided twice")
	}
}

func TestFinanceWritesTheMonthlyReportOnce(t *testing.T) {
	ta, co := clerk(t)
	ctx := context.Background()
	base := "/api/companies/" + co
	ta.do(t, "PUT", base+"/roles/manager", map[string]any{"title": "Manager", "capabilities": []string{"costs.read"}})
	o, _ := ta.Companies.Org(ctx, co)
	zone := companyLevels{o}.zone()
	first := time.Date(2026, 10, 1, 10, 0, 0, 0, zone)
	ta.askForReports(ctx, first.Add(-2*time.Hour))
	ta.askForReports(ctx, first)
	ta.askForReports(ctx, first.Add(time.Hour))
	works, _ := ta.Companies.Works(ctx, co, 10)
	var asked []company.Work
	for _, w := range works {
		if strings.HasPrefix(w.From, "finance:") {
			asked = append(asked, w)
		}
	}
	if len(asked) != 1 || asked[0].Member != "bia" || !strings.Contains(asked[0].Request, "2026-09") {
		t.Fatalf("asked = %+v", asked)
	}
}

func TestADayStandsOutAgainstTheTwoWeeksBefore(t *testing.T) {
	d := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	daily := map[string]float64{}
	for i := 1; i <= 14; i++ {
		daily[d.AddDate(0, 0, -i).Format(time.DateOnly)] = 1
	}
	for spent, want := range map[float64]bool{1.5: false, 2.5: true, 0.5: false} {
		daily[d.Format(time.DateOnly)] = spent
		if got := dayAnomaly(daily, d) != ""; got != want {
			t.Errorf("$%.2f = %v", spent, got)
		}
	}
}

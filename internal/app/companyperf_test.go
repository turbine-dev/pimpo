package app

import (
	"context"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
)

func TestEachMemberHasItsPerformance(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{{"Fix the cart", func(llm.AgentRequest) string { return "Fixed." }}}
	ta, co := team(t, s)
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	for range 2 {
		w, _ := ta.enqueue(ctx, o, "bia", "Fix the cart", nil, "test", 0)
		ta.waitWorkState(t, w.ID, company.WorkDone)
	}
	ta.Companies.SaveTask(ctx, company.Task{ID: "t_1", Company: co, Root: "t_1", Requester: "rui", Assignee: "bia", Title: "Cart", State: company.TaskDone})
	if code, _ := ta.do(t, "GET", "/api/companies/"+co+"/performance", nil); code != 200 {
		t.Fatalf("performance route: %d", code)
	}
	perf := ta.performance(ctx, o)
	var bia memberPerf
	for _, p := range perf {
		if p.Member == "bia" {
			bia = p
		}
	}
	if len(perf) != 2 || bia.Done != 2 || bia.Tasks != 1 || bia.CostUSD < 0.019 || bia.CostEach < 0.009 {
		t.Fatalf("performance = %+v", perf)
	}
}

func TestACompanyOnTheDashboardIsOnlyItsPeoples(t *testing.T) {
	h := newHouse(t)
	co := h.owner["company"]
	code, body := h.raw(t, "tok", "GET", "/api/widgets", nil)
	if code != 200 || !strings.Contains(body, `"id":"company:`+co+`:summary"`) || !strings.Contains(body, `"id":"company:`+co+`:spent"`) {
		t.Fatalf("the owner's widgets: %d %s", code, body)
	}
	if _, body := h.raw(t, h.ana, "GET", "/api/widgets", nil); strings.Contains(body, co) {
		t.Fatalf("Ana sees the owner's company on her dashboards: %s", body)
	}
	if _, ok := h.anyWidget(people.With(context.Background(), h.anaID), "company:"+co+":summary", false); ok {
		t.Fatal("Ana drew the owner's company widget")
	}
}

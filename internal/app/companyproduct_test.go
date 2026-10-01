package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/judge"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
)

// doubtful doubts any claim whose quote says "made up".
type doubtful struct{}

func (doubtful) Ask(_ context.Context, _ string, item any) (judge.Answer, error) {
	if strings.Contains(fmt.Sprint(item), "made up") {
		return judge.Answer{P: 0.1}, nil
	}
	return judge.Answer{P: 0.9}, nil
}

func TestAProductOwnersBriefIsCheckedAfterItShips(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{
		{"Gather the signals", func(r llm.AgentRequest) string {
			for _, sig := range []map[string]any{
				{"source": "issue", "title": "Dark mode", "url": "https://github.com/ana/shop/issues/4?utm_source=mail"},
				{"source": "forum", "title": "Dark mode too", "url": "https://github.com/ana/shop/issues/4"},
			} {
				errorf(t, rpc(r.MCPURL, 1, "company_signal", sig))
			}
			return "Two signals."
		}},
		{"Propose dark mode", func(r llm.AgentRequest) string {
			errorf(t, rpc(r.MCPURL, 1, "company_brief", map[string]any{
				"title": "Dark mode", "problem": "Night shoppers leave", "proposal": "A dark theme",
				"claims": []map[string]string{
					{"text": "Users ask for it", "source": "https://github.com/ana/shop/issues/4", "quote": "Please add a dark mode"},
					{"text": "Half the visits are at night", "source": "analytics", "quote": "made up"},
					{"text": "Competitors have it", "source": "https://rival.example"},
				},
				"scores":      map[string]int{"value": 4, "differentiation": 2, "adoption": 4, "build_risk": 2, "safety_risk": 1},
				"predictions": []map[string]string{{"metric": "night conversion", "expected": "+5%"}},
			}))
			return "Proposed."
		}},
		{"Day 30 after shipping", func(r llm.AgentRequest) string {
			id := r.Prompt[strings.Index(r.Prompt, "(b_")+1:]
			id = id[:strings.Index(id, ")")]
			errorf(t, rpc(r.MCPURL, 1, "company_brief_review", map[string]any{"brief": id, "day": 30, "results": []map[string]any{{"metric": "night conversion", "actual": "+7%", "met": true}}}))
			return "Reviewed."
		}},
	}
	ta, co := team(t, s)
	ta.DemoJudge = doubtful{}
	base := "/api/companies/" + co
	caps := []string{"company.signal", "company.signals", "company.brief", "company.briefs", "company.brief_review"}
	ta.do(t, "PUT", base+"/roles/pm", map[string]any{"title": "Product owner", "capabilities": caps})
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	run := func(request string) {
		t.Helper()
		w, _ := ta.enqueue(ctx, o, "rui", request, nil, "test", 0)
		if done := ta.waitWorkState(t, w.ID, company.WorkDone, company.WorkFailed); done.State != company.WorkDone {
			t.Fatalf("%s: %+v", request, done)
		}
	}
	run("Gather the signals")
	if sigs, _ := ta.Companies.Signals(ctx, co, 10); len(sigs) != 1 || sigs[0].Count != 2 || sigs[0].Seen[0] != "forum" {
		t.Fatalf("signals = %+v", sigs)
	}
	run("Propose dark mode")
	briefs, _ := ta.Companies.Briefs(ctx, co)
	if len(briefs) != 1 {
		t.Fatalf("briefs = %+v", briefs)
	}
	b := briefs[0]
	if b.Score != 2*4+2+2*4-2-1 || b.State != company.BriefProposed || b.Claims[0].Flag != "" || !strings.Contains(b.Claims[1].Flag, "Jev doubts") || b.Claims[2].Flag != "quotes nothing from its source" {
		t.Fatalf("brief = %+v", b)
	}
	if needs, _ := ta.needs(people.With(ctx, people.OwnerID)); !containsKind(needs, "company_brief") {
		t.Fatal("the CEO was not asked about the brief")
	}
	if code, _ := ta.do(t, "POST", base+"/briefs/"+b.ID+"/state", map[string]string{"state": "shipped"}); code != 400 {
		t.Fatal("a brief shipped before it was accepted")
	}
	ta.do(t, "POST", base+"/briefs/"+b.ID+"/state", map[string]string{"state": "accepted"})
	if code, out := ta.do(t, "POST", base+"/briefs/"+b.ID+"/state", map[string]string{"state": "shipped", "ref": "https://github.com/ana/shop/pull/9"}); code != 200 {
		t.Fatalf("ship: %d %v", code, out)
	}

	ta.dueBriefReviews(ctx, time.Now())
	if b, _ := ta.Companies.Brief(ctx, b.ID); len(b.Checks) != 0 {
		t.Fatal("a review was asked for on the day it shipped")
	}
	ta.Companies.UpdateBrief(ctx, b.ID, func(x *company.Brief) error { x.Shipped = x.Shipped.AddDate(0, 0, -31); return nil })
	ta.dueBriefReviews(ctx, time.Now())
	ta.dueBriefReviews(ctx, time.Now())
	ta.waitFor(t, "the day-30 review", func() bool {
		b, _ := ta.Companies.Brief(ctx, b.ID)
		return len(b.Reviews) == 1
	})
	if b, _ := ta.Companies.Brief(ctx, b.ID); len(b.Checks) != 1 || b.Reviews[0].By != "rui" {
		t.Fatalf("brief = %+v", b)
	}
	_, out := ta.do(t, "GET", base+"/product", nil)
	if acc := out["accuracy"].(map[string]any)["rui"].([]any); acc[0] != 1.0 || acc[1] != 1.0 {
		t.Fatalf("accuracy = %v", out["accuracy"])
	}
}

func containsKind(needs []need, kind string) bool {
	for _, n := range needs {
		if n.Kind == kind {
			return true
		}
	}
	return false
}

func TestAStandupHasWhatTheTeamDid(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{{"Fix the cart", func(llm.AgentRequest) string { return "Fixed." }}}
	ta, co := team(t, s)
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	w, _ := ta.enqueue(ctx, o, "bia", "Fix the cart", nil, "test", 0)
	ta.waitWorkState(t, w.ID, company.WorkDone)
	ta.Companies.SaveTask(ctx, company.Task{ID: "t_blocked", Company: co, Root: "t_blocked", Requester: "rui", Assignee: "bia", Title: "Pay with Pix", State: company.TaskBlocked, Report: "No sandbox account"})
	got := ta.standup(ctx, o, "rui")["members"].([]standupLine)
	if len(got) != 1 || got[0].Name != "Bia" || len(got[0].Done) != 1 || !strings.Contains(got[0].Done[0], "Fixed.") || !strings.Contains(got[0].Blocked[0], "No sandbox account") {
		t.Fatalf("standup = %+v", got)
	}
	if got := ta.standup(ctx, o, "bia")["members"].([]standupLine); len(got) != 0 {
		t.Fatalf("Bia's standup had people who are not hers: %+v", got)
	}
}

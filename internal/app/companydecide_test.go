package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/policy"
)

// clerk is a company where Clara, a clerk, may send email and messages,
// reporting to Bia, a manager.
func clerk(t *testing.T) (*testApp, string) {
	t.Helper()
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.companiesOn(t)
	_, out := ta.do(t, "POST", "/api/companies", map[string]any{"name": "Lume"})
	co := out["id"].(string)
	base := "/api/companies/" + co
	ta.do(t, "PUT", base+"/roles/manager", map[string]any{"title": "Manager"})
	ta.do(t, "PUT", base+"/roles/clerk", map[string]any{"title": "Clerk", "capabilities": []string{"gmail.send", "whatsapp.send_to"}})
	ta.do(t, "PUT", base+"/members/bia", map[string]any{"name": "Bia", "role": "manager", "reports_to": "ceo"})
	ta.do(t, "PUT", base+"/members/clara", map[string]any{"name": "Clara", "role": "clerk", "reports_to": "bia"})
	return ta, co
}

func (ta *testApp) autonomy(t *testing.T, co string, decider map[string]any) {
	t.Helper()
	if code, out := ta.do(t, "PUT", "/api/companies/"+co+"/members/clara", map[string]any{"name": "Clara", "role": "clerk", "reports_to": "bia",
		"autonomy": []map[string]any{{"min_risk": "irreversible", "decider": decider}}}); code != 200 {
		t.Fatalf("autonomy: %d %v", code, out)
	}
}

func sendAs(co string) policy.Action {
	return policy.Action{Capability: "gmail.send", Risk: capability.Irreversible, Args: map[string]any{"to": "ana@example.com"}, Source: "exploration:x", Person: "owner", Role: "owner", Member: co + "/clara"}
}

func TestWhoDecidesWhatAMemberAsks(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name    string
		decider map[string]any
		judge   float64
		model   []llm.Response
		want    policy.Verdict
	}{
		{"a person, by default", nil, 0, nil, policy.Ask},
		{"itself", map[string]any{"kind": "self"}, 0, nil, policy.Allow},
		{"Jev, sure", map[string]any{"kind": "jev", "threshold": 0.9}, 0.95, nil, policy.Allow},
		{"Jev, sure it should not", map[string]any{"kind": "jev", "threshold": 0.9}, 0.05, nil, policy.Block},
		{"Jev, unsure", map[string]any{"kind": "jev", "threshold": 0.9}, 0.6, nil, policy.Ask},
		{"a model saying no", map[string]any{"kind": "model"}, 0, []llm.Response{{Structured: json.RawMessage(`{"approve":false,"reason":"too soon"}`)}}, policy.Block},
		{"the boss agent", map[string]any{"kind": "boss"}, 0, []llm.Response{{Structured: json.RawMessage(`{"approve":true,"reason":"fine"}`)}}, policy.Allow},
		{"a split committee", map[string]any{"kind": "committee", "members": []string{"bia", "clara"}}, 0,
			[]llm.Response{{Structured: json.RawMessage(`{"approve":true,"reason":"ok"}`)}, {Structured: json.RawMessage(`{"approve":false,"reason":"no"}`)}}, policy.Block},
		{"a cascade going on from an unsure Jev", map[string]any{"kind": "cascade", "steps": []map[string]any{{"kind": "jev", "threshold": 0.9}, {"kind": "boss"}}}, 0.6,
			[]llm.Response{{Structured: json.RawMessage(`{"approve":true,"reason":"fine"}`)}}, policy.Allow},
	} {
		t.Run(c.name, func(t *testing.T) {
			ta, co := clerk(t)
			if c.decider != nil {
				ta.autonomy(t, co, c.decider)
			}
			ta.DemoJudge = fixedJudge(c.judge)
			ta.LLM = &llm.Fake{Responses: c.model}
			if d := ta.decide(ctx, sendAs(co)); d.Verdict != c.want {
				t.Fatalf("verdict = %+v", d)
			}
			if c.want != policy.Ask {
				if list, _ := ta.Companies.Decisions(ctx, co, 10); len(list) != 1 || list[0].Member != "clara" {
					t.Fatalf("decisions = %+v", list)
				}
			}
		})
	}
}

func TestSomeThingsAlwaysGoToAPerson(t *testing.T) {
	ctx := context.Background()
	ta, co := clerk(t)
	ta.autonomy(t, co, map[string]any{"kind": "self"})
	if d := ta.decide(ctx, policy.Action{Capability: "whatsapp.send_to", Risk: capability.Irreversible, Source: "exploration:x", Member: co + "/clara"}); d.Verdict != policy.Ask {
		t.Fatalf("a message to someone else was decided by the member: %+v", d)
	}
	rules := []policy.Rule{{ID: "careful-mail", Text: "Ask me before any email", When: policy.When{Capabilities: []string{"gmail.send"}}, Then: policy.Ask}}
	if err := ta.Rules.SaveRules(ctx, rules, "test"); err != nil {
		t.Fatal(err)
	}
	if d := ta.decide(ctx, sendAs(co)); d.Verdict != policy.Ask || d.Rule != "careful-mail" {
		t.Fatalf("the administrator's own rule was decided by the member: %+v", d)
	}
	// A rehearsal is never decided, so nothing is spent on it.
	ta.Rules.SaveRules(ctx, policy.Preset(), "test")
	ta.autonomy(t, co, map[string]any{"kind": "model"})
	fake := &llm.Fake{}
	ta.LLM = fake
	act := sendAs(co)
	act.Rehearsal = true
	if d := ta.decide(ctx, act); d.Verdict != policy.Ask || len(fake.Requests) != 0 {
		t.Fatalf("a rehearsal was decided: %+v, %d model calls", d, len(fake.Requests))
	}
}

func TestDecidersAreChecked(t *testing.T) {
	ta, co := clerk(t)
	for _, bad := range []map[string]any{
		{"kind": "jev", "threshold": 0.3},
		{"kind": "committee", "members": []string{"bia"}},
		{"kind": "committee", "members": []string{"bia", "ceo"}},
		{"kind": "cascade", "steps": []map[string]any{{"kind": "self"}}},
		{"kind": "oracle"},
	} {
		code, _ := ta.do(t, "PUT", "/api/companies/"+co+"/members/clara", map[string]any{"name": "Clara", "role": "clerk", "reports_to": "bia",
			"autonomy": []map[string]any{{"min_risk": "irreversible", "decider": bad}}})
		if code != 400 {
			t.Errorf("%v: %d", bad, code)
		}
	}
	_, out := ta.do(t, "GET", "/api/companies/"+co+"/members/clara/preview", nil)
	for _, r := range out["rules"].([]any) {
		r := r.(map[string]any)
		if r["capability"] == "gmail.send" && r["decider"] != "person" {
			t.Fatalf("preview = %v", r)
		}
	}
}

// No capability a member can call changes a company: members cannot give
// themselves autonomy, money, tools or accounts, or write exceptions.
func TestMembersCannotChangeTheirCompany(t *testing.T) {
	var got []string
	for name := range capability.Catalog {
		if strings.HasPrefix(name, "company.") {
			got = append(got, name)
		}
	}
	want := map[string]bool{"company.wake": true, "company.assign": true, "company.report": true, "company.ask": true, "company.answer": true,
		"company.remember": true, "company.recall": true, "company.meet": true,
		// Signals, briefs and their reviews are the product owner's own records.
		"company.signal": true, "company.signals": true, "company.brief": true, "company.briefs": true, "company.brief_review": true, "company.standup": true}
	if len(got) != len(want) {
		t.Fatalf("company capabilities = %v", got)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("%s is new: check it cannot change the company, then add it here", n)
		}
	}
	_ = company.CEO
}

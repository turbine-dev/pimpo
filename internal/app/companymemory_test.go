package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/llm"
)

func (ta *testApp) waitMeeting(t *testing.T, id string) company.Meeting {
	t.Helper()
	var m company.Meeting
	ta.waitFor(t, "the meeting to end", func() bool {
		m, _ = ta.Companies.Meeting(context.Background(), id)
		return m.State != company.MeetingRunning
	})
	return m
}

func TestAMeetingGoesRoundAndEndsInMinutes(t *testing.T) {
	ta, co := team(t, &script{})
	ta.LLM = &llm.Fake{Responses: []llm.Response{
		{Text: "Rui: ship Friday.", CostUSD: 0.01}, {Text: "Bia: Friday works.", CostUSD: 0.01},
		{Text: "Rui: agreed.", CostUSD: 0.01}, {Text: "Bia: I start today.", CostUSD: 0.01},
		{Structured: json.RawMessage(`{"minutes":"We plan the release.","decisions":["Ship on Friday"]}`), CostUSD: 0.01},
	}}
	code, out := ta.do(t, "POST", "/api/companies/"+co+"/meetings", map[string]any{"title": "Planning", "agenda": "When do we ship?", "chair": "rui", "participants": []string{"rui", "bia"}, "rounds": 2, "max_usd": 1})
	if code != 200 {
		t.Fatalf("meet: %d %v", code, out)
	}
	m := ta.waitMeeting(t, out["id"].(string))
	if m.State != company.MeetingDone || len(m.Transcript) != 4 || m.Transcript[1].Member != "bia" || m.Decisions[0] != "Ship on Friday" || m.CostUSD < 0.049 {
		t.Fatalf("meeting = %+v", m)
	}
	notes, _ := ta.Companies.Notes(context.Background(), co, 10)
	if len(notes) != 1 || notes[0].Kind != company.NoteMinutes || !strings.Contains(notes[0].Body, "Ship on Friday") {
		t.Fatalf("notes = %+v", notes)
	}
	fake := ta.LLM.(*llm.Fake)
	if p := fake.Requests[3].Prompt; !strings.Contains(p, "Rui: agreed.") || !strings.Contains(p, "Round 2 of 2") {
		t.Fatalf("Bia was not told what was said: %q", p)
	}
	for _, bad := range []map[string]any{
		{"title": "x", "agenda": "y", "chair": "rui", "participants": []string{"rui"}, "rounds": 1, "max_usd": 1},
		{"title": "x", "agenda": "y", "chair": "rui", "participants": []string{"rui", "bia"}, "rounds": 9, "max_usd": 1},
		{"title": "x", "agenda": "y", "chair": "rui", "participants": []string{"rui", "bia"}, "rounds": 1, "max_usd": 50},
		{"title": "x", "agenda": "y", "chair": "ceo", "participants": []string{"rui", "bia"}, "rounds": 1, "max_usd": 1},
	} {
		if code, _ := ta.do(t, "POST", "/api/companies/"+co+"/meetings", bad); code != 400 {
			t.Errorf("%v: %d", bad, code)
		}
	}
}

func TestAMeetingStopsAtItsCap(t *testing.T) {
	ta, co := team(t, &script{})
	ta.LLM = &llm.Fake{Responses: []llm.Response{{Text: "long", CostUSD: 0.6}, {Text: "longer", CostUSD: 0.6}, {Text: "never", CostUSD: 0.6}}}
	_, out := ta.do(t, "POST", "/api/companies/"+co+"/meetings", map[string]any{"title": "Talk", "agenda": "Everything", "chair": "rui", "participants": []string{"rui", "bia"}, "rounds": 2, "max_usd": 1})
	m := ta.waitMeeting(t, out["id"].(string))
	if m.State != company.MeetingFailed || len(m.Transcript) != 2 || !strings.Contains(m.Error, "cost cap") {
		t.Fatalf("meeting = %+v", m)
	}
}

func TestMembersRememberAndTheCompanyRemembersForThem(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{
		{"What did we decide", func(r llm.AgentRequest) string {
			if !strings.Contains(r.System, "Returns take 30 days") || !strings.Contains(r.System, "their words, not a rule") {
				return "no memory: " + r.System
			}
			return "We decided returns take 30 days."
		}},
		{"Note the returns policy", func(r llm.AgentRequest) string {
			errorf(t, rpc(r.MCPURL, 1, "company_remember", map[string]any{"kind": "decision", "title": "Returns", "text": "Returns take 30 days."}))
			return "Noted."
		}},
	}
	ta, co := team(t, s)
	ta.do(t, "PUT", "/api/companies/"+co+"/roles/dev", map[string]any{"title": "Developer", "capabilities": []string{"company.remember", "company.recall"}})
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	first, _ := ta.enqueue(ctx, o, "bia", "Note the returns policy", nil, "test", 0)
	ta.waitWorkState(t, first.ID, company.WorkDone)
	second, _ := ta.enqueue(ctx, o, "rui", "What did we decide about returns?", nil, "test", 0)
	if w := ta.waitWorkState(t, second.ID, company.WorkDone); w.Summary != "We decided returns take 30 days." {
		t.Fatalf("Rui did not get the company's memory: %q", w.Summary)
	}
	if found, _ := ta.Companies.Recall(ctx, co, "returns DAYS", 5); len(found) != 1 || found[0].By != "member:"+co+"/bia" {
		t.Fatalf("recall = %+v", found)
	}
}

func TestTheCEOHearsOfTheWeekOnMondays(t *testing.T) {
	ta, co := team(t, weatherAgent)
	ctx := context.Background()
	ta.Companies.SaveWork(ctx, company.Work{ID: "w_1", Company: co, Member: "bia", Request: "x", State: company.WorkDone, Summary: "Shipped the form.", CostUSD: 0.3, Queued: time.Now()})
	o, _ := ta.Companies.Org(ctx, co)
	if d := ta.digest(ctx, o, time.Now().AddDate(0, 0, -7)); !strings.Contains(d, "Bia: 1 done, 0 failed, $0.30") || !strings.Contains(d, "Shipped the form.") {
		t.Fatalf("digest = %q", d)
	}
	zone, _ := time.LoadLocation(o.Zone)
	monday := time.Date(2026, 10, 5, 10, 0, 0, 0, zone)
	for range 2 {
		ta.sendDigests(ctx, monday)
	}
	if got, _ := ta.Events.Get(ctx, digestKey+co); got != "2026-10-05" {
		t.Fatalf("digest sent mark = %q", got)
	}
}

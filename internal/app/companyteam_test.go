package app

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/judge"
	"github.com/turbine-dev/pimpo/internal/llm"
)

// script is a fake agent that answers each prompt with the first rule
// whose key it contains, and remembers every prompt.
type script struct {
	mu      sync.Mutex
	rules   []scriptRule
	prompts []string
}

type scriptRule struct {
	when string
	do   func(r llm.AgentRequest) string
}

func (s *script) Run(_ context.Context, r llm.AgentRequest) (llm.Response, error) {
	s.mu.Lock()
	s.prompts = append(s.prompts, r.Prompt)
	rules := s.rules
	s.mu.Unlock()
	for _, rule := range rules {
		if strings.Contains(r.Prompt, rule.when) {
			return llm.Response{Text: rule.do(r), CostUSD: 0.01}, nil
		}
	}
	return llm.Response{Text: "nothing to do"}, nil
}

func (s *script) saw(sub string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.prompts {
		if strings.Contains(p, sub) {
			return p
		}
	}
	return ""
}

// team is a company where the CEO has Rui, a project manager, and Rui has
// Bia, a developer.
func team(t *testing.T, agent llm.Agent) (*testApp, string) {
	t.Helper()
	ta := newApp(t, agent, &llm.Fake{})
	ta.jobPoll = 10 * time.Millisecond
	// Tasks stay on course unless a test says otherwise.
	ta.DemoJudge = fixedJudge(0.9)
	ta.companiesOn(t)
	_, out := ta.do(t, "POST", "/api/companies", map[string]any{"name": "Pimpo Dev"})
	co := out["id"].(string)
	base := "/api/companies/" + co
	caps := []string{"company.assign", "company.report", "company.ask", "company.answer"}
	ta.do(t, "PUT", base+"/roles/pm", map[string]any{"title": "Project manager", "capabilities": caps})
	ta.do(t, "PUT", base+"/roles/dev", map[string]any{"title": "Developer", "capabilities": caps})
	ta.do(t, "PUT", base+"/members/rui", map[string]any{"name": "Rui", "role": "pm", "reports_to": "ceo"})
	ta.do(t, "PUT", base+"/members/bia", map[string]any{"name": "Bia", "role": "dev", "reports_to": "rui"})
	return ta, co
}

func errorf(t *testing.T, err error) {
	if err != nil {
		t.Errorf("%v", err)
	}
}

func (ta *testApp) waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("waited too long for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWorkGoesDownTheTreeAndReportsBackUp(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{
		{"reported on task", func(r llm.AgentRequest) string { return "Noted Bia's report." }},
		// Bia's handoff names the task above hers, so her rule comes first.
		{"Build the login form", func(r llm.AgentRequest) string {
			task := r.Prompt[strings.Index(r.Prompt, "Task ")+5:]
			task = task[:strings.Index(task, " ")]
			if err := rpc(r.MCPURL, 1, "company_report", map[string]any{"task": task, "status": "done", "summary": "Form built, test passes."}); err != nil {
				t.Errorf("report: %v", err)
			}
			return "Done."
		}},
		{"Ship the login page", func(r llm.AgentRequest) string {
			errorf(t, rpc(r.MCPURL, 1, "company_assign", map[string]any{"assignee": "bia", "title": "Build the login form", "objective": "People sign in with a passkey",
				"acceptance": "A test signs in", "links": []map[string]string{{"kind": "issue", "ref": "#12"}}}))
			return "Handed to Bia."
		}},
	}
	ta, co := team(t, s)
	code, out := ta.do(t, "POST", "/api/companies/"+co+"/tasks", map[string]any{"assignee": "rui", "title": "Ship the login page", "objective": "Sign-in works", "acceptance": "It is live"})
	if code != 200 {
		t.Fatalf("assign: %d %v", code, out)
	}
	root := out["id"].(string)
	ta.waitFor(t, "Rui to hear back", func() bool { return s.saw("reported on task") != "" })
	tasks, _ := ta.Companies.Tasks(context.Background(), co)
	var child company.Task
	for _, x := range tasks {
		if x.Assignee == "bia" {
			child = x
		}
	}
	if child.Parent != root || child.Root != root || child.Depth != 2 || child.State != company.TaskDone || child.Report != "Form built, test passes." {
		t.Fatalf("Bia's task = %+v", child)
	}
	dossier := map[string]bool{}
	for _, l := range child.Dossier {
		dossier[l.Kind+":"+l.Ref] = true
	}
	if !dossier["task:"+root] || !dossier["issue:#12"] {
		t.Fatalf("the dossier lost where it came from: %+v", child.Dossier)
	}
	if p := s.saw("Build the login form"); !strings.Contains(p, "Done when: A test signs in") || !strings.Contains(p, "issue #12") {
		t.Fatalf("Bia was handed %q", p)
	}
}

func TestHandoffsAreRefusedWhenTheyWouldRunAway(t *testing.T) {
	ta, co := team(t, &script{})
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	good := company.Task{Assignee: "bia", Title: "Fix the bug", Objective: "No crash", Acceptance: "The test passes"}
	for _, c := range []struct {
		name string
		from string
		t    company.Task
		want string
	}{
		{"no acceptance", "rui", company.Task{Assignee: "bia", Title: "x", Objective: "y"}, "when it is done"},
		{"up the tree", "bia", company.Task{Assignee: "rui", Title: "x", Objective: "y", Acceptance: "z"}, "below them"},
		{"to itself", "rui", company.Task{Assignee: "rui", Title: "x", Objective: "y", Acceptance: "z"}, "itself"},
		{"to the CEO seat", "rui", company.Task{Assignee: "ceo", Title: "x", Objective: "y", Acceptance: "z"}, "agent"},
	} {
		if _, err := ta.assign(ctx, o, c.from, company.Work{}, c.t); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := ta.assign(ctx, o, "rui", company.Work{}, good); err != nil {
		t.Fatal(err)
	}
	if _, err := ta.assign(ctx, o, "rui", company.Work{}, company.Task{Assignee: "bia", Title: "fix the  BUG", Objective: "x", Acceptance: "y"}); err == nil || !strings.Contains(err.Error(), "already has this task") {
		t.Fatalf("a duplicate: %v", err)
	}
}

func TestAQuestionWaitsAndTheWorkGoesOnWithTheAnswer(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{
		{"The answer: B", func(r llm.AgentRequest) string { return "Went with B." }},
		{"Bia asks you", func(r llm.AgentRequest) string {
			q := r.Prompt[strings.Index(r.Prompt, "(question ")+10:]
			q = q[:strings.Index(q, ")")]
			rpc(r.MCPURL, 1, "company_answer", map[string]any{"question": q, "choice": "B", "reason": "simpler"})
			return "Answered."
		}},
		{"Pick a database", func(r llm.AgentRequest) string {
			rpc(r.MCPURL, 1, "company_ask", map[string]any{"question": "A or B?", "options": []string{"A", "B"}, "recommendation": "B"})
			return "Waiting for Rui."
		}},
	}
	ta, co := team(t, s)
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	w, err := ta.enqueue(ctx, o, "bia", "Pick a database", nil, "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	done := ta.waitWorkState(t, w.ID, company.WorkDone)
	if done.Summary != "Went with B." || len(done.Episodes) != 1 || done.Episodes[0].Answer != "B (simpler)" || done.Episodes[0].Summary != "Waiting for Rui." {
		t.Fatalf("work = %+v", done)
	}
	resumed := s.saw("The answer: B")
	if !strings.Contains(resumed, "Pick a database") || !strings.Contains(resumed, "You noted: Waiting for Rui.") || !strings.Contains(resumed, "You asked: A or B?") {
		t.Fatalf("the work went on with %q", resumed)
	}
	qs, _ := ta.Companies.Questions(ctx, co, false)
	if len(qs) != 1 || qs[0].AnsweredBy != "member:"+co+"/rui" || qs[0].To != "rui" {
		t.Fatalf("questions = %+v", qs)
	}
}

func TestAQuestionToTheCEOWaitsForThem(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{
		{"The answer:", func(r llm.AgentRequest) string { return "Raised the price." }},
		{"Set the price", func(r llm.AgentRequest) string {
			rpc(r.MCPURL, 1, "company_ask", map[string]any{"question": "Raise the price by 10%?", "options": []string{"Yes", "No"}})
			return "Asked."
		}},
	}
	h := newHouse(t)
	h.Agent = s
	h.jobPoll = 10 * time.Millisecond
	ctx := context.Background()
	co := h.owner["company"]
	h.do(t, "PUT", "/api/companies/"+co+"/roles/pm", map[string]any{"title": "PM", "capabilities": []string{"company.ask"}})
	h.do(t, "PUT", "/api/companies/"+co+"/members/rui", map[string]any{"name": "Rui", "role": "pm", "reports_to": "ceo"})
	o, _ := h.Companies.Org(ctx, co)
	w, _ := h.enqueue(ctx, o, "rui", "Set the price", nil, "test", 0)
	h.waitWorkState(t, w.ID, company.WorkWaiting)
	_, needs := h.do(t, "GET", "/api/needs", nil)
	var qid string
	for _, it := range needs["items"].([]any) {
		it := it.(map[string]any)
		if it["kind"] == "company_question" {
			qid = it["id"].(string)
		}
	}
	if qid == "" {
		t.Fatalf("the CEO was not asked: %v", needs)
	}
	// Ana is not a partner; for her the question does not exist.
	if code, _ := h.raw(t, h.ana, "POST", "/api/companies/"+co+"/questions/"+qid+"/answer", js(map[string]string{"choice": "1"})); code != 404 {
		t.Fatalf("Ana answered: %d", code)
	}
	if code, out := h.do(t, "POST", "/api/companies/"+co+"/questions/"+qid+"/answer", map[string]string{"choice": "maybe"}); code != 400 {
		t.Fatalf("an answer that is no option: %d %v", code, out)
	}
	if code, out := h.do(t, "POST", "/api/companies/"+co+"/questions/"+qid+"/answer", map[string]string{"choice": "1"}); code != 200 {
		t.Fatalf("answer: %d %v", code, out)
	}
	if got := h.waitWorkState(t, w.ID, company.WorkDone); got.Episodes[0].Answer != "Yes" {
		t.Fatalf("work = %+v", got)
	}
	if code, _ := h.do(t, "POST", "/api/companies/"+co+"/questions/"+qid+"/answer", map[string]string{"choice": "No"}); code != 400 {
		t.Fatal("a question was answered twice")
	}
}

type fixedJudge float64

func (f fixedJudge) Ask(context.Context, string, any) (judge.Answer, error) {
	return judge.Answer{P: float64(f)}, nil
}

func TestATaskThatStraysWaitsForTheBoss(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{
		{"Grow the newsletter", func(r llm.AgentRequest) string {
			rpc(r.MCPURL, 1, "company_assign", map[string]any{"assignee": "bia", "title": "Rewrite the website in Rust", "objective": "Faster pages", "acceptance": "It builds"})
			return "Handed down."
		}},
	}
	ta, co := team(t, s)
	ta.DemoJudge = fixedJudge(0.1)
	ctx := context.Background()
	_, out := ta.do(t, "POST", "/api/companies/"+co+"/tasks", map[string]any{"assignee": "rui", "title": "Grow the newsletter", "objective": "More readers", "acceptance": "1000 readers"})
	root := out["id"].(string)
	var child company.Task
	ta.waitFor(t, "the stray task", func() bool {
		tasks, _ := ta.Companies.Tasks(ctx, co)
		for _, x := range tasks {
			if x.Parent == root {
				child = x
				return true
			}
		}
		return false
	})
	if !child.Drift {
		t.Fatalf("the stray task was not marked: %+v", child)
	}
	var qs []company.Question
	ta.waitFor(t, "the question to Rui", func() bool { qs, _ = ta.Companies.Questions(ctx, co, true); return len(qs) > 0 })
	if len(qs) != 1 || !qs[0].Drift || qs[0].To != "rui" || qs[0].From != "bia" {
		t.Fatalf("questions = %+v", qs)
	}
	if s.saw("Rewrite the website") != "" && !strings.Contains(s.saw("Rewrite the website"), "Bia asks you") {
		t.Fatal("Bia started the stray task before her boss said so")
	}
	// Once the boss says to start it, it starts.
	if _, err := ta.answerQuestion(ctx, qs[0], "Start it", "fine", "human:owner"); err != nil {
		t.Fatal(err)
	}
	ta.waitFor(t, "Bia's work on the task", func() bool {
		works, _ := ta.Companies.Works(ctx, co, 20)
		return slices.ContainsFunc(works, func(w company.Work) bool { return w.Task == child.ID && w.State == company.WorkDone })
	})
}

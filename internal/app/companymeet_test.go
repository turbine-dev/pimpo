package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/llm"
)

func (ta *testApp) turns(t *testing.T, id string, n int) company.Meeting {
	t.Helper()
	var m company.Meeting
	ta.waitFor(t, "the answers", func() bool {
		m, _ = ta.Companies.Meeting(context.Background(), id)
		return len(m.Transcript) >= n
	})
	ta.waitFor(t, "the room to be free", func() bool {
		if meetLock(id).TryLock() {
			meetLock(id).Unlock()
			return true
		}
		return false
	})
	return m
}

func TestTheCEOTalksWithTheirAgents(t *testing.T) {
	ta, co := team(t, &script{})
	fake := &llm.Fake{Responses: []llm.Response{
		{Text: "The sprint is on track."}, {Text: "Login is half done."},
		{Text: "Login is done by Friday."},
		{Structured: json.RawMessage(`{"minutes":"We reviewed the sprint.","decisions":["Login by Friday"]}`)},
	}}
	ta.LLM = fake
	base := "/api/companies/" + co + "/meetings"
	code, out := ta.do(t, "POST", base, map[string]any{"title": "Monday", "with_ceo": true, "participants": []string{"rui", "bia"}})
	if code != 200 || out["state"] != company.MeetingOpen {
		t.Fatalf("open: %d %v", code, out)
	}
	id := out["id"].(string)
	if code, out := ta.do(t, "POST", base+"/"+id+"/say", map[string]any{"text": "Good morning, how is the sprint?"}); code != 200 {
		t.Fatalf("say: %d %v", code, out)
	}
	m := ta.turns(t, id, 3)
	if m.Transcript[0].Member != company.CEO || m.Transcript[1].Member != "rui" || m.Transcript[2].Member != "bia" {
		t.Fatalf("transcript = %+v", m.Transcript)
	}
	if p := fake.Requests[1].Prompt; !strings.Contains(p, "The sprint is on track.") {
		t.Fatalf("Bia did not hear Rui: %q", p)
	}
	ta.do(t, "POST", base+"/"+id+"/say", map[string]any{"text": "@Bia and the login?"})
	m = ta.turns(t, id, 5)
	if len(m.Transcript) != 5 || m.Transcript[4].Member != "bia" {
		t.Fatalf("only Bia should answer: %+v", m.Transcript)
	}
	if code, out := ta.do(t, "POST", base+"/"+id+"/end", nil); code != 200 || out["minutes"] != "We reviewed the sprint." {
		t.Fatalf("end: %d %v", code, out)
	}
	if notes, _ := ta.Companies.Notes(context.Background(), co, 5); len(notes) != 1 || !strings.Contains(notes[0].Body, "Login by Friday") {
		t.Fatalf("notes = %+v", notes)
	}
	if code, _ := ta.do(t, "POST", base+"/"+id+"/say", map[string]any{"text": "One more thing"}); code != 400 {
		t.Fatalf("talked in an ended meeting: %d", code)
	}
}

func TestTalkingOverAQuestionBeforeAnswering(t *testing.T) {
	ta, co := team(t, &script{})
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	w, _ := ta.enqueue(ctx, o, "rui", "Plan the launch", nil, "test", 0)
	ta.waitWorkState(t, w.ID, company.WorkDone)
	q, err := ta.putQuestion(ctx, o, company.Question{From: "rui", To: company.CEO, Kind: company.Decide, Text: "Launch on Monday or Friday?", Options: []string{"Monday", "Friday"}, Recommendation: "Friday"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &llm.Fake{Responses: []llm.Response{{Text: "Friday gives QA two more days."}}}
	ta.LLM = fake
	_, out := ta.do(t, "POST", "/api/companies/"+co+"/meetings", map[string]any{"title": "The launch", "with_ceo": true, "question": q.ID})
	id := out["id"].(string)
	ta.do(t, "POST", "/api/companies/"+co+"/meetings/"+id+"/say", map[string]any{"text": "Why Friday?"})
	ta.turns(t, id, 2)
	if sys := fake.Requests[0].System; !strings.Contains(sys, "Launch on Monday or Friday?") || !strings.Contains(sys, "Your recommendation: Friday") {
		t.Fatalf("Rui was not told about his question: %q", sys)
	}
	code, out := ta.do(t, "POST", "/api/companies/"+co+"/meetings/"+id+"/task", map[string]any{"member": "rui", "request": "Book the QA room for Thursday"})
	if code != 200 || out["from"] != "meeting:"+id {
		t.Fatalf("a task from the conversation: %d %v", code, out)
	}
	work, _ := ta.Companies.Work(ctx, out["id"].(string))
	if !strings.Contains(string(work.Data), "Why Friday?") {
		t.Fatalf("the task lost its conversation: %s", work.Data)
	}
}

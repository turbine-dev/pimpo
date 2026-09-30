package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/store"
)

type needsReply struct {
	Items  []need         `json:"items"`
	Counts map[string]int `json:"counts"`
	Total  int            `json:"total"`
}

func getNeeds(t *testing.T, ta *testApp, token string) needsReply {
	t.Helper()
	code, body := ta.raw(t, token, "GET", "/api/needs", nil)
	if code != 200 {
		t.Fatalf("needs: %d %s", code, body)
	}
	var out needsReply
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// askAndWait leaves an approval waiting for as long as the test runs.
func askAndWait(t *testing.T, ta *testApp, act policy.Action) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	before := len(ta.Approvals.Open())
	go ta.Approvals.Ask(ctx, act, "ask first")
	for range 200 {
		if len(ta.Approvals.Open()) > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the approval never opened")
}

// Approvals come first, then questions, then what broke, then the rest,
// and within each the newest first.
func TestNeedsMergeAndOrder(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.Approvals.Timeout = time.Hour
	now := time.Now()
	ta.saveQuestions(ctx, []question{
		{ID: "q-old", Question: "Treinou ontem?", Options: []string{"Sim", "Não"}, Asked: now.Add(-2 * time.Hour), Expires: now.Add(time.Hour)},
		{ID: "q-new", Question: "Treinou hoje?", Options: []string{"Sim", "Não"}, Asked: now.Add(-time.Minute), Expires: now.Add(time.Hour)},
		{ID: "q-gone", Question: "Expirou?", Options: []string{"Sim", "Não"}, Asked: now.Add(-48 * time.Hour), Expires: now.Add(-time.Hour)},
	})
	ta.Store.SaveRoutine(ctx, "news", routine.Routine{Name: "Notícias", Code: "async function run() {}"}, "test", "human:owner")
	run, _ := ta.Store.StartRun(ctx, "news", 1)
	ta.Store.FinishRun(ctx, run, store.RunFailed, "the site is down", 0, 0)
	ta.Store.SetRoutineState(ctx, "news", store.RoutineBroken)
	ta.saveJob(ctx, &Job{ID: "plan", Request: "Comparar fornecedores", State: JobPlanned, BudgetUSD: 1, Created: now})
	ta.saveJob(ctx, &Job{ID: "bad", Request: "Resumo do mês", State: JobDone, BudgetUSD: 1, Created: now,
		Parts: []JobPart{{ID: "p1", Title: "Faturas", State: PartDone}, {ID: "p2", Title: "Extratos", State: PartFailed, Error: "no access"}}})
	ta.saveJob(ctx, &Job{ID: "fine", Request: "Tudo certo", State: JobDone, BudgetUSD: 1, Created: now})
	askAndWait(t, ta, policy.Action{Capability: "gmail.send", Risk: 3, Source: "routine:cobranca#1"})

	got := getNeeds(t, ta, "tok")
	var order []string
	for _, n := range got.Items {
		order = append(order, n.Kind+":"+n.ID)
	}
	// The job went wrong after the routine did, so it comes first among failures.
	want := []string{"approval:", "question:q-new", "question:q-old", "job_error:bad", "failed_routine:news", "job_planned:plan"}
	if len(order) != len(want) {
		t.Fatalf("order %v", order)
	}
	for i, w := range want {
		if !strings.HasPrefix(order[i], w) {
			t.Fatalf("item %d is %s, want %s (all: %v)", i, order[i], w, order)
		}
	}
	ap := got.Items[0]
	if ap.Urgency != urgencyDecide || ap.Expires.IsZero() || strings.Join(ap.Actions, ",") != "once,run,always,deny,routine" || ap.Risk != 3 {
		t.Fatalf("approval %+v", ap)
	}
	if q := got.Items[1]; len(q.Options) != 2 || q.Actions[0] != "answer" {
		t.Fatalf("question %+v", q)
	}
	if r := got.Items[4]; r.Detail != "the site is down" || r.Link != "/routines/news" {
		t.Fatalf("routine %+v", r)
	}
	if j := got.Items[3]; !strings.Contains(j.Detail, "Extratos") || j.Link != "/jobs/bad" {
		t.Fatalf("job %+v", j)
	}
	if got.Total != 6 || got.Counts["approval"] != 1 || got.Counts["question"] != 2 || got.Counts["failed_routine"] != 1 ||
		got.Counts["job_error"] != 1 || got.Counts["job_planned"] != 1 {
		t.Fatalf("counts %v total %d", got.Counts, got.Total)
	}

}

// An approval close to its deadline jumps ahead of everything and says when.
func TestNeedsApprovalAboutToExpire(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.Approvals.Timeout = 5 * time.Minute
	now := time.Now()
	ta.saveQuestions(ctx, []question{{ID: "q", Question: "Treinou?", Options: []string{"Sim", "Não"}, Asked: now, Expires: now.Add(time.Hour)}})
	askAndWait(t, ta, policy.Action{Capability: "gmail.label", Risk: 2, Source: "routine:triagem#1"})
	got := getNeeds(t, ta, "tok")
	if len(got.Items) != 2 || got.Items[0].Kind != "approval" || got.Items[0].Urgency != urgencyNow || time.Until(got.Items[0].Expires) > 5*time.Minute {
		t.Fatalf("an approval about to expire is not first: %+v", got.Items)
	}
}

func TestSortNeeds(t *testing.T) {
	now := time.Now()
	list := []need{
		{ID: "ready", Urgency: urgencyWhenFree, Created: now},
		{ID: "old-q", Urgency: urgencyAnswer, Created: now.Add(-time.Hour)},
		{ID: "soon", Urgency: urgencyNow, Created: now.Add(-25 * time.Minute)},
		{ID: "new-q", Urgency: urgencyAnswer, Created: now},
		{ID: "broke", Urgency: urgencyFailure, Created: now.Add(time.Minute)},
	}
	sortNeeds(list)
	var ids []string
	for _, n := range list {
		ids = append(ids, n.ID)
	}
	if strings.Join(ids, ",") != "soon,new-q,old-q,broke,ready" {
		t.Fatalf("sorted %v", ids)
	}
}

// Each person sees only their own: the owner none of Ana's approvals,
// questions or broken routines, and Ana none of the owner's.
func TestNeedsArePerPerson(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	h.Approvals.Timeout = time.Hour
	now := time.Now()
	h.saveQuestions(ctx, []question{
		{ID: "q-owner", Question: h.ownerMark + " treinou?", Options: []string{"Sim", "Não"}, Asked: now, Expires: now.Add(time.Hour)},
		{ID: "q-ana", Person: h.anaID, Question: h.anaMark + " treinou?", Options: []string{"Sim", "Não"}, Asked: now, Expires: now.Add(time.Hour)},
	})
	h.Store.SetRoutineState(ctx, h.owner["routine"], store.RoutineBroken)
	h.Store.SetRoutineState(ctx, h.anas["routine"], store.RoutineBroken)
	askAndWait(t, h.testApp, policy.Action{Capability: "gmail.send", Risk: 3, Source: "routine:" + h.owner["routine"] + "#1"})
	askAndWait(t, h.testApp, policy.Action{Capability: "gmail.send", Risk: 3, Source: "routine:" + h.anas["routine"] + "#1", Person: h.anaID, Role: "member"})

	for _, c := range []struct {
		who, token, mine, theirs string
		ids                      map[string]string
	}{
		{"owner", "tok", h.ownerMark, h.anaMark, h.owner},
		{"ana", h.ana, h.anaMark, h.ownerMark, h.anas},
	} {
		_, body := h.raw(t, c.token, "GET", "/api/needs", nil)
		if strings.Contains(body, c.theirs) {
			t.Errorf("%s sees someone else's needs: %.400s", c.who, body)
		}
		got := getNeeds(t, h.testApp, c.token)
		if got.Counts["approval"] != 1 || got.Counts["question"] != 1 || got.Counts["failed_routine"] != 1 || got.Counts["job_planned"] != 1 {
			t.Errorf("%s counts %v", c.who, got.Counts)
		}
		for _, n := range got.Items {
			if n.Kind == "failed_routine" && n.ID != c.ids["routine"] {
				t.Errorf("%s sees routine %s", c.who, n.ID)
			}
		}
	}
	// A member may not make a lasting rule, so the list does not offer it.
	for _, n := range getNeeds(t, h.testApp, h.ana).Items {
		if n.Kind == "approval" && strings.Contains(strings.Join(n.Actions, ","), "always") {
			t.Errorf("ana is offered always: %v", n.Actions)
		}
	}
	// The owner's own list carries the house's suggestions kind; Ana's never does.
	if _, ok := getNeeds(t, h.testApp, h.ana).Counts["suggestion"]; ok {
		t.Error("a member sees the owner's suggestions")
	}
}

// A routine's approval may be granted for the routine, with the amount it
// moves as the least limit; one that always asks is not offered it. Every
// question takes a typed answer too.
func TestNeedsGrantsAndTypedAnswers(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.Approvals.Timeout = time.Hour
	now := time.Now()
	ta.saveQuestions(ctx, []question{{ID: "q", Question: "Treinou?", Options: []string{"Sim", "Não"}, Asked: now, Expires: now.Add(time.Hour)}})
	askAndWait(t, ta, policy.Action{Capability: "shop.pay", Risk: 3, Source: "routine:contas#2", Args: map[string]any{"amount": 80.0, "url": "https://pay.example/1"}})
	askAndWait(t, ta, policy.Action{Capability: "whatsapp.send_to", Risk: 3, Source: "routine:contas#2", Args: map[string]any{"to": "+5511"}})
	var pay, send need
	for _, n := range getNeeds(t, ta, "tok").Items {
		switch {
		case n.Kind == "approval" && n.Amount != nil:
			pay = n
		case n.Kind == "approval":
			send = n
		case n.Kind == "question":
			if strings.Join(n.Actions, ",") != "answer,type" {
				t.Errorf("question actions %v", n.Actions)
			}
		}
	}
	if !slices.Contains(pay.Actions, "routine") || *pay.Amount != 80 {
		t.Fatalf("the payment is not grantable up to 80: %+v", pay)
	}
	if send.ID == "" || slices.Contains(send.Actions, "routine") || send.Amount != nil {
		t.Fatalf("an always-ask approval is offered a grant: %+v", send)
	}
}

// Key requests and lessons come from their own pages' scoped lists: each
// person sees only their own, a request ranks with approvals, lessons are
// one item with their count, and a guest sees neither.
func TestNeedsKeysAndLessonsArePerPerson(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	now := time.Now()
	h.saveCredentialRequests(ctx, []credentialRequest{
		{ID: "c-owner", Person: people.OwnerID, Connector: "notion", Field: "token", Routine: h.owner["routine"], Asked: now, Expires: now.Add(time.Hour)},
		{ID: "c-ana", Person: h.anaID, Connector: "notion", Field: "token", Routine: h.anas["routine"], Asked: now, Expires: now.Add(time.Hour)},
		{ID: "c-gone", Person: h.anaID, Connector: "notion", Field: "token", Asked: now.Add(-48 * time.Hour), Expires: now.Add(-time.Hour)},
	})
	h.Memory.AddFor("ANALESSON curto", learnTopic, "aprendido: x", memory.Learned, h.anaID)
	h.Memory.AddFor("ANALESSON tópicos", learnTopic, "aprendido: z", memory.Learned, h.anaID)
	h.Memory.AddFor("OWNERLESSON longo", learnTopic, "aprendido: y", memory.Learned, "")

	for _, c := range []struct {
		who, token, key, lesson, theirs string
	}{
		{"owner", "tok", "c-owner", "OWNERLESSON", "ANALESSON"},
		{"ana", h.ana, "c-ana", "ANALESSON", "OWNERLESSON"},
	} {
		_, body := h.raw(t, c.token, "GET", "/api/needs", nil)
		if strings.Contains(body, c.theirs) {
			t.Errorf("%s sees someone else's lessons: %.400s", c.who, body)
		}
		got := getNeeds(t, h.testApp, c.token)
		var key, lesson need
		for _, n := range got.Items {
			switch n.Kind {
			case "credential_request":
				key = n
			case "lesson":
				lesson = n
			}
		}
		if got.Counts["credential_request"] != 1 || key.ID != c.key || key.Link != "/credentials/"+c.key || key.Urgency != urgencyDecide || key.Title == "" {
			t.Errorf("%s key request %+v (counts %v)", c.who, key, got.Counts)
		}
		want := proposedCount(t, h, c.token)
		if got.Counts["lesson"] != 1 || lesson.Count != want || want == 0 || lesson.Link != "/lessons" || lesson.Urgency != urgencyWhenFree || !strings.Contains(body, c.lesson) {
			t.Errorf("%s lessons %+v, want count %d", c.who, lesson, want)
		}
	}

	// A guest keeps no keys and reviews no lessons.
	if code, _ := h.do(t, "PUT", "/api/people/"+h.anaID, map[string]string{"role": "guest"}); code != 200 {
		t.Fatal(code)
	}
	got := getNeeds(t, h.testApp, h.ana)
	for _, kind := range []string{"credential_request", "lesson", "approval"} {
		if _, ok := got.Counts[kind]; ok {
			t.Errorf("a guest's list has %s: %v", kind, got.Counts)
		}
	}
}

// proposedCount is how many lessons the person's own page lists.
func proposedCount(t *testing.T, h *house, token string) int {
	_, body := h.raw(t, token, "GET", "/api/lessons", nil)
	var out struct {
		Proposed []lesson `json:"proposed"`
	}
	json.Unmarshal([]byte(body), &out)
	return len(out.Proposed)
}

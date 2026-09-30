package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/store"
	"github.com/turbine-dev/pimpo/internal/trace"
)

func lessonOf(ls []lesson, kind, from string) (lesson, bool) {
	for _, l := range ls {
		if l.Kind == kind && l.From == from {
			return l, true
		}
	}
	return lesson{}, false
}

func readyExploration(t *testing.T, ta *testApp, id, request, person, routineID string, at time.Time) {
	t.Helper()
	err := ta.Store.SaveExploration(context.Background(), store.Exploration{ID: id, Request: request, State: store.ExplorationReady, Person: person, Routine: routineID,
		Summary: "feito: " + request, Trace: &trace.Trace{ID: id, Request: request}, CreatedAt: at, UpdatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
}

// Every producer proposes a lesson, and none of them changes anything.
func TestLessonsAreProposedNeverApplied(t *testing.T) {
	fake := &llm.Fake{Responses: []llm.Response{{Structured: json.RawMessage(`{"preferences":[{"text":"Respostas curtas, em tópicos.","evidence":"pediu curto 3 vezes","requests":[0,2]}]}`)}}}
	ta := newApp(t, weatherAgent, fake)
	ctx := context.Background()
	for i, r := range []string{"me dá um resumo curto do dia", "resumo curto das notícias", "de novo curto, por favor"} {
		ta.Events.Append(ctx, "exploration.started", "human:owner", map[string]string{"exploration": "ex" + string(rune('a'+i)), "request": r})
	}
	if len(ta.learn(ctx)) != 1 {
		t.Fatal("nothing learned")
	}
	// A note the agent took during a task, and a fact imported from
	// another assistant, which Pimpo did not notice.
	note, _ := ta.Memory.AddFor("O chefe do dono é o Carlos.", "trabalho", "exploration:e9", memory.Low, "")
	ta.Memory.AddFor("Mora em Lisboa.", "sobre mim", "openclaw:USER.md", memory.Low, "")
	// The same task asked twice, one asked once, and a repair ready.
	now := time.Now()
	readyExploration(t, ta, "r1", "Todo dia me mande o clima de Lisboa.", "", "", now.Add(-2*time.Hour))
	readyExploration(t, ta, "r2", "todo dia me mande o clima de lisboa", "", "", now.Add(-time.Hour))
	readyExploration(t, ta, "r3", "Quanto custa um café?", "", "", now)
	ta.Store.SaveRoutine(ctx, "clima", routine.Routine{Name: "Clima", Code: "async function run() {}"}, "test", "human:owner")
	run, _ := ta.Store.StartRun(ctx, "clima", 1)
	ta.Store.FinishRun(ctx, run, store.RunFailed, "o site mudou", 0, 0)
	readyExploration(t, ta, "fix1", "Todo dia me mande o clima", "", "clima", now)
	ta.saveSuggestions(ctx, []suggestion{{ID: "s1", Title: "Conta de luz", Why: "Chega todo mês.", Request: "Todo mês me avise da conta de luz.", Made: now, Expires: now.Add(time.Hour)}})

	ls := ta.proposedLessons(ctx)
	if len(ls) != 5 {
		t.Fatalf("%d lessons: %+v", len(ls), ls)
	}
	pref, ok := lessonOf(ls, lessonPreference, "learned")
	if !ok || pref.Change != "Respostas curtas, em tópicos." || pref.State != lessonProposed || len(pref.Evidence) != 3 || pref.Evidence[1].To != "/explorations/exa" || pref.Evidence[2].To != "/explorations/exc" {
		t.Fatalf("preference %+v", pref)
	}
	if f, ok := lessonOf(ls, lessonFact, "note"); !ok || f.Ref != note.ID || f.Evidence[0].To != "/explorations/e9" {
		t.Fatalf("note %+v", f)
	}
	if r, ok := lessonOf(ls, lessonRoutine, "repeated"); !ok || r.Ref != "r2" || len(r.Evidence) != 2 {
		t.Fatalf("repeated %+v", r)
	}
	if f, ok := lessonOf(ls, lessonFix, "repair"); !ok || f.Routine != "clima" || f.Detail != "o site mudou" || f.Ref != "fix1" {
		t.Fatalf("fix %+v", f)
	}
	if s, ok := lessonOf(ls, lessonRoutine, "suggestion"); !ok || s.Ref != "s1" {
		t.Fatalf("suggestion %+v", s)
	}
	// Nothing was applied: the facts are unconfirmed, no routine was made,
	// the explorations still wait.
	facts, _ := ta.Memory.InstructionsFor("")
	if len(facts) != 0 {
		t.Fatalf("confirmed without accepting: %+v", facts)
	}
	if list, _ := ta.Store.Routines(ctx); len(list) != 1 {
		t.Fatalf("routines %d", len(list))
	}
	if e, _ := ta.Store.Exploration(ctx, "fix1"); e.State != store.ExplorationReady {
		t.Fatalf("repair applied: %s", e.State)
	}
	// The same through the API, with the count the menu shows.
	code, out := ta.do(t, "GET", "/api/lessons", nil)
	if code != 200 || len(out["proposed"].([]any)) != 5 {
		t.Fatalf("GET %d %v", code, out)
	}
}

func TestAcceptEditAndRejectLessons(t *testing.T) {
	fake := &llm.Fake{Responses: []llm.Response{{Structured: json.RawMessage(greeting), CostUSD: 0.05}}}
	ta := newApp(t, weatherAgent, fake)
	ctx := context.Background()
	for range 2 {
		ta.do(t, "POST", "/api/explorations", map[string]string{"request": "Todo dia às 7h me manda bom dia"})
		ta.Explore.Wait()
	}
	pref, _ := ta.Memory.AddFor("Respostas curtas, em tópicos.", learnTopic, "aprendido: x", memory.Learned, "")
	note, _ := ta.Memory.AddFor("O chefe é o Carlos.", "trabalho", "exploration:e1", memory.Low, "")
	other, _ := ta.Memory.AddFor("Prefere e-mail a telefone.", learnTopic, "aprendido: y", memory.Learned, "")
	ls := ta.proposedLessons(ctx)
	byRef := map[string]lesson{}
	for _, l := range ls {
		byRef[l.Ref] = l
	}
	rt, ok := lessonOf(ls, lessonRoutine, "repeated")
	if !ok {
		t.Fatalf("no routine lesson in %+v", ls)
	}

	// Accept compiles through the usual path.
	code, out := ta.do(t, "POST", "/api/lessons/"+rt.ID+"/accept", nil)
	if code != 200 || out["state"] != lessonAccepted || out["result"] != "bom-dia" {
		t.Fatalf("accept %d %v", code, out)
	}
	if evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"routine.created"}}); len(evs) != 1 {
		t.Fatal("not compiled through the compiler")
	}
	// Accept confirms a preference.
	if code, _ := ta.do(t, "POST", "/api/lessons/"+byRef[pref.ID].ID+"/accept", nil); code != 200 {
		t.Fatal(code)
	}
	if f, _ := ta.Memory.Get(pref.ID); f.Trust != memory.High {
		t.Fatalf("not confirmed: %+v", f)
	}
	// Edit keeps the person's words, confirmed, in place of the note.
	code, out = ta.do(t, "POST", "/api/lessons/"+byRef[note.ID].ID+"/edit", map[string]string{"text": "O chefe é o Carlos Souza."})
	if code != 200 || out["state"] != lessonEdited {
		t.Fatalf("edit %d %v", code, out)
	}
	if _, ok := ta.Memory.Get(note.ID); ok {
		t.Fatal("the note is still there")
	}
	if f, ok := ta.Memory.Get(out["result"].(string)); !ok || f.Text != "O chefe é o Carlos Souza." || f.Trust != memory.High {
		t.Fatalf("edited %+v", f)
	}
	// Reject removes it, and it is never learned again, however worded.
	rej := byRef[other.ID]
	if code, _ := ta.do(t, "POST", "/api/lessons/"+rej.ID+"/reject", nil); code != 200 {
		t.Fatal(code)
	}
	if _, ok := ta.Memory.Get(other.ID); ok {
		t.Fatal("rejected preference kept")
	}
	if code, _ := ta.do(t, "POST", "/api/lessons/"+rej.ID+"/accept", nil); code != 404 {
		t.Fatalf("a decided lesson answered again: %d", code)
	}
	for _, r := range []string{"resumo curto", "resumo curto de novo", "curto, por favor"} {
		ta.Events.Append(ctx, "exploration.started", "human:owner", map[string]string{"request": r})
	}
	fake.Responses = []llm.Response{{Structured: json.RawMessage(`{"preferences":[{"text":"prefere e-mail a telefone","evidence":"de novo"}]}`)}}
	if again := ta.learn(ctx); len(again) != 0 {
		t.Fatalf("relearned a rejected lesson: %+v", again)
	}
	_, out = ta.do(t, "GET", "/api/lessons", nil)
	if len(out["proposed"].([]any)) != 0 || len(out["decided"].([]any)) != 4 {
		t.Fatalf("lessons %v", out)
	}
	// A routine lesson rejected does not come back when asked once more.
	ta.do(t, "POST", "/api/explorations", map[string]string{"request": "Me diga o clima"})
	ta.Explore.Wait()
	ta.do(t, "POST", "/api/explorations", map[string]string{"request": "me diga o clima!"})
	ta.Explore.Wait()
	clima, ok := lessonOf(ta.proposedLessons(ctx), lessonRoutine, "repeated")
	if !ok {
		t.Fatal("no lesson for the repeated task")
	}
	ta.do(t, "POST", "/api/lessons/"+clima.ID+"/reject", nil)
	ta.do(t, "POST", "/api/explorations", map[string]string{"request": "Me diga o clima."})
	ta.Explore.Wait()
	if _, ok := lessonOf(ta.proposedLessons(ctx), lessonRoutine, "repeated"); ok {
		t.Fatal("a rejected routine lesson came back")
	}
}

func TestRejectingARepairDiscardsIt(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.Store.SaveRoutine(ctx, "clima", routine.Routine{Name: "Clima", Code: "async function run() {}"}, "test", "human:owner")
	readyExploration(t, ta, "fix1", "Todo dia me mande o clima", "", "clima", time.Now())
	fix, ok := lessonOf(ta.proposedLessons(ctx), lessonFix, "repair")
	if !ok {
		t.Fatal("no fix lesson")
	}
	if _, err := ta.decideLesson(ctx, fix.ID, "reject", ""); err != nil {
		t.Fatal(err)
	}
	if e, _ := ta.Store.Exploration(ctx, "fix1"); e.State != store.ExplorationDiscarded {
		t.Fatalf("repair %s", e.State)
	}
	if r, _ := ta.Store.Routine(ctx, "clima"); r.Version != 1 {
		t.Fatalf("routine changed: v%d", r.Version)
	}
}

// Each person sees and decides only their own lessons.
func TestLessonsArePerPerson(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	h.Memory.AddFor("ANAPREF curto", learnTopic, "aprendido: x", memory.Learned, h.anaID)
	h.Memory.AddFor("OWNERPREF longo", learnTopic, "aprendido: y", memory.Learned, "")
	h.Memory.AddFor("CASAPREF", learnTopic, "aprendido: z", memory.Learned, people.Household)
	h.saveSuggestions(ctx, []suggestion{{ID: "s1", Title: "OWNERSUGGESTION", Request: "x", Made: time.Now(), Expires: time.Now().Add(time.Hour)}})
	_, anas := h.raw(t, h.ana, "GET", "/api/lessons", nil)
	if !strings.Contains(anas, "ANAPREF") || strings.Contains(anas, "OWNERPREF") || strings.Contains(anas, "OWNERSUGGESTION") || strings.Contains(anas, "CASAPREF") {
		t.Fatalf("Ana's lessons: %s", anas)
	}
	_, owners := h.raw(t, "tok", "GET", "/api/lessons", nil)
	if strings.Contains(owners, "ANAPREF") || !strings.Contains(owners, "OWNERPREF") {
		t.Fatalf("owner's lessons: %s", owners)
	}
	ownerLesson, _ := lessonOf(h.proposedLessons(ctx), lessonPreference, "learned")
	if code, _ := h.raw(t, h.ana, "POST", "/api/lessons/"+ownerLesson.ID+"/reject", nil); code != 404 {
		t.Fatalf("Ana decided the owner's lesson: %d", code)
	}
	anaLesson, _ := lessonOf(h.proposedLessons(people.With(ctx, h.anaID)), lessonPreference, "learned")
	if code, _ := h.raw(t, "tok", "POST", "/api/lessons/"+anaLesson.ID+"/accept", nil); code != 404 {
		t.Fatalf("the owner decided Ana's lesson: %d", code)
	}
	if code, body := h.raw(t, h.ana, "POST", "/api/lessons/"+anaLesson.ID+"/accept", nil); code != 200 {
		t.Fatalf("Ana could not accept hers: %d %s", code, body)
	}
	facts, _ := h.Memory.InstructionsFor(h.anaID)
	if len(facts) == 0 || !strings.Contains(facts[len(facts)-1].Text+facts[0].Text, "ANAPREF") {
		t.Fatalf("Ana's facts %+v", facts)
	}
}

func TestTheWeeklyDigestOnlyLinks(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	s := ta.Settings(ctx)
	s.Zone = "UTC"
	ta.SaveSettings(ctx, s, "test")
	day := time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC)
	if n := ta.lessonDigests(ctx, day); n != 0 {
		t.Fatal("a digest with nothing to review")
	}
	ta.Memory.AddFor("Respostas curtas.", learnTopic, "aprendido: x", memory.Learned, "")
	if n := ta.lessonDigests(ctx, day.Add(-3*time.Hour)); n != 0 {
		t.Fatal("sent before ten")
	}
	if n := ta.lessonDigests(ctx, day); n != 1 {
		t.Fatalf("sent %d", n)
	}
	if n := ta.lessonDigests(ctx, day.Add(24*time.Hour)); n != 0 {
		t.Fatal("sent twice in a week")
	}
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"notice.sent"}})
	var d struct {
		Text    string
		Actions []any
	}
	evs[len(evs)-1].Decode(&d)
	if len(d.Actions) != 0 || !strings.Contains(d.Text, "1") {
		t.Fatalf("digest %+v", d)
	}
	s.LessonDigestOff = true
	ta.SaveSettings(ctx, s, "test")
	if n := ta.lessonDigests(ctx, day.Add(8*24*time.Hour)); n != 0 {
		t.Fatal("sent while off")
	}
}

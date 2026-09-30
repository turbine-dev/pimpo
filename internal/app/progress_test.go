package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/turbine-dev/pimpo/internal/chatlink"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/store"
)

func waitProgress(t *testing.T, ta *testApp, id string, ok func(store.Progress) bool) store.Progress {
	t.Helper()
	var p store.Progress
	for i := 0; i < 300; i++ {
		var err error
		if p, err = ta.Store.Progress(context.Background(), id); err == nil && ok(p) {
			return p
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("progress %s is %+v", id, p)
	return p
}

// A job's progress is kept as it goes, and a restart shows it picked up
// again; a routine run a restart cut short says so instead of running
// forever.
func TestProgressIsKeptAndRestored(t *testing.T) {
	pa := &partAgent{}
	ta := newApp(t, pa.agent(), &llm.Fake{Responses: []llm.Response{{Structured: json.RawMessage(`{"report":"ok"}`)}}})
	ta.jobPoll = 10 * time.Millisecond
	ctx := context.Background()
	before := ta.startedAt.Add(-time.Minute)

	j := Job{ID: "j1", Request: "Duas partes", State: JobRunning, BudgetUSD: 1, Created: before, Parts: []JobPart{
		{ID: "p1", Title: "Feita", State: PartDone, Summary: "ok", Attempts: 1},
		{ID: "p2", Title: "Interrompida", State: PartRunning, Attempts: 1, Started: before},
	}}
	ta.saveJob(ctx, &j)
	p, err := ta.Store.Progress(ctx, "job:j1")
	if err != nil || p.State != store.ProgressRunning || p.Done != 1 || p.Total != 2 || p.Label != "Interrompida" || p.Kind != "job" || p.Person != people.OwnerID {
		t.Fatalf("%v %+v", err, p)
	}

	// A run that was going when Pimpo stopped.
	ta.Store.SaveRoutine(ctx, "brief", routine.Routine{Name: "Resumo", Code: "async function run() {}", Manifest: runtime.Manifest{Capabilities: []string{"telegram.send"}}}, "test", "human:owner")
	runID, _ := ta.Store.StartRun(ctx, "brief", 1)
	old := store.Progress{ID: "run:brief:" + itoa(runID), Person: people.OwnerID, Kind: "run", Routine: "brief", Run: runID, Title: "Resumo", Label: "gmail.search", Done: 3,
		State: store.ProgressRunning, StartedAt: before, UpdatedAt: before}
	ta.Store.SaveProgress(ctx, old)

	ta.settleProgress(ctx)
	ta.resumeJobs(ctx)
	got := waitProgress(t, ta, old.ID, func(p store.Progress) bool { return p.Final() })
	if got.State != store.ProgressFailed || got.Phase != "interrupted" || got.Done != 3 {
		t.Fatalf("%+v", got)
	}
	if runs, _ := ta.Store.Runs(ctx, "brief", 1); runs[0].Outcome != store.RunFailed {
		t.Fatalf("the interrupted run stayed %s", runs[0].Outcome)
	}
	done := waitProgress(t, ta, "job:j1", func(p store.Progress) bool { return p.Final() })
	if done.State != store.ProgressDone || done.Done != 2 || !done.Resumed {
		t.Fatalf("%+v", done)
	}
	if active, _ := ta.Store.ActiveProgress(ctx, people.OwnerID); len(active) != 0 {
		t.Fatalf("still running: %+v", active)
	}

	// A routine run keeps its record from start to end.
	if _, err := ta.Scheduler.RunNow(ctx, "brief", "owner"); err != nil {
		t.Fatal(err)
	}
	runs, _ := ta.Store.Runs(ctx, "brief", 1)
	r, err := ta.Store.Progress(ctx, "run:brief:"+itoa(runs[0].ID))
	if err != nil || r.State != store.ProgressDone || r.Title != "Resumo" || r.Routine != "brief" {
		t.Fatalf("%v %+v", err, r)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

// A person sees only their own progress, on the API and on the stream,
// where someone else's is only its type.
func TestProgressIsEachPersonsOwn(t *testing.T) {
	h := newHouse(t)
	ctx := context.Background()
	c, _, err := websocket.Dial(ctx, strings.Replace(h.srv.URL, "http", "ws", 1)+"/api/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + h.ana}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	seen := make(chan event.Event, 64)
	go func() {
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			var e event.Event
			json.Unmarshal(b, &e)
			if e.Type == EventProgress {
				seen <- e
			}
		}
	}()
	time.Sleep(100 * time.Millisecond) // the subscription starts with the handler
	for _, rid := range []string{h.owner["routine"], h.anas["routine"]} {
		h.Scheduler.RunNow(ctx, rid, "owner") // fails, and says so in its progress
	}
	for _, c := range []struct{ token, mine, other string }{{h.ana, h.anaMark, h.ownerMark}, {"tok", h.ownerMark, h.anaMark}} {
		code, body := h.raw(t, c.token, "GET", "/api/progress", nil)
		if code != 200 || !strings.Contains(body, c.mine) || strings.Contains(body, c.other) {
			t.Fatalf("%d %.400s", code, body)
		}
	}
	whole, bare := 0, 0
	deadline := time.After(3 * time.Second)
	for whole < 2 || bare < 2 {
		select {
		case e := <-seen:
			if strings.Contains(string(e.Data), h.ownerMark) {
				t.Fatalf("Ana saw the owner's progress: %s", e.Data)
			}
			if strings.Contains(string(e.Data), h.anaMark) {
				whole++
			} else if string(e.Data) == "{}" {
				bare++
			}
		case <-deadline:
			t.Fatalf("Ana saw %d of her own and %d bare progress events", whole, bare)
		}
	}
}

// fakeChannels is Telegram, Discord and Slack, counting what is sent and
// edited.
type fakeChannels struct {
	mu    sync.Mutex
	sent  map[string]int
	edits map[string]int
	last  map[string]string
}

func (f *fakeChannels) note(kind string, edit bool, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if edit {
		f.edits[kind]++
	} else {
		f.sent[kind]++
	}
	f.last[kind] = text
}

func (f *fakeChannels) count(kind string) (int, int, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[kind], f.edits[kind], f.last[kind]
}

func (f *fakeChannels) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	json.Unmarshal(raw, &body)
	text := func(k string) string { s, _ := body[k].(string); return s }
	switch {
	case strings.HasSuffix(r.URL.Path, "/sendMessage"):
		f.note("telegram", false, text("text"))
		w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	case strings.HasSuffix(r.URL.Path, "/editMessageText"):
		f.note("telegram", true, text("text"))
		w.Write([]byte(`{"ok":true,"result":{}}`))
	case r.URL.Path == "/users/@me/channels":
		w.Write([]byte(`{"id":"dm1"}`))
	case r.URL.Path == "/channels/dm1/messages" && r.Method == "POST":
		f.note("discord", false, text("content"))
		w.Write([]byte(`{"id":"m1"}`))
	case r.URL.Path == "/channels/dm1/messages/m1" && r.Method == "PATCH":
		f.note("discord", true, text("content"))
		w.Write([]byte(`{"id":"m1"}`))
	case r.URL.Path == "/conversations.open":
		w.Write([]byte(`{"ok":true,"channel":{"id":"D1"}}`))
	case r.URL.Path == "/chat.postMessage":
		f.note("slack", false, text("text"))
		w.Write([]byte(`{"ok":true,"ts":"1.1"}`))
	case r.URL.Path == "/chat.update":
		if text("ts") != "1.1" || text("channel") != "D1" {
			http.Error(w, "wrong message", 400)
			return
		}
		f.note("slack", true, text("text"))
		w.Write([]byte(`{"ok":true}`))
	default:
		http.NotFound(w, r)
	}
}

// A followed job keeps one message per channel, edited in place no more
// often than the interval, and ends showing the result.
func TestFollowedJobEditsOneMessageThrottled(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	fc := &fakeChannels{sent: map[string]int{}, edits: map[string]int{}, last: map[string]string{}}
	srv := httptest.NewServer(fc)
	defer srv.Close()
	ta.Vault.Set(ctx, "telegram.token", "1:t")
	ta.Events.Put(ctx, "telegram.chat", "100")
	ta.TelegramAPI = srv.URL
	linksMu.Lock()
	ta.links = map[string]*linkRun{
		"discord": {link: &chatlink.Discord{Token: "d", API: srv.URL}, cancel: func() {}},
		"slack":   {link: &chatlink.Slack{BotToken: "b", AppToken: "a", API: srv.URL}, cancel: func() {}},
	}
	linksMu.Unlock()
	ta.Events.Put(ctx, linkOwnerKey("discord"), "u1")
	ta.Events.Put(ctx, linkOwnerKey("slack"), "U1")
	ta.ProgressEvery = 300 * time.Millisecond

	parts := make([]JobPart, 30)
	for i := range parts {
		parts[i] = JobPart{ID: itoa(int64(i)), Title: "Parte", State: PartWaiting}
	}
	j := Job{ID: "jf", Request: "Muitas partes", State: JobRunning, BudgetUSD: 1, Follow: true, Parts: parts, Created: time.Now()}
	ta.saveJob(ctx, &j)
	start := time.Now()
	for i := range parts {
		ta.updateJob(ctx, "jf", func(j *Job) { j.Parts[i].State = PartDone })
		time.Sleep(20 * time.Millisecond)
	}
	ta.updateJob(ctx, "jf", func(j *Job) { j.State, j.Report = JobDone, "ok" })
	elapsed := time.Since(start)
	for _, kind := range []string{"telegram", "discord", "slack"} {
		var sent, edits int
		var last string
		for i := 0; i < 200; i++ {
			if sent, edits, last = fc.count(kind); strings.HasPrefix(last, "✅") {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		// One message, edited at most once per interval plus the end.
		most := int(elapsed/ta.ProgressEvery) + 2
		if sent != 1 || edits < 1 || edits > most || !strings.Contains(last, "30") {
			t.Fatalf("%s: %d sent, %d edits (at most %d), last %q", kind, sent, edits, most, last)
		}
	}
}

// Where the service cannot edit, a followed job sends only its start;
// the notice at the end tells the rest.
func TestFollowWithoutEditingSendsOnlyTheStart(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	var mu sync.Mutex
	var sent []string
	ta.ProgressEvery = 10 * time.Millisecond
	ta.ProgressSinks = func(context.Context, string) []progressSink {
		return []progressSink{{name: "signal", send: func(_ context.Context, text string) (string, error) {
			mu.Lock()
			sent = append(sent, text)
			mu.Unlock()
			return "", nil
		}}}
	}
	j := Job{ID: "js", Request: "Sem edição", State: JobRunning, BudgetUSD: 1, Follow: true, Parts: []JobPart{{ID: "p1", Title: "A", State: PartRunning}}, Created: time.Now()}
	ta.saveJob(ctx, &j)
	time.Sleep(50 * time.Millisecond)
	ta.updateJob(ctx, "js", func(j *Job) { j.Parts[0].State = PartDone })
	time.Sleep(50 * time.Millisecond)
	ta.updateJob(ctx, "js", func(j *Job) { j.State = JobDone })
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "⏳") {
		t.Fatalf("%q", sent)
	}
}

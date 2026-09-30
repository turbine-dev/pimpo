package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/llm"
	ownerpkg "github.com/turbine-dev/pimpo/internal/owner"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/store"
)

const pastedKey = "ghp_R4nd0mT0k3nV4lu3ThatNobodyMayS33x9"

// nowhere fails the test when value shows up in any event of the log.
func nowhere(t *testing.T, ta *testApp, value string) {
	t.Helper()
	evs, _ := ta.Events.List(context.Background(), event.Query{Limit: 10000})
	for _, e := range evs {
		if strings.Contains(string(e.Data), value) {
			t.Fatalf("the value is in a %s event: %s", e.Type, e.Data)
		}
	}
}

func noticesTo(t *testing.T, ta *testApp, person string) []string {
	t.Helper()
	evs, _ := ta.Events.List(context.Background(), event.Query{Types: []string{"notice.sent"}})
	var out []string
	for _, e := range evs {
		var n struct{ Text, To string }
		e.Decode(&n)
		if people.Norm(n.To) == person && strings.Contains(n.Text, "/credentials/") {
			out = append(out, n.Text)
		}
	}
	return out
}

// A routine that needs a key its person never gave fails saying Pimpo
// asked for it privately; the person gets one notice with the form's
// link, the form writes the key to the vault, and the answer and the log
// say only that it was saved.
func TestAMissingKeyIsAskedForPrivately(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	r := routine.Routine{Name: "Issues", Manifest: runtime.Manifest{Schedule: "0 9 * * *", Capabilities: []string{"github.issues"}},
		Code: `async function run() { await github.issues({repo: "a/b"}) }`}
	ta.Store.SaveRoutine(ctx, "issues", r, "test", "human:owner")
	_, err := ta.Scheduler.RunNow(ctx, "issues", "owner")
	if err == nil || !strings.Contains(err.Error(), "asked the person privately") {
		t.Fatalf("run error: %v", err)
	}
	ta.Scheduler.RunNow(ctx, "issues", "owner")
	_, out := ta.do(t, "GET", "/api/credentials", nil)
	list := out["list"].([]any)
	if len(list) != 1 {
		t.Fatalf("requests: %v", out)
	}
	req := list[0].(map[string]any)
	if req["connector"] != "github" || req["field"] != "token" || req["routine"] != "issues" || req["title"] != "GitHub" || req["description"] == "" {
		t.Fatalf("request: %v", req)
	}
	id := req["id"].(string)
	if n := noticesTo(t, ta, people.OwnerID); len(n) != 1 || !strings.Contains(n[0], "/credentials/"+id) {
		t.Fatalf("notices: %v", n)
	}

	code, out := ta.do(t, "POST", "/api/credentials/"+id, map[string]string{"value": " " + pastedKey + " "})
	if code != 200 || out["saved"] != true || out["routine"] != "issues" {
		t.Fatalf("save %d %v", code, out)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), pastedKey) {
		t.Fatal("the answer holds the value")
	}
	if v, _ := ta.Vault.Get(ctx, "conn.github.token"); v != pastedKey {
		t.Fatalf("vault holds %q", v)
	}
	if code, _ := ta.do(t, "POST", "/api/credentials/"+id, map[string]string{"value": "again"}); code != 404 {
		t.Fatalf("a used link worked again: %d", code)
	}
	if _, out := ta.do(t, "GET", "/api/credentials", nil); len(out["list"].([]any)) != 0 {
		t.Fatalf("still open: %v", out)
	}
	nowhere(t, ta, pastedKey)
	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{"credential.saved"}})
	if len(evs) != 1 {
		t.Fatalf("%d saved events", len(evs))
	}
}

// A task the model runs gets the same private request; what the model
// reads back never holds a value.
func TestAnExplorationAsksAndTheModelNeverSeesTheValue(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		err := rpc(r.MCPURL, 1, "github_issues", map[string]any{"repo": "a/b"})
		b, _ := json.Marshal(r)
		mu.Lock()
		seen = append(seen, string(b), errText(err))
		mu.Unlock()
		return llm.Response{Text: "GitHub needs a token."}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	code, out := ta.do(t, "POST", "/api/explorations", map[string]string{"request": "list my issues on a/b"})
	if code != 202 {
		t.Fatalf("start %d %v", code, out)
	}
	exp := out["id"].(string)
	ta.Explore.Wait()
	mu.Lock()
	toolErr := seen[1]
	mu.Unlock()
	if !strings.Contains(toolErr, "asked the person privately") {
		t.Fatalf("the model read %q", toolErr)
	}
	_, out = ta.do(t, "GET", "/api/credentials", nil)
	req := out["list"].([]any)[0].(map[string]any)
	if req["exploration"] != exp {
		t.Fatalf("request: %v", req)
	}
	_, out = ta.do(t, "POST", "/api/credentials/"+req["id"].(string), map[string]string{"value": pastedKey})
	if out["exploration"] != exp {
		t.Fatalf("save: %v", out)
	}
	// Asked again, the task starts over, and the key stays out of it.
	code, out = ta.do(t, "POST", "/api/explorations/"+exp+"/retry", nil)
	if code != 202 || out["id"] == "" {
		t.Fatalf("retry %d %v", code, out)
	}
	ta.Explore.Wait()
	mu.Lock()
	defer mu.Unlock()
	for _, s := range seen {
		if strings.Contains(s, pastedKey) {
			t.Fatal("the model saw the value")
		}
	}
	nowhere(t, ta, pastedKey)
}

// A member's request is theirs alone: the administrator neither sees nor
// answers it, and the key lands under the member's name, not the house's.
func TestCredentialRequestsBelongToTheirPerson(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	_, p := ta.do(t, "POST", "/api/people", map[string]string{"name": "Ana", "role": "member"})
	ana := p["id"].(string)
	link, _ := ta.invite(t, ana, "Celular da Ana")
	_, anaTok := ta.open(t, link)
	err := ta.missingCredential(people.With(ctx, ana), "routine:x#1", &connector.MissingCredential{Connector: "notion", Field: "token"})
	if err == nil || !strings.Contains(err.Error(), "privately") {
		t.Fatalf("hook: %v", err)
	}
	if _, out := ta.do(t, "GET", "/api/credentials", nil); len(out["list"].([]any)) != 0 {
		t.Fatalf("the administrator sees Ana's request: %v", out)
	}
	_, body := ta.raw(t, anaTok, "GET", "/api/credentials", nil)
	var mine []map[string]any
	json.Unmarshal([]byte(body), &mine)
	if len(mine) != 1 {
		t.Fatalf("Ana's requests: %s", body)
	}
	id := mine[0]["id"].(string)
	if n := noticesTo(t, ta, ana); len(n) != 1 {
		t.Fatalf("Ana's notices: %v", n)
	}
	if n := noticesTo(t, ta, people.OwnerID); len(n) != 0 {
		t.Fatalf("the administrator was told: %v", n)
	}
	for _, m := range []string{"GET", "POST", "DELETE"} {
		if code, _ := ta.do(t, m, "/api/credentials/"+id, map[string]string{"value": "owner-typed"}); code != 404 {
			t.Fatalf("the administrator reached Ana's request with %s: %d", m, code)
		}
	}
	if code, body := ta.raw(t, anaTok, "POST", "/api/credentials/"+id, []byte(`{"value":"`+pastedKey+`"}`)); code != 200 || strings.Contains(body, pastedKey) {
		t.Fatalf("Ana saves: %d %s", code, body)
	}
	if v, _ := ta.Vault.Get(ctx, "person."+ana+".conn.notion.token"); v != pastedKey {
		t.Fatalf("Ana's vault entry: %q", v)
	}
	if _, err := ta.Vault.Get(ctx, "conn.notion.token"); err == nil {
		t.Fatal("the key went to the house")
	}
	// A house key is the administrator's to give: a member's run only
	// says what is missing.
	if err := ta.missingCredential(people.With(ctx, ana), "", &connector.MissingCredential{Connector: "nope", Field: "x", Err: errString("x")}); strings.Contains(err.Error(), "privately") {
		t.Fatalf("unknown connector: %v", err)
	}
	nowhere(t, ta, pastedKey)
}

type errString string

func (e errString) Error() string { return string(e) }

// A key pasted into a channel message or the app's chat is taken out
// before the model, the conversation and the log see it, and the person
// is warned; a message that was only a key goes no further.
func TestPastedKeysAreRemovedBeforeAnythingSeesThem(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		b, _ := json.Marshal(r)
		mu.Lock()
		prompts = append(prompts, string(b))
		mu.Unlock()
		return llm.Response{Text: "ok"}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	ctx := ownerpkg.Via(people.With(context.Background(), people.OwnerID), "telegram")

	reply, err := handler{ta.App}.Request(ctx, "use this for GitHub: "+pastedKey)
	if err != nil || !strings.HasPrefix(reply, "🔒") {
		t.Fatalf("channel reply %q %v", reply, err)
	}
	before, _ := ta.Store.Explorations(context.Background())
	reply, _ = handler{ta.App}.Request(ctx, pastedKey)
	after, _ := ta.Store.Explorations(context.Background())
	if !strings.HasPrefix(reply, "🔒") || len(after) != len(before) {
		t.Fatalf("a lone key started a task: %q (%d → %d)", reply, len(before), len(after))
	}

	code, out := ta.do(t, "POST", "/api/chats", map[string]string{"text": "my password: Tr0ub4dor&3xx and list my mail"})
	if code != 201 || out["warning"] == nil {
		t.Fatalf("chat %d %v", code, out)
	}
	if code, _ := ta.do(t, "POST", "/api/chats", map[string]string{"text": "sk-ant-api03-" + strings.Repeat("Zq9_", 12)}); code != 400 {
		t.Fatalf("a chat of only a key: %d", code)
	}
	ta.Explore.Wait()
	exps, _ := ta.Store.Explorations(context.Background())
	for _, e := range exps {
		if strings.Contains(e.Request, pastedKey) || strings.Contains(e.Request, "Tr0ub4dor") {
			t.Fatalf("stored request %q", e.Request)
		}
	}
	chats, _ := ta.Store.Chats(context.Background(), chatPerson(people.OwnerID))
	for _, c := range chats {
		if strings.Contains(c.Title, "Tr0ub4dor") {
			t.Fatalf("chat title %q", c.Title)
		}
	}
	mu.Lock()
	for _, p := range prompts {
		if strings.Contains(p, pastedKey) || strings.Contains(p, "Tr0ub4dor") {
			t.Fatal("the model saw a pasted key")
		}
	}
	mu.Unlock()
	nowhere(t, ta, pastedKey)
	nowhere(t, ta, "Tr0ub4dor")
	if e, _ := ta.Store.Explorations(context.Background(), store.ExplorationRunning); len(e) != 0 {
		t.Fatal("still running")
	}
}

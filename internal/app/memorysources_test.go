package app

import (
	"context"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/owner"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/routine"
)

func factNamed(t *testing.T, ta *testApp, prefix string) []memory.Fact {
	t.Helper()
	all, _ := ta.Memory.List()
	var out []memory.Fact
	for _, f := range all {
		if strings.HasPrefix(f.Text, prefix) {
			out = append(out, f)
		}
	}
	return out
}

// Facts the agent notes keep what asked for them (a conversation, an
// exploration, a routine) and, when the sender is in mail it read, the
// email; a sender it made up is not kept.
func TestNotedFactsKeepTheirSources(t *testing.T) {
	ta := newApp(t, nil, &llm.Fake{})
	ctx := context.Background()
	mailboxWith(t, ta, [][]byte{message("PLANOS")})
	ta.Agent = llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		callTool(r.MCPURL, "gmail_search", map[string]any{"query": "", "max": 10})
		word := strings.Fields(r.Prompt)[len(strings.Fields(r.Prompt))-1]
		callTool(r.MCPURL, "memory_note", map[string]any{"fact": "NOTE " + word, "email": map[string]string{"sender": "X@y.com", "message_id": "<PLANOS@y.com>", "subject": "PLANOS"}})
		callTool(r.MCPURL, "memory_note", map[string]any{"fact": "FORGED " + word, "email": map[string]string{"sender": "boss@elsewhere.com"}})
		return llm.Response{Text: "ok"}, nil
	}}

	_, out := ta.do(t, "POST", "/api/chats", map[string]string{"text": "o que diz o email chat"})
	chat := out["chat"].(string)
	_, out = ta.do(t, "POST", "/api/explorations", map[string]string{"request": "resuma solo"})
	exp := out["id"].(string)
	ta.Store.SaveRoutine(ctx, "r1", routine.Routine{Name: "Resumo", Description: "resuma rotina", Code: "async function run() {}"}, "test", "human:owner")
	if _, err := ta.Explore.Repair(ctx, "r1", "", "human:owner"); err != nil {
		t.Fatal(err)
	}
	(handler{ta.App}).Request(owner.Via(people.With(ctx, "owner"), "telegram"), "e no telegram")
	ta.Explore.Wait()
	_, chats := ta.do(t, "GET", "/api/chats", nil)
	var tg string
	for _, c := range chats["list"].([]any) {
		if c := c.(map[string]any); c["id"] != chat {
			tg = c["id"].(string)
		}
	}
	if fs := factNamed(t, ta, "NOTE telegram"); len(fs) != 1 || tg == "" || !fs[0].Has("conversation:"+tg) {
		t.Fatalf("a channel conversation's note: %q %+v", tg, fs)
	}

	for prefix, key := range map[string]string{"NOTE chat": "conversation:" + chat, "NOTE solo": "exploration:" + exp, "NOTE rotina": "routine:r1"} {
		fs := factNamed(t, ta, prefix)
		if len(fs) != 1 || !fs[0].Has(key) || !fs[0].Has("email:x@y.com") {
			t.Fatalf("%s: %+v", prefix, fs)
		}
	}
	for _, f := range factNamed(t, ta, "FORGED") {
		if f.Has("email:boss@elsewhere.com") || len(f.From()) != 1 {
			t.Fatalf("a made-up sender was kept: %+v", f.From())
		}
	}
	if f := factNamed(t, ta, "NOTE chat")[0]; f.From()[0].Label != "o que diz o email chat" || f.From()[0].Turn == "" {
		t.Fatalf("conversation origin: %+v", f.From()[0])
	}

	code, got := ta.do(t, "POST", "/api/memory", map[string]string{"text": "TYPED fact"})
	if code != 200 || !strings.Contains(js(got["origins"]).String(), `"kind":"typed"`) {
		t.Fatalf("%d %v", code, got)
	}

	// Forgetting the sender takes the four notes read in their mail and
	// nothing else.
	_, src := ta.do(t, "GET", "/api/memory/sources", nil)
	if !strings.Contains(js(src).String(), `"key":"email:x@y.com"`) {
		t.Fatalf("%v", src)
	}
	code, res := ta.do(t, "POST", "/api/memory/sources/forget", map[string]string{"key": "email:x@y.com"})
	if code != 200 || len(res["removed"].([]any)) != 4 {
		t.Fatalf("%d %v", code, res)
	}
	if len(factNamed(t, ta, "NOTE")) != 0 || len(factNamed(t, ta, "FORGED")) != 4 || len(factNamed(t, ta, "TYPED")) != 1 {
		t.Fatal("forgot the wrong facts")
	}
	if h, _ := ta.Memory.History(1); !strings.HasPrefix(h[0].Message, "forget source: email") {
		t.Fatalf("history %v", h)
	}
}

// A person sees and forgets only their own sources.
func TestSourcesArePrivate(t *testing.T) {
	h := newHouse(t)
	ta := h.testApp
	ta.Memory.AddFrom(h.ownerMark+" noted", "t", "x", memory.Low, "", memory.Origin{Kind: memory.FromConversation, Ref: h.owner["chat"], Label: h.ownerMark + " chat"})
	ta.raw(t, h.ana, "POST", "/api/memory", js(map[string]any{"text": "ANA house fact", "shared": true}))

	code, body := ta.raw(t, h.ana, "GET", "/api/memory/sources", nil)
	if code != 200 || strings.Contains(body, h.ownerMark) || !strings.Contains(body, "ANA house fact") || !strings.Contains(body, h.anaMark) {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := ta.raw(t, h.ana, "POST", "/api/memory/sources/forget", js(map[string]string{"key": "conversation:" + h.owner["chat"]})); code != 404 {
		t.Fatalf("Ana forgot the owner's source: %d", code)
	}
	// The owner's sources hold neither Ana's facts nor the house fact she shared.
	_, own := ta.do(t, "GET", "/api/memory/sources", nil)
	if s := js(own).String(); strings.Contains(s, h.anaMark) || strings.Contains(s, "ANA house") {
		t.Fatalf("%s", s)
	}
	ta.do(t, "POST", "/api/memory/sources/forget", map[string]string{"key": "typed"})
	if len(factNamed(t, ta, h.ownerMark+" gosta")) != 0 || len(factNamed(t, ta, h.anaMark)) != 1 || len(factNamed(t, ta, "ANA house")) != 1 {
		t.Fatal("the owner's forgetting reached Ana's facts")
	}
	if code, _ := ta.raw(t, h.ana, "POST", "/api/memory/sources/forget", js(map[string]string{"key": "typed"})); code != 200 {
		t.Fatal(code)
	}
	if len(factNamed(t, ta, h.anaMark)) != 0 || len(factNamed(t, ta, "ANA house")) != 0 || len(factNamed(t, ta, h.ownerMark+" noted")) != 1 {
		t.Fatal("Ana's forgetting took the wrong facts")
	}
}

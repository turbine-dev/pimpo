package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denerFernandes/pimpo/internal/capability"
	"github.com/denerFernandes/pimpo/internal/llm"
)

// notes is a connector with one reversible capability.
type notes struct {
	mu    sync.Mutex
	saved []string
}

func (n *notes) Capabilities() []string { return []string{"note.save"} }
func (n *notes) Call(_ context.Context, _, _ string, args any) (any, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.saved = append(n.saved, args.(map[string]any)["text"].(string))
	return map[string]bool{"saved": true}, nil
}

func TestChatRehearsesThenDoesExactlyWhatItShowed(t *testing.T) {
	capability.Register(capability.Spec{Name: "note.save", Risk: capability.Reversible, Signature: "note.save({text})", Returns: "saved"})
	defer capability.Unregister("note.save")
	var mu sync.Mutex
	var prompts []string
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		mu.Lock()
		prompts = append(prompts, r.Prompt)
		mu.Unlock()
		rpc(r.MCPURL, 1, "note_save", map[string]any{"text": "pagar a conta de luz"})
		return llm.Response{Text: "Salvo a nota 'pagar a conta de luz'.", CostUSD: 0.02}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	n := &notes{}
	ta.Router.Add(n)

	code, out := ta.do(t, "POST", "/api/chats", map[string]string{"text": "Anota: pagar a conta de luz"})
	if code != 201 {
		t.Fatalf("%d %v", code, out)
	}
	chat, first := out["chat"].(string), out["turn"].(string)
	ta.Explore.Wait()
	_, got := ta.do(t, "GET", "/api/chats/"+chat, nil)
	turn := got["turns"].([]any)[0].(map[string]any)
	acts := turn["actions"].([]any)
	if turn["state"] != "ready" || len(acts) != 1 || acts[0].(map[string]any)["capability"] != "note.save" {
		t.Fatalf("%v", turn)
	}
	if len(n.saved) != 0 {
		t.Fatal("the rehearsal changed something")
	}
	_, list := ta.do(t, "GET", "/api/chats", nil)
	if l := list["list"].([]any); len(l) != 1 || l[0].(map[string]any)["title"] != "Anota: pagar a conta de luz" {
		t.Fatalf("%v", list)
	}

	code, _ = ta.do(t, "POST", "/api/chats/"+chat+"/messages", map[string]string{"text": "e a de água também"})
	if code != 201 {
		t.Fatal(code)
	}
	ta.Explore.Wait()
	mu.Lock()
	second := prompts[1]
	mu.Unlock()
	if !strings.Contains(second, "Owner: Anota: pagar a conta de luz") || !strings.HasSuffix(second, "Now the owner says: e a de água também") {
		t.Fatalf("the second message lost the conversation: %q", second)
	}

	code, res := ta.do(t, "POST", "/api/chats/"+chat+"/turns/"+first+"/do", nil)
	if code != 202 {
		t.Fatalf("%d %v", code, res)
	}
	for i := 0; i < 100; i++ {
		_, got = ta.do(t, "GET", "/api/chats/"+chat, nil)
		if d, _ := got["turns"].([]any)[0].(map[string]any)["done"].(map[string]any); d != nil && d["state"] != "running" {
			if d["state"] != "done" {
				t.Fatalf("%v", d)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	n.mu.Lock()
	saved := strings.Join(n.saved, "|")
	n.mu.Unlock()
	if saved != "pagar a conta de luz" {
		t.Fatalf("did %q", saved)
	}
	if code, _ := ta.do(t, "POST", "/api/chats/"+chat+"/turns/"+first+"/do", nil); code != 409 {
		t.Fatal("did it twice")
	}
	if code, _ := ta.do(t, "POST", "/api/chats/other/turns/"+first+"/do", nil); code != 404 {
		t.Fatal("reached an answer through another conversation")
	}
	if code, _ := ta.do(t, "DELETE", "/api/chats/"+chat, nil); code != 200 {
		t.Fatal(code)
	}
}

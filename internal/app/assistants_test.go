package app

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/denerFernandes/zodim/internal/capability"
	"github.com/denerFernandes/zodim/internal/llm"
)

func TestAssistantOnlyUsesItsCapabilities(t *testing.T) {
	capability.Register(capability.Spec{Name: "note.save", Risk: capability.Reversible, Signature: "note.save({text})", Returns: "saved"})
	defer capability.Unregister("note.save")
	var mu sync.Mutex
	var system string
	var calendarErr, noteErr error
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		e1 := rpc(r.MCPURL, 1, "calendar_events", map[string]any{"from": "today", "to": "tomorrow"})
		e2 := rpc(r.MCPURL, 2, "note_save", map[string]any{"text": "ideia"})
		mu.Lock()
		system, calendarErr, noteErr = r.System, e1, e2
		mu.Unlock()
		return llm.Response{Text: "Anotei."}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	ta.Router.Add(&notes{})

	for _, bad := range []map[string]any{
		{"name": ""},
		{"name": "Notas", "capabilities": []string{"nope.x"}},
	} {
		if code, _ := ta.do(t, "PUT", "/api/assistants/notas", bad); code != 400 {
			t.Fatalf("accepted %v", bad)
		}
	}
	code, out := ta.do(t, "PUT", "/api/assistants/notas", map[string]any{"name": "Notas", "emoji": "📝", "instructions": "Só cuida das minhas anotações.", "capabilities": []string{"note.save", "note.save"}})
	if code != 200 || len(out["capabilities"].([]any)) != 1 {
		t.Fatalf("%d %v", code, out)
	}
	code, out = ta.do(t, "POST", "/api/chats", map[string]string{"text": "anota uma ideia", "assistant": "notas"})
	if code != 201 {
		t.Fatalf("%d %v", code, out)
	}
	ta.Explore.Wait()
	mu.Lock()
	defer mu.Unlock()
	if calendarErr == nil || noteErr != nil {
		t.Fatalf("calendar %v, note %v", calendarErr, noteErr)
	}
	if !strings.Contains(system, "assistant named Notas") || !strings.Contains(system, "Só cuida das minhas anotações.") {
		t.Fatalf("system prompt: %q", system)
	}
	_, list := ta.do(t, "GET", "/api/chats", nil)
	if list["list"].([]any)[0].(map[string]any)["assistant"] != "notas" {
		t.Fatalf("%v", list)
	}
	if code, _ := ta.do(t, "POST", "/api/chats", map[string]string{"text": "oi", "assistant": "ghost"}); code != 400 {
		t.Fatal("started a chat with a missing assistant")
	}
	if code, _ := ta.do(t, "DELETE", "/api/assistants/notas", nil); code != 200 {
		t.Fatal(code)
	}
}

func TestSettingsMuteOnlyWhatMayBeMuted(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	base := ta.Settings(t.Context())
	for _, bad := range []Settings{{Mute: []string{"approval"}}, {LabsOff: []string{"everything"}}} {
		s := base
		s.Mute, s.LabsOff = bad.Mute, bad.LabsOff
		if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 400 {
			t.Fatalf("accepted %+v", bad)
		}
	}
	s := base
	s.Mute, s.LabsOff = []string{"task"}, []string{"mcp_registry"}
	if code, _ := ta.do(t, "PUT", "/api/settings", s); code != 200 {
		t.Fatal(code)
	}
	if !ta.Channel.Muted(t.Context(), "task") || ta.Channel.Muted(t.Context(), "approval") || ta.lab(t.Context(), "mcp_registry") {
		t.Fatal("settings not applied")
	}
	if code, _ := ta.do(t, "GET", "/api/connectors/registry?q=x", nil); code != 403 {
		t.Fatal("the registry answered while turned off")
	}
}

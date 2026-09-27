package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denerFernandes/pimpo/internal/chatlink"
	"github.com/denerFernandes/pimpo/internal/llm"
)

type typingLink struct {
	fakeLink
	mu     sync.Mutex
	typing int
}

func (l *typingLink) Typing(context.Context, string) error {
	l.mu.Lock()
	l.typing++
	l.mu.Unlock()
	return nil
}

func (l *typingLink) said(part string) bool {
	l.fakeLink.mu.Lock()
	defer l.fakeLink.mu.Unlock()
	for _, m := range l.sent {
		if strings.Contains(m, part) {
			return true
		}
	}
	return false
}

// Follow-ups on a chat channel continue one conversation, shown in the app
// too; /new starts over; the channel shows "typing…" while a task runs.
func TestConversationOnAChatChannel(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	release := make(chan struct{})
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		mu.Lock()
		prompts = append(prompts, r.Prompt)
		first := len(prompts) == 1
		mu.Unlock()
		if first {
			<-release
		}
		return llm.Response{Text: "Amanhã: dentista às 10h."}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	ta.TypingEvery = 20 * time.Millisecond
	ctx := context.Background()
	l := &typingLink{}
	run := &linkRun{link: l, cancel: func() {}}
	ta.links = map[string]*linkRun{"signal": run}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "pimpo " + ta.Channel.PairingCode()})

	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "o que tenho hoje?"})
	waitFor(t, func() bool { l.mu.Lock(); defer l.mu.Unlock(); return l.typing >= 2 })
	close(release)
	ta.Explore.Wait()
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "e amanhã?"})
	ta.Explore.Wait()
	if !l.said("seguindo a conversa") {
		t.Fatalf("sent %q", l.sent)
	}
	mu.Lock()
	second := prompts[1]
	mu.Unlock()
	if !strings.Contains(second, "o que tenho hoje?") || !strings.Contains(second, "e amanhã?") {
		t.Fatalf("the follow-up lost the conversation: %q", second)
	}
	_, chats := ta.do(t, "GET", "/api/chats", nil)
	list := chats["list"].([]any)
	if len(list) != 1 || !strings.HasPrefix(list[0].(map[string]any)["title"].(string), "Signal · o que tenho hoje?") || list[0].(map[string]any)["turns"] != float64(2) {
		t.Fatalf("chats %v", list)
	}

	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "/novo"})
	if !l.said("Nova conversa") {
		t.Fatalf("sent %q", l.sent)
	}
	ta.linkMessage(ctx, "signal", run, chatlink.Inbound{From: "+551199", Text: "qual o tempo?"})
	ta.Explore.Wait()
	mu.Lock()
	third := prompts[2]
	mu.Unlock()
	if strings.Contains(third, "amanhã") {
		t.Fatalf("/new kept the old conversation: %q", third)
	}
}

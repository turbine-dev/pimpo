package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/llm"
	"github.com/denerFernandes/pimpo/internal/owner"
)

// A reminder asked for in chat is set during the exploration itself, is
// not offered as a routine, goes out once when due (saying so when late),
// and can be cancelled.
func TestReminders(t *testing.T) {
	agent := llm.FakeAgent{Script: func(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
		if err := rpc(r.MCPURL, 1, "reminder_set", map[string]any{"in": "30m", "text": "conferir o deploy"}); err != nil {
			return llm.Response{}, err
		}
		return llm.Response{Text: "Vou te lembrar em 30 minutos."}, nil
	}}
	ta := newApp(t, agent, &llm.Fake{})
	ctx := context.Background()
	_, out := ta.do(t, "POST", "/api/chats", map[string]string{"text": "daqui a 30 min me lembra de conferir o deploy"})
	ta.Explore.Wait()
	_, chat := ta.do(t, "GET", "/api/chats/"+out["chat"].(string), nil)
	if st := chat["turns"].([]any)[0].(map[string]any)["state"]; st != "done" {
		t.Fatalf("a reminder was offered as a routine: %v", st)
	}
	_, list := ta.do(t, "GET", "/api/reminders", nil)
	items := list["list"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["text"] != "conferir o deploy" {
		t.Fatalf("reminders %v", list)
	}
	ta.sendDue(ctx, time.Now().Add(10*time.Minute))
	if n := notices(t, ta); len(n) != 0 {
		t.Fatalf("sent early: %q", n)
	}
	ta.sendDue(ctx, time.Now().Add(31*time.Minute))
	ta.sendDue(ctx, time.Now().Add(32*time.Minute))
	n := notices(t, ta)
	if len(n) != 1 || !strings.Contains(n[0], "conferir o deploy") || strings.Contains(n[0], "fechado") {
		t.Fatalf("notices %q", n)
	}

	c := reminderCap{ta.App}
	late, _ := c.Call(ctx, "reminder.set", "", map[string]any{"in": "1m", "text": "tomar água"})
	c.Call(ctx, "reminder.set", "", map[string]any{"at": time.Now().Add(2 * time.Hour).Format(time.RFC3339), "text": "ligar pra Ana"})
	ta.sendDue(ctx, time.Now().Add(20*time.Minute))
	if n := notices(t, ta); len(n) != 2 || !strings.Contains(n[1], "tomar água") || !strings.Contains(n[1], "fechado") {
		t.Fatalf("late notice %q (%v)", n, late)
	}
	_, list = ta.do(t, "GET", "/api/reminders", nil)
	id := list["list"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := ta.do(t, "DELETE", "/api/reminders/"+id, nil); code != 200 {
		t.Fatal("cancel")
	}
	if _, list = ta.do(t, "GET", "/api/reminders", nil); len(list["list"].([]any)) != 0 {
		t.Fatalf("left %v", list)
	}
	for _, bad := range []map[string]any{{"text": "x"}, {"in": "soon", "text": "x"}, {"at": "2020-01-01T00:00:00Z", "text": "x"}, {"in": "5m"}} {
		if _, err := c.Call(ctx, "reminder.set", "", bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}

func notices(t *testing.T, ta *testApp) []string {
	evs, _ := ta.Events.List(context.Background(), event.Query{Types: []string{owner.EventNotice}})
	var texts []string
	for _, e := range evs {
		var n struct{ Text string }
		e.Decode(&n)
		if strings.Contains(n.Text, "⏰") {
			texts = append(texts, n.Text)
		}
	}
	return texts
}

package owner

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/explore"
	"github.com/denerFernandes/vigia/internal/telegram"
)

type fakeBot struct {
	mu     sync.Mutex
	sent   []string
	edits  []string
	chats  []int64
	button [][]telegram.Button
}

func (b *fakeBot) Send(_ context.Context, chat int64, text string, rows ...[]telegram.Button) (telegram.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, text)
	b.chats = append(b.chats, chat)
	b.button = append(b.button, rows...)
	return telegram.Message{}, nil
}
func (b *fakeBot) Edit(_ context.Context, _, _ int64, text string) error {
	b.edits = append(b.edits, text)
	return nil
}
func (b *fakeBot) Answer(context.Context, string, string) error             { return nil }
func (b *fakeBot) Poll(context.Context, int64, func(telegram.Update)) error { return nil }

type handler struct{ requests, buttons []string }

func (h *handler) Request(_ context.Context, text string) (string, error) {
	h.requests = append(h.requests, text)
	return "Começando.", nil
}
func (h *handler) Button(_ context.Context, action, id string) (string, error) {
	h.buttons = append(h.buttons, action+":"+id)
	return "Rotina criada.", nil
}

func msg(chat int64, text string) telegram.Update {
	m := &telegram.Message{Text: text}
	m.Chat.ID = chat
	m.From.FirstName = "Dener"
	return telegram.Update{Message: m}
}

func TestPairingAndOwnerOnly(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &fakeBot{}
	h := &handler{}
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot }, Handler: h}
	ctx := context.Background()

	c.handle(ctx, bot, msg(42, "resumo por favor"))
	if len(h.requests) != 0 {
		t.Fatal("an unpaired chat started work")
	}
	c.handle(ctx, bot, msg(42, "/start 000"))
	if chat, _ := c.Chat(ctx); chat != 0 {
		t.Fatal("paired with a wrong code")
	}
	c.handle(ctx, bot, msg(42, "/start "+c.PairingCode()))
	if chat, _ := c.Chat(ctx); chat != 42 {
		t.Fatalf("not paired, chat %d", chat)
	}
	c.handle(ctx, bot, msg(99, "/start anything"))
	c.handle(ctx, bot, msg(99, "manda meus emails"))
	if len(h.requests) != 0 {
		t.Fatal("a stranger reached the handler")
	}
	c.handle(ctx, bot, msg(42, "Todo dia às 7h me manda a agenda"))
	if len(h.requests) != 1 {
		t.Fatalf("owner request lost: %v", h.requests)
	}

	cb := &telegram.Callback{ID: "1", Data: "compile:e1", Message: &telegram.Message{ID: 5, Text: "Quer uma rotina?"}}
	cb.Message.Chat.ID = 42
	c.handle(ctx, bot, telegram.Update{Callback: cb})
	if len(h.buttons) != 1 || h.buttons[0] != "compile:e1" || !strings.Contains(bot.edits[0], "Rotina criada.") {
		t.Fatalf("button %v edits %v", h.buttons, bot.edits)
	}
}

func TestNotifyAlwaysLogsAndSendsWhenPaired(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &fakeBot{}
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot }}
	ctx := context.Background()
	c.Notify(ctx, explore.Notice{Text: "antes de parear"})
	if len(bot.sent) != 0 {
		t.Fatal("sent before pairing")
	}
	ev.Put(ctx, chatKey, "42")
	c.Notify(ctx, explore.Notice{Text: "Quer uma rotina?", Actions: []explore.Action{{Label: "Sim", Data: "compile:e1"}}})
	if len(bot.sent) != 1 || bot.chats[0] != 42 || bot.button[0][0].Data != "compile:e1" {
		t.Fatalf("sent %v %v", bot.sent, bot.button)
	}
	evs, _ := ev.List(ctx, event.Query{Types: []string{EventNotice}})
	if len(evs) != 2 {
		t.Fatalf("notices logged %d", len(evs))
	}
}

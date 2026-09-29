package owner

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/telegram"
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

type voiceBot struct{ fakeBot }

func (b *voiceBot) Download(_ context.Context, id string) ([]byte, error) {
	return []byte("audio:" + id), nil
}

func TestVoiceNotesBecomeRequests(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &voiceBot{}
	h := &handler{}
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot }, Handler: h,
		Transcribe: func(_ context.Context, audio []byte) (string, error) {
			if string(audio) != "audio:f1" {
				return "", errors.New("wrong file")
			}
			return "me lembra de pagar a luz", nil
		}}
	ctx := context.Background()
	ev.Put(ctx, chatKey, "42")
	voice := func(chat int64) telegram.Update {
		u := msg(chat, "")
		u.Message.Voice = &struct {
			FileID   string `json:"file_id"`
			Duration int    `json:"duration"`
		}{FileID: "f1", Duration: 3}
		return u
	}
	c.handle(ctx, bot, voice(99))
	if len(h.requests) != 0 || len(bot.sent) != 0 {
		t.Fatal("a stranger's voice note was heard")
	}
	c.handle(ctx, bot, voice(42))
	if len(h.requests) != 1 || h.requests[0] != "me lembra de pagar a luz" || !strings.Contains(bot.sent[0], "pagar a luz") {
		t.Fatalf("requests %v sent %v", h.requests, bot.sent)
	}
	c.Transcribe = nil
	c.handle(ctx, bot, voice(42))
	if len(h.requests) != 1 || !strings.Contains(bot.sent[len(bot.sent)-1], "Não deu para entender o áudio") {
		t.Fatalf("without transcription: %v", bot.sent)
	}
}

func TestPhotosBecomeRequests(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &voiceBot{}
	h := &handler{}
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot }, Handler: h,
		ReadPhoto: func(_ context.Context, img []byte) (string, error) {
			return "Conta de luz, vence 10/10 (" + string(img) + ")", nil
		}}
	ctx := context.Background()
	ev.Put(ctx, chatKey, "42")
	u := msg(42, "")
	u.Message.Caption = "Me lembre de pagar esta conta"
	u.Message.Photo = []struct {
		FileID string `json:"file_id"`
		Width  int    `json:"width"`
	}{{"small", 90}, {"big", 1280}}
	c.handle(ctx, bot, u)
	if len(h.requests) != 1 || !strings.HasPrefix(h.requests[0], "Me lembre de pagar esta conta") || !strings.Contains(h.requests[0], "(audio:big)") {
		t.Fatalf("%v", h.requests)
	}
}

func TestMutedNoticesStayInTheInbox(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &fakeBot{}
	mirrored := 0
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot },
		Mirror: func(context.Context, explore.Notice) { mirrored++ },
		Muted:  func(_ context.Context, kind string) bool { return kind == "task" }}
	ctx := context.Background()
	ev.Put(ctx, chatKey, "42")
	c.Notify(ctx, explore.Notice{Text: "✅ pronto", Kind: "task"})
	c.Notify(ctx, explore.Notice{Text: "Posso apagar?", Kind: "approval"})
	if len(bot.sent) != 1 || bot.sent[0] != "Posso apagar?" || mirrored != 1 {
		t.Fatalf("sent %v, mirrored %d", bot.sent, mirrored)
	}
	evs, _ := ev.List(ctx, event.Query{Types: []string{EventNotice}})
	if len(evs) != 2 {
		t.Fatalf("a muted notice left the inbox: %d", len(evs))
	}
}

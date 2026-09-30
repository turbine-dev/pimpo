package owner

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
	m.Chat.ID, m.Chat.Type = chat, "private"
	m.From.ID, m.From.FirstName = chat, "Dener"
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
	cb.Message.Chat.ID, cb.Message.Chat.Type, cb.From.ID = 42, "private", 42
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

// A group's chat id is shared by everyone in it: a group never pairs and
// is never heard, even the owner's chat id with someone else speaking.
func TestGroupsAndChannelsAreNeverAWayIn(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &fakeBot{}
	h := &handler{}
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot }, Handler: h}
	ctx := context.Background()
	group := msg(-100, "/start "+c.PairingCode())
	group.Message.Chat.Type, group.Message.From.ID = "supergroup", 7
	c.handle(ctx, bot, group)
	if chat, _ := c.Chat(ctx); chat != 0 {
		t.Fatalf("a group paired: %d", chat)
	}
	ev.Put(ctx, chatKey, "-100")
	group = msg(-100, "manda meus emails")
	group.Message.Chat.Type, group.Message.From.ID = "group", 7
	c.handle(ctx, bot, group)
	cb := &telegram.Callback{ID: "1", Data: "approve:a1", Message: &telegram.Message{ID: 5}}
	cb.Message.Chat.ID, cb.Message.Chat.Type, cb.From.ID = -100, "group", 7
	c.handle(ctx, bot, telegram.Update{Callback: cb})
	if len(h.requests) != 0 || len(h.buttons) != 0 {
		t.Fatalf("a group reached the handler: %v %v", h.requests, h.buttons)
	}
}

// Guessing the pairing code is hopeless: it is long, a wrong code gets
// no answer, a sender who keeps guessing is ignored, and the code is
// used once and changes when someone keeps guessing.
func TestPairingCodeResistsGuessing(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &fakeBot{}
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot }, Handler: &handler{}}
	ctx := context.Background()
	code := c.PairingCode()
	if len(code) < 10 || c.PairingCode() != code {
		t.Fatalf("code %q", code)
	}
	for _, guess := range []string{"", "000000", "123456", "AAAAAAAAAA"} {
		c.handle(ctx, bot, msg(99, "/start "+guess))
	}
	if len(bot.sent) != 0 {
		t.Fatalf("wrong codes were answered: %v", bot.sent)
	}
	c.handle(ctx, bot, msg(99, "/start "+code))
	if chat, _ := c.Chat(ctx); chat != 0 {
		t.Fatal("a sender who kept guessing paired")
	}
	if !c.ClaimOwner(strings.ToLower(code)) || c.ClaimOwner(code) {
		t.Fatal("the code must work once, whatever the case")
	}
	next := c.PairingCode()
	for i := range globalTries {
		c.Wrong("signal:" + strings.Repeat("x", i+1))
	}
	if c.PairingCode() == next {
		t.Fatal("many wrong codes did not change the code")
	}
	c.made = c.made.Add(-codeLife - time.Minute)
	if c.PairingCode() == code || c.ClaimOwner(next) {
		t.Fatal("an old code still works")
	}
}

// replyHandler also answers typed replies to notices with buttons.
type replyHandler struct {
	handler
	replies []string
}

func (h *replyHandler) Reply(_ context.Context, choices []explore.Action, text string) (string, bool) {
	if len(choices) == 0 || !strings.HasPrefix(choices[0].Data, "answer:") {
		return "", false
	}
	h.replies = append(h.replies, choices[0].Data+"|"+text)
	return "Anotado", true
}

// A reply in words to a question goes to the handler with the question's
// buttons; a reply to anything else is an ordinary message. A question's
// many options go three to a row.
func TestTypedReplyToAQuestion(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &fakeBot{}
	h := &replyHandler{}
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot }, Handler: h}
	ctx := context.Background()
	c.handle(ctx, bot, msg(42, "/start "+c.PairingCode()))

	var opts []explore.Action
	for i, o := range []string{"Corrida", "Bike", "Natação", "Descanso"} {
		opts = append(opts, explore.Action{Label: o, Data: "answer:q1." + string(rune('0'+i))})
	}
	c.Notify(ctx, explore.Notice{Text: "❓ Que treino?", Actions: opts})
	if len(bot.button) != 2 || len(bot.button[0]) != 3 || len(bot.button[1]) != 1 {
		t.Fatalf("rows %v", bot.button)
	}
	bot.button = nil
	c.Notify(ctx, explore.Notice{Text: "Posso?", Actions: []explore.Action{{Label: "Sim", Data: "approve:a"}, {Label: "Sempre", Data: "always:a"}, {Label: "Não", Data: "deny:a"}, {Label: "Todos", Data: "batch:a"}}})
	if len(bot.button) != 1 {
		t.Fatalf("an approval's choices left one row: %v", bot.button)
	}

	reply := func(text string, markup *telegram.Markup) telegram.Update {
		u := msg(42, text)
		u.Message.ReplyTo = &telegram.Message{ID: 7, Text: "❓ Que treino?", Markup: markup}
		return u
	}
	c.handle(ctx, bot, reply("natacao", &telegram.Markup{Keyboard: keyboard(opts)}))
	if len(h.replies) != 1 || h.replies[0] != "answer:q1.0|natacao" || bot.sent[len(bot.sent)-1] != "Anotado" {
		t.Fatalf("replies %v sent %v", h.replies, bot.sent)
	}
	c.handle(ctx, bot, reply("e amanhã?", nil))
	c.handle(ctx, bot, reply("ok", &telegram.Markup{Keyboard: [][]telegram.Button{{{Text: "Sim", Data: "compile:e1"}}}}))
	if len(h.replies) != 1 || len(h.requests) != 2 {
		t.Fatalf("replies %v requests %v", h.replies, h.requests)
	}
}

// A notice that waits for an answer offers the Mini App when there is one,
// and only then.
func TestNoticesOfferTheMiniApp(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	bot := &fakeBot{}
	url := ""
	c := &Channel{Events: ev, Bot: func(context.Context) Bot { return bot }, MiniApp: func(context.Context) string { return url }}
	ctx := context.Background()
	ev.Put(ctx, chatKey, "42")
	ask := explore.Notice{Text: "Pode?", Actions: []explore.Action{{Label: "Sim", Data: "approve:a1"}}}
	c.Notify(ctx, ask)
	if len(bot.button) != 1 {
		t.Fatalf("offered a Mini App without an address: %v", bot.button)
	}
	url = "https://pimpo.example.ts.net/tg/app"
	c.Notify(ctx, ask)
	c.Notify(ctx, explore.Notice{Text: "só um aviso"})
	if len(bot.button) != 3 || bot.button[2][0].WebApp == nil || bot.button[2][0].WebApp.URL != url || bot.button[2][0].Data != "" {
		t.Fatalf("buttons %+v", bot.button)
	}
}

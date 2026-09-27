// Package owner is the conversation with the owner, and the people of the
// house, over Telegram: pairing, requests that start explorations, and
// buttons that approve things.
package owner

import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/denerFernandes/pimpo/internal/i18n"
	"math/big"
	"strconv"
	"strings"
	"sync"

	"github.com/denerFernandes/pimpo/internal/event"
	"github.com/denerFernandes/pimpo/internal/explore"
	"github.com/denerFernandes/pimpo/internal/people"
	"github.com/denerFernandes/pimpo/internal/telegram"
)

const (
	EventNotice = "notice.sent"
	EventPaired = "telegram.paired"
	chatKey     = "telegram.chat"
)

// Bot is the part of the Telegram client the channel uses.
type Bot interface {
	Send(ctx context.Context, chat int64, text string, rows ...[]telegram.Button) (telegram.Message, error)
	Edit(ctx context.Context, chat, message int64, text string) error
	Answer(ctx context.Context, callbackID, text string) error
	Poll(ctx context.Context, offset int64, handle func(telegram.Update)) error
}

// Handler reacts to the owner. Button data looks like "compile:<id>". The
// context says who is talking (people.From).
type Handler interface {
	Request(ctx context.Context, text string) (string, error)
	Button(ctx context.Context, action, id string) (string, error)
}

type Channel struct {
	Events *event.Store
	// Bot returns the current bot, or nil when Telegram is not set up.
	Bot     func(ctx context.Context) Bot
	Handler Handler
	// People lets household members talk to Pimpo; nil means only the owner.
	People *people.Directory
	// Mirror also delivers every notice on another channel, such as WhatsApp.
	Mirror func(ctx context.Context, n explore.Notice)
	// Muted says whether the owner silenced this kind of notice; it is
	// still kept in the app's inbox.
	Muted func(ctx context.Context, kind string) bool
	// Transcribe turns a voice note into text; nil means voice notes are
	// not understood.
	Transcribe func(ctx context.Context, audio []byte) (string, error)
	// ReadPhoto finds the text in a photo; nil means photos are ignored.
	ReadPhoto func(ctx context.Context, image []byte) (string, error)

	mu   sync.Mutex
	code string
}

// PairingCode returns a short code the owner sends as "/start <code>".
func (c *Channel) PairingCode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.code == "" {
		n, _ := rand.Int(rand.Reader, big.NewInt(900000))
		c.code = strconv.FormatInt(n.Int64()+100000, 10)
	}
	return c.code
}

func (c *Channel) Chat(ctx context.Context) (int64, error) {
	v, err := c.Events.Get(ctx, chatKey)
	if err != nil || v == "" {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}

type viaKey struct{}

// Via marks a request as coming from a chat channel (telegram, whatsapp,
// discordchat…), so follow-up messages there continue one conversation.
func Via(ctx context.Context, channel string) context.Context {
	return context.WithValue(ctx, viaKey{}, channel)
}

type typingKey struct{}

// WithTyping gives a request a way to show "typing…" where it came from.
func WithTyping(ctx context.Context, show func(context.Context) error) context.Context {
	return context.WithValue(ctx, typingKey{}, show)
}

// TypingOf is how to show "typing…" for a request, or nil.
func TypingOf(ctx context.Context) func(context.Context) error {
	f, _ := ctx.Value(typingKey{}).(func(context.Context) error)
	return f
}

// ChannelOf is the chat channel a request came from, or "".
func ChannelOf(ctx context.Context) string {
	v, _ := ctx.Value(viaKey{}).(string)
	return v
}

// Notify sends a notice to the owner and keeps it in the event log, where
// the web inbox shows it even when Telegram is not paired.
func (c *Channel) Notify(ctx context.Context, n explore.Notice) error {
	c.Events.Append(ctx, EventNotice, "system", map[string]any{"text": n.Text, "actions": n.Actions, "to": people.Norm(n.To), "kind": n.Kind})
	if n.Kind != "" && c.Muted != nil && c.Muted(ctx, n.Kind) {
		return nil
	}
	if c.Mirror != nil {
		c.Mirror(ctx, n)
	}
	bot := c.Bot(ctx)
	chat, _ := c.Chat(ctx)
	if n.To != "" && n.To != people.OwnerID {
		chat = 0
		if c.People != nil {
			if p, err := c.People.Get(ctx, n.To); err == nil {
				chat = p.Chat
			}
		}
	}
	if bot == nil || chat == 0 {
		return nil
	}
	var rows [][]telegram.Button
	var row []telegram.Button
	for _, a := range n.Actions {
		row = append(row, telegram.Button{Text: a.Label, Data: a.Data})
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	_, err := bot.Send(ctx, chat, n.Text, rows...)
	return err
}

// Listen polls Telegram until ctx ends.
func (c *Channel) Listen(ctx context.Context) error {
	bot := c.Bot(ctx)
	if bot == nil {
		return errors.New("telegram is not set up")
	}
	return bot.Poll(ctx, 0, func(u telegram.Update) { c.handle(ctx, bot, u) })
}

// who returns the person a chat belongs to.
func (c *Channel) who(ctx context.Context, chat int64) (string, bool) {
	if owner, _ := c.Chat(ctx); owner != 0 && chat == owner {
		return people.OwnerID, true
	}
	if c.People != nil {
		if p, ok := c.People.ByChat(ctx, chat); ok {
			return p.ID, true
		}
	}
	return "", false
}

func (c *Channel) handle(ctx context.Context, bot Bot, u telegram.Update) {
	if u.Message != nil {
		m := u.Message
		text := strings.TrimSpace(m.Text)
		if strings.HasPrefix(text, "/start") {
			c.pair(ctx, bot, m, strings.TrimSpace(strings.TrimPrefix(text, "/start")))
			return
		}
		person, ok := c.who(ctx, m.Chat.ID)
		if !ok {
			// Only people of the house can talk to Pimpo; others get nothing.
			return
		}
		if text == "" && len(m.Photo) > 0 {
			seen, err := c.read(ctx, bot, m.Photo[len(m.Photo)-1].FileID)
			if err != nil {
				bot.Send(ctx, m.Chat.ID, i18n.T(ctx, "msg.photo.failed", "error", err))
				return
			}
			ask := strings.TrimSpace(m.Caption)
			if ask == "" {
				ask = i18n.T(ctx, "msg.photo.ask")
			}
			text = ask + "\n\n" + i18n.T(ctx, "msg.photo.text") + "\n" + seen
		}
		if text == "" && m.Voice != nil {
			heard, err := c.listen(ctx, bot, m.Voice.FileID)
			if err != nil {
				bot.Send(ctx, m.Chat.ID, i18n.T(ctx, "msg.voice.failed", "error", err))
				return
			}
			bot.Send(ctx, m.Chat.ID, "🎙️ “"+heard+"”")
			text = heard
		}
		if text == "" {
			return
		}
		rctx := Via(people.With(ctx, person), "telegram")
		if t, ok := bot.(interface {
			Typing(context.Context, int64) error
		}); ok {
			chat := m.Chat.ID
			rctx = WithTyping(rctx, func(ctx context.Context) error { return t.Typing(ctx, chat) })
		}
		reply, err := c.Handler.Request(rctx, text)
		if err != nil {
			reply = i18n.T(ctx, "msg.start.failed", "error", err)
		}
		bot.Send(ctx, m.Chat.ID, reply)
		return
	}
	if cb := u.Callback; cb != nil {
		if cb.Message == nil {
			bot.Answer(ctx, cb.ID, "")
			return
		}
		person, ok := c.who(ctx, cb.Message.Chat.ID)
		if !ok {
			bot.Answer(ctx, cb.ID, "")
			return
		}
		action, id, _ := strings.Cut(cb.Data, ":")
		bot.Answer(ctx, cb.ID, "Ok")
		reply, err := c.Handler.Button(people.With(ctx, person), action, id)
		if err != nil {
			reply = "⚠️ " + err.Error()
		}
		if reply != "" {
			bot.Edit(ctx, cb.Message.Chat.ID, cb.Message.ID, cb.Message.Text+"\n\n→ "+reply)
		}
	}
}

func (c *Channel) read(ctx context.Context, bot Bot, fileID string) (string, error) {
	d, ok := bot.(interface {
		Download(ctx context.Context, fileID string) ([]byte, error)
	})
	if c.ReadPhoto == nil || !ok {
		return "", errors.New("photos are not set up")
	}
	img, err := d.Download(ctx, fileID)
	if err != nil {
		return "", err
	}
	return c.ReadPhoto(ctx, img)
}

func (c *Channel) listen(ctx context.Context, bot Bot, fileID string) (string, error) {
	d, ok := bot.(interface {
		Download(ctx context.Context, fileID string) ([]byte, error)
	})
	if c.Transcribe == nil || !ok {
		return "", errors.New("voice notes are not set up")
	}
	audio, err := d.Download(ctx, fileID)
	if err != nil {
		return "", err
	}
	return c.Transcribe(ctx, audio)
}

func (c *Channel) pair(ctx context.Context, bot Bot, m *telegram.Message, code string) {
	chat, _ := c.Chat(ctx)
	if c.People != nil && code != "" && chat != m.Chat.ID {
		if p, err := c.People.Pair(ctx, code, m.Chat.ID); err == nil {
			c.Events.Append(ctx, EventPaired, "human:"+p.ID, map[string]any{"chat": m.Chat.ID, "person": p.ID})
			bot.Send(ctx, m.Chat.ID, i18n.T(ctx, "msg.pair.person", "name", p.Name))
			return
		}
	}
	if chat != 0 && chat != m.Chat.ID {
		return
	}
	if chat == m.Chat.ID {
		bot.Send(ctx, m.Chat.ID, i18n.T(ctx, "msg.pair.already"))
		return
	}
	if code == "" || code != c.PairingCode() {
		bot.Send(ctx, m.Chat.ID, i18n.T(ctx, "msg.pair.badCode"))
		return
	}
	c.Events.Put(ctx, chatKey, strconv.FormatInt(m.Chat.ID, 10))
	c.Events.Append(ctx, EventPaired, "human:owner", map[string]any{"chat": m.Chat.ID, "name": m.From.FirstName})
	c.mu.Lock()
	c.code = ""
	c.mu.Unlock()
	bot.Send(ctx, m.Chat.ID, i18n.T(ctx, "msg.pair.owner", "name", m.From.FirstName))
}

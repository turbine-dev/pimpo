// Package owner is the conversation with the owner over Telegram: pairing,
// requests that start explorations, and buttons that approve things.
package owner

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/explore"
	"github.com/denerFernandes/vigia/internal/telegram"
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

// Handler reacts to the owner. Button data looks like "compile:<id>".
type Handler interface {
	Request(ctx context.Context, text string) (string, error)
	Button(ctx context.Context, action, id string) (string, error)
}

type Channel struct {
	Events *event.Store
	// Bot returns the current bot, or nil when Telegram is not set up.
	Bot     func(ctx context.Context) Bot
	Handler Handler

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

// Notify sends a notice to the owner and keeps it in the event log, where
// the web inbox shows it even when Telegram is not paired.
func (c *Channel) Notify(ctx context.Context, n explore.Notice) error {
	c.Events.Append(ctx, EventNotice, "system", map[string]any{"text": n.Text, "actions": n.Actions})
	bot := c.Bot(ctx)
	chat, _ := c.Chat(ctx)
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

func (c *Channel) handle(ctx context.Context, bot Bot, u telegram.Update) {
	chat, _ := c.Chat(ctx)
	if u.Message != nil {
		m := u.Message
		text := strings.TrimSpace(m.Text)
		if strings.HasPrefix(text, "/start") {
			c.pair(ctx, bot, m, strings.TrimSpace(strings.TrimPrefix(text, "/start")))
			return
		}
		if chat == 0 || m.Chat.ID != chat {
			// Only the paired owner can talk to Vigia; others get nothing.
			return
		}
		if text == "" {
			return
		}
		reply, err := c.Handler.Request(ctx, text)
		if err != nil {
			reply = "Não consegui começar: " + err.Error()
		}
		bot.Send(ctx, chat, reply)
		return
	}
	if cb := u.Callback; cb != nil {
		if chat == 0 || cb.Message == nil || cb.Message.Chat.ID != chat {
			bot.Answer(ctx, cb.ID, "")
			return
		}
		action, id, _ := strings.Cut(cb.Data, ":")
		bot.Answer(ctx, cb.ID, "Ok")
		reply, err := c.Handler.Button(ctx, action, id)
		if err != nil {
			reply = "⚠️ " + err.Error()
		}
		if reply != "" {
			bot.Edit(ctx, chat, cb.Message.ID, cb.Message.Text+"\n\n→ "+reply)
		}
	}
}

func (c *Channel) pair(ctx context.Context, bot Bot, m *telegram.Message, code string) {
	chat, _ := c.Chat(ctx)
	if chat != 0 && chat != m.Chat.ID {
		return
	}
	if chat == m.Chat.ID {
		bot.Send(ctx, m.Chat.ID, "Já estamos conectados. Me peça algo que você faz toda semana.")
		return
	}
	if code == "" || code != c.PairingCode() {
		bot.Send(ctx, m.Chat.ID, "Esse código não confere. Abra Conexões no Vigia e use o código mostrado lá.")
		return
	}
	c.Events.Put(ctx, chatKey, strconv.FormatInt(m.Chat.ID, 10))
	c.Events.Append(ctx, EventPaired, "human:owner", map[string]any{"chat": m.Chat.ID, "name": m.From.FirstName})
	c.mu.Lock()
	c.code = ""
	c.mu.Unlock()
	bot.Send(ctx, m.Chat.ID, fmt.Sprintf("Oi, %s! Estamos conectados. 👋\n\nMe peça algo que você faz toda semana, por exemplo:\n• \"Todo dia às 7h me manda a agenda e os e-mails importantes\"\n\nNa primeira vez eu faço com você olhando. Depois, faço sozinho.", m.From.FirstName))
}

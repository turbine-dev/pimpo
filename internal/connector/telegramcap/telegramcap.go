// Package telegramcap is the telegram.send capability: messages to the
// owner's chat and nowhere else.
package telegramcap

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/denerFernandes/zodim/internal/connector"
	"github.com/denerFernandes/zodim/internal/telegram"
)

type Sender interface {
	Send(ctx context.Context, chat int64, text string, rows ...[]telegram.Button) (telegram.Message, error)
}

type Owner struct {
	Bot  Sender
	Chat func(ctx context.Context) (int64, error)
}

func (o *Owner) Capabilities() []string { return []string{"telegram.send"} }

func (o *Owner) Call(ctx context.Context, _, _ string, args any) (any, error) {
	var a struct {
		Text string `json:"text"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Text) == "" {
		return nil, errors.New("text is empty")
	}
	chat, err := o.Chat(ctx)
	if err != nil {
		return nil, err
	}
	if chat == 0 {
		return nil, errors.New("Telegram is not paired yet; open Connections to pair it")
	}
	// Telegram caps messages at 4096 characters.
	for _, part := range split(a.Text, 4000) {
		if _, err := o.Bot.Send(ctx, chat, part); err != nil {
			return nil, err
		}
	}
	return map[string]any{"ok": true}, nil
}

func split(s string, n int) []string {
	var out []string
	for utf8.RuneCountInString(s) > n {
		r := []rune(s)
		cut := n
		if i := strings.LastIndex(string(r[:n]), "\n"); i > 0 {
			cut = utf8.RuneCountInString(string(r[:n])[:i])
		}
		out = append(out, string(r[:cut]))
		s = strings.TrimLeft(string(r[cut:]), "\n")
	}
	return append(out, s)
}

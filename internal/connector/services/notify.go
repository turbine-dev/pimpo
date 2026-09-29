package services

import (
	"context"
	"errors"
	"strings"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/connector"
)

// Slack and Discord reach the owner through an incoming webhook of their
// own private channel: messages to the owner, nobody else.
func init() {
	register(Kind{
		ID: "slack", Title: "Slack — avisos num canal", Description: "Avisos num canal seu do Slack.",
		Help:   "Crie um app em api.slack.com/apps com Incoming Webhooks para um canal só seu e cole o endereço do webhook.",
		Fields: []Field{{Name: "webhook", Label: "Webhook", Placeholder: "https://hooks.slack.com/services/…", Secret: true}},
		Specs: []capability.Spec{{Name: "slack.send", Risk: capability.Notify, Signature: "slack.send({text})", Returns: "{ok}; posts to the owner's own Slack channel",
			Schema: obj(`"text":{"type":"string"}`, "text")}},
		Call: func(ctx context.Context, cfg Config, _, _ string, args any) (any, error) {
			return webhook(ctx, cfg, "Slack", "https://hooks.slack.com/", args, func(t string) any { return map[string]string{"text": t} })
		},
	})
	register(Kind{
		ID: "discord", Title: "Discord — avisos num canal", Description: "Avisos num canal seu do Discord.",
		Help:   "No canal, Editar canal › Integrações › Webhooks › Novo webhook, e copie o endereço.",
		Fields: []Field{{Name: "webhook", Label: "Webhook", Placeholder: "https://discord.com/api/webhooks/…", Secret: true}},
		Specs: []capability.Spec{{Name: "discord.send", Risk: capability.Notify, Signature: "discord.send({text})", Returns: "{ok}; posts to the owner's own Discord channel",
			Schema: obj(`"text":{"type":"string"}`, "text")}},
		Call: func(ctx context.Context, cfg Config, _, _ string, args any) (any, error) {
			return webhook(ctx, cfg, "Discord", "https://discord.com/api/webhooks/", args, func(t string) any {
				if r := []rune(t); len(r) > 2000 {
					t = string(r[:1999]) + "…"
				}
				return map[string]any{"content": t, "allowed_mentions": map[string]any{"parse": []string{}}}
			})
		},
	})
}

func webhook(ctx context.Context, cfg Config, title, prefix string, args any, body func(string) any) (any, error) {
	v, err := need(ctx, cfg, title, "webhook")
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(v[0], prefix) && BaseURL[strings.ToLower(title)] == "" {
		return nil, errors.New(title + " webhook must start with " + prefix)
	}
	var a struct {
		Text string `json:"text"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Text) == "" {
		return nil, errEmpty
	}
	if err := doJSON(ctx, "POST", v[0], nil, body(a.Text), nil); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

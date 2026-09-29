package services

import (
	"context"

	"github.com/turbine-dev/pimpo/internal/chatlink"
)

// Conversation channels: set up here like any connector, they let the
// owner talk to Pimpo in private messages. They offer no capabilities.

// LinkKinds lists the catalog entries that are conversation channels.
var LinkKinds = []string{"discordchat", "slackchat", "signal"}

// NewLink builds a channel from its configuration.
func NewLink(ctx context.Context, kind string, cfg Config) (chatlink.Link, error) {
	switch kind {
	case "discordchat":
		v, err := need(ctx, cfg, "Discord", "token")
		if err != nil {
			return nil, err
		}
		return &chatlink.Discord{Token: v[0], API: BaseURL["discordchat"]}, nil
	case "slackchat":
		v, err := need(ctx, cfg, "Slack", "bot_token", "app_token")
		if err != nil {
			return nil, err
		}
		return &chatlink.Slack{BotToken: v[0], AppToken: v[1], API: BaseURL["slackchat"]}, nil
	case "signal":
		url, _ := cfg(ctx, "url")
		acct, _ := cfg(ctx, "account")
		return &chatlink.Signal{URL: url, Account: acct}, nil
	}
	return nil, nil
}

func checkLink(kind string) func(ctx context.Context, cfg Config) error {
	return func(ctx context.Context, cfg Config) error {
		l, err := NewLink(ctx, kind, cfg)
		if err != nil {
			return err
		}
		return l.Check(ctx)
	}
}

func init() {
	register(Kind{
		ID: "discordchat", Title: "Discord", Description: "Converse com o Pimpo por mensagem privada no Discord.",
		Help:   "Em discord.com/developers/applications, crie um app, vá em Bot, gere o token e convide o bot para um servidor seu. Depois mande para ele, no privado: pimpo e o código de pareamento de Conexões.",
		Fields: []Field{{Name: "token", Label: "Token do bot", Secret: true}},
		Probe:  checkLink("discordchat"),
	})
	register(Kind{
		ID: "slackchat", Title: "Slack", Description: "Converse com o Pimpo por mensagem direta no Slack.",
		Help: "Em api.slack.com/apps, crie um app, ligue o Socket Mode (gera o token xapp- com connections:write), assine o evento message.im, dê ao bot im:history, im:write e chat:write e instale no workspace. " +
			"Depois mande para o app, na mensagem direta: pimpo e o código de pareamento de Conexões.",
		Fields: []Field{{Name: "bot_token", Label: "Bot token (xoxb-)", Secret: true}, {Name: "app_token", Label: "App token (xapp-)", Secret: true}},
		Probe:  checkLink("slackchat"),
	})
	register(Kind{
		ID: "signal", Title: "Signal", Description: "Converse com o Pimpo pelo Signal, cifrado até este computador.",
		Help: "Instale o signal-cli, registre ou vincule um número e deixe rodando: signal-cli -a +55NUMERO daemon --http 127.0.0.1:8080. " +
			"Depois mande para esse número, pelo seu Signal: pimpo e o código de pareamento de Conexões.",
		Fields: []Field{{Name: "url", Label: "Endereço do signal-cli", Placeholder: "http://127.0.0.1:8080"}, {Name: "account", Label: "Número do Pimpo no Signal", Placeholder: "+5511…", Optional: true}},
		Probe:  checkLink("signal"),
	})
}

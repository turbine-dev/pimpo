package services

import (
	"context"
	"runtime"

	"github.com/turbine-dev/pimpo/internal/chatlink"
)

// Conversation channels: set up here like any connector, they let the
// owner talk to Pimpo in private messages. They offer no capabilities.

// LinkKinds lists the catalog entries that are conversation channels.
var LinkKinds = []string{"discordchat", "slackchat", "signal", "imessage", "wapersonal"}

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
	case "imessage":
		return &chatlink.IMessage{}, nil
	case "wapersonal":
		url, _ := cfg(ctx, "url")
		session, _ := cfg(ctx, "session")
		key, _ := cfg(ctx, "api_key")
		return &chatlink.WhatsAppPersonal{URL: url, Session: session, APIKey: key}, nil
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
			"Depois mande para o app, na mensagem direta: pimpo e o código de pareamento de Conexões. Para rotinas que reagem ao Slack, assine também message.channels e app_mention, dê ao bot channels:history e app_mentions:read e adicione o app aos canais.",
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
	imessage := Kind{
		ID: "imessage", Title: "iMessage", Description: "Converse com o Pimpo pelo iMessage, no Mac.",
		Help: "Só no Mac. Entre no app Mensagens com um Apple ID (de preferência um só do Pimpo) e dê Acesso Total ao Disco para o Pimpo em Ajustes do Sistema › Privacidade e Segurança, para ele ler as mensagens que chegam. " +
			"Depois mande para esse Apple ID, do seu iPhone: pimpo e o código de pareamento de Conexões.",
		Fields: []Field{{Name: "account", Label: "Apple ID do Pimpo no Mensagens", Placeholder: "pimpo@icloud.com"}},
		Probe:  checkLink("imessage"),
	}
	if runtime.GOOS == "darwin" {
		register(imessage)
	}
	register(Kind{
		ID: "wapersonal", Title: "WhatsApp pessoal (não oficial)", Description: "Converse com o Pimpo pelo seu próprio número de WhatsApp, por uma ponte não oficial.",
		Help: "Atenção: o WhatsApp não permite isso. O número pode ser banido e a ponte para de funcionar sem aviso quando o WhatsApp muda. Use um número que você pode perder; o WhatsApp oficial (Business) em Conexões não tem esse risco. " +
			"Só funciona com o Laboratório › WhatsApp pessoal ligado, e nunca aprova nada: escolhas esperam o app ou outro canal. Rode o WAHA (github.com/devlikeapro/waha) neste computador, entre com o QR code e mande para esse número: pimpo e o código de pareamento de Conexões.",
		Fields: []Field{{Name: "url", Label: "Endereço do WAHA", Placeholder: "http://127.0.0.1:3000"}, {Name: "session", Label: "Sessão", Placeholder: "default", Optional: true}, {Name: "api_key", Label: "Chave da API do WAHA", Secret: true, Optional: true}},
		Probe:  checkLink("wapersonal"),
	})
}

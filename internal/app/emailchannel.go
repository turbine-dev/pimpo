package app

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/denerFernandes/zodim/internal/connector/mail"
	"github.com/denerFernandes/zodim/internal/people"
)

var zodimSubject = regexp.MustCompile(`(?i)^\s*(re:\s*)*zodim\s*[:\-–]\s*`)

func (a *App) emailChannel(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if a.Settings(ctx).EmailChannel {
				a.checkEmailChannel(ctx)
			}
		case <-ctx.Done():
			return
		}
	}
}

// checkEmailChannel reads messages the owner sent to themselves with
// "Zodim:" in the subject and treats each as a request. Only the owner's
// own address counts: anyone can put "Zodim" in a subject.
func (a *App) checkEmailChannel(ctx context.Context) int {
	ctx = people.With(ctx, people.OwnerID)
	me, _ := a.Events.Get(ctx, "mail.user")
	if me == "" {
		return 0
	}
	got, err := a.Router.Call(ctx, "gmail.search", "", map[string]any{"query": "from:" + me + " to:" + me + " subject:zodim", "max": 20})
	if err != nil {
		return 0
	}
	msgs, _ := got.([]mail.Message)
	n := 0
	for _, m := range msgs {
		if !strings.EqualFold(m.From, me) || !zodimSubject.MatchString(m.Subject) {
			continue
		}
		id := m.MessageID
		if id == "" {
			id = m.ID
		}
		key := "email.channel." + id
		if done, _ := a.Events.Get(ctx, key); done != "" {
			continue
		}
		a.Events.Put(ctx, key, time.Now().Format(time.RFC3339))
		text := strings.TrimSpace(zodimSubject.ReplaceAllString(m.Subject, "") + "\n" + m.Snippet)
		reply, err := handler{a}.Request(ctx, text)
		if err != nil {
			reply = "Não consegui começar: " + err.Error()
		}
		a.Router.Call(ctx, "gmail.archive", "", map[string]any{"id": m.ID})
		// Sending needs a known server: one set up, or Gmail's default.
		smtp, _ := a.Events.Get(ctx, "mail.smtp")
		if imapAddr, _ := a.Events.Get(ctx, "mail.addr"); smtp != "" || strings.HasPrefix(imapAddr, "imap.gmail.com") {
			a.Router.Call(ctx, "gmail.send", "", map[string]any{"to": me, "subject": "Re: " + m.Subject, "body": reply + "\n\nO resultado chega no Telegram, no WhatsApp ou em Precisa de você."})
		}
		n++
	}
	return n
}

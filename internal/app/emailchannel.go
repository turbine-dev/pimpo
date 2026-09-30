package app

import (
	"context"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"regexp"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector/mail"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/people"
)

// pimpoSubject also accepts the old name, so saved habits keep working.
var pimpoSubject = regexp.MustCompile(`(?i)^\s*(re:\s*)*(pimpo|zodim)\s*[:\-–]\s*`)

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
// "Pimpo:" in the subject and treats each as a request. Only the owner's
// own address counts: anyone can put "Pimpo" in a subject, and anyone
// can write the owner's address in From, so the provider must have
// checked the sender too (fromOwner).
func (a *App) checkEmailChannel(ctx context.Context) int {
	ctx = people.With(ctx, people.OwnerID)
	me, _ := a.Events.Get(ctx, "mail.user")
	if me == "" {
		return 0
	}
	got, err := a.Router.Call(ctx, "gmail.search", "", map[string]any{"query": "from:" + me + " to:" + me + " subject:pimpo", "max": 20})
	if err != nil {
		return 0
	}
	msgs, _ := got.([]mail.Message)
	n := 0
	for _, m := range msgs {
		if !strings.EqualFold(m.From, me) || !pimpoSubject.MatchString(m.Subject) || !fromOwner(m, me) {
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
		text := strings.TrimSpace(pimpoSubject.ReplaceAllString(m.Subject, "") + "\n" + m.Snippet)
		// Facts noted while answering come from this email.
		from := memory.WithOrigin(ctx, memory.Origin{Kind: memory.FromEmail, Ref: id, Sender: me, Label: m.Subject})
		reply, err := handler{a}.Request(from, text)
		if err != nil {
			reply = i18n.T(ctx, "msg.start.failed", "error", err)
		}
		a.Router.Call(ctx, "gmail.archive", "", map[string]any{"id": m.ID})
		// Sending needs a known server: one set up, or Gmail's default.
		smtp, _ := a.Events.Get(ctx, "mail.smtp")
		if imapAddr, _ := a.Events.Get(ctx, "mail.addr"); smtp != "" || strings.HasPrefix(imapAddr, "imap.gmail.com") {
			a.Router.Call(ctx, "gmail.send", "", map[string]any{"to": me, "subject": "Re: " + m.Subject, "body": reply + "\n\n" + i18n.T(ctx, "msg.email.where")})
		}
		n++
	}
	return n
}

// fromOwner says whether the mail provider vouched for the sender: its
// Authentication-Results show DKIM passing for the owner's domain, or
// DMARC passing for it. Only the top header counts, the one the
// receiving provider adds; any below it came with the message and could
// be forged. No header means no proof, so the message is not taken.
func fromOwner(m mail.Message, me string) bool {
	_, domain, ok := strings.Cut(strings.ToLower(me), "@")
	if !ok || domain == "" || len(m.AuthResults) == 0 {
		return false
	}
	aligned := func(d string) bool {
		d = strings.TrimPrefix(strings.ToLower(strings.Trim(d, `"`)), "@")
		if _, after, found := strings.Cut(d, "@"); found {
			d = after
		}
		return strings.Contains(d, ".") && (d == domain || strings.HasSuffix(domain, "."+d))
	}
	for _, res := range strings.Split(stripComments(m.AuthResults[0]), ";")[1:] {
		fields := strings.Fields(res)
		if len(fields) == 0 {
			continue
		}
		method, result, _ := strings.Cut(strings.ToLower(fields[0]), "=")
		if result != "pass" {
			continue
		}
		for _, prop := range fields[1:] {
			k, v, _ := strings.Cut(prop, "=")
			switch k = strings.ToLower(k); {
			case method == "dkim" && (k == "header.d" || k == "header.i") && aligned(v):
				return true
			case method == "dmarc" && k == "header.from" && strings.EqualFold(strings.Trim(v, `"`), domain):
				return true
			}
		}
	}
	return false
}

// stripComments drops the (comments) of a header value.
func stripComments(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

package app

import (
	"context"
	"fmt"
	"github.com/denerFernandes/pimpo/internal/i18n"
	ownerpkg "github.com/denerFernandes/pimpo/internal/owner"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/denerFernandes/pimpo/internal/chatlink"
	"github.com/denerFernandes/pimpo/internal/connector/services"
	"github.com/denerFernandes/pimpo/internal/explore"
	"github.com/denerFernandes/pimpo/internal/people"
)

// Conversation channels beyond Telegram and WhatsApp: Discord, Slack and
// Signal. The owner pairs by sending "pimpo <code>" in a private message;
// strangers get nothing. These services have no buttons, so choices come
// numbered and the owner answers with the number.

type linkRun struct {
	link   chatlink.Link
	cancel context.CancelFunc
	// pending are the choices of the last notice, answered by number.
	mu      sync.Mutex
	pending []explore.Action
}

var linksMu sync.Mutex

func linkOwnerKey(kind string) string { return kind + ".owner" }

// startLinks (re)starts every configured channel.
func (a *App) startLinks(ctx context.Context) {
	for _, k := range services.LinkKinds {
		a.restartLink(ctx, k)
	}
}

func (a *App) restartLink(ctx context.Context, kind string) {
	linksMu.Lock()
	defer linksMu.Unlock()
	if a.links == nil {
		a.links = map[string]*linkRun{}
	}
	if old := a.links[kind]; old != nil {
		old.cancel()
		delete(a.links, kind)
		a.health.forget(kind)
	}
	k, _ := services.Get(kind)
	if ok, _ := a.catalogConfigured(ctx, k); !ok {
		return
	}
	l, err := services.NewLink(ctx, kind, a.catalogConfig(kind))
	if err != nil || l == nil {
		return
	}
	lctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	run := &linkRun{link: l, cancel: cancel}
	a.links[kind] = run
	go chatlink.Keep(lctx, l, func(in chatlink.Inbound) { a.linkMessage(lctx, kind, run, in) }, func(err error) {
		a.health.report(kind, err)
		if err == nil {
			return
		}
		a.Events.Append(lctx, "channel.failed", "system", map[string]string{"channel": kind, "error": err.Error()})
	})
}

func (a *App) linkMessage(ctx context.Context, kind string, run *linkRun, in chatlink.Inbound) {
	reply := func(text string) { run.link.Send(ctx, in.From, text) }
	text := strings.TrimSpace(in.Text)
	owner, _ := a.Events.Get(ctx, linkOwnerKey(kind))
	if code, ok := pairingCode(text); ok {
		if owner == "" && code == a.Channel.PairingCode() {
			a.Events.Put(ctx, linkOwnerKey(kind), in.From)
			a.Events.Append(ctx, "channel.paired", "human:owner", map[string]string{"channel": kind})
			reply(i18n.T(ctx, "msg.pair.link"))
		} else if owner == "" {
			reply(i18n.T(ctx, "msg.pair.linkBad"))
		}
		return
	}
	if owner == "" || in.From != owner {
		return
	}
	pctx := people.With(ctx, people.OwnerID)
	h := handler{a}
	if n, err := strconv.Atoi(text); err == nil {
		run.mu.Lock()
		pending := run.pending
		run.mu.Unlock()
		if n >= 1 && n <= len(pending) {
			action, id, _ := strings.Cut(pending[n-1].Data, ":")
			out, err := h.Button(pctx, action, id)
			if err != nil {
				out = "⚠️ " + err.Error()
			}
			reply(out)
			return
		}
	}
	rctx := ownerpkg.Via(pctx, kind)
	if t, ok := run.link.(chatlink.Typer); ok {
		from := in.From
		rctx = ownerpkg.WithTyping(rctx, func(ctx context.Context) error { return t.Typing(ctx, from) })
	}
	out, err := h.Request(rctx, text)
	if err != nil {
		out = i18n.T(ctx, "msg.start.failed", "error", err)
	}
	reply(out)
}

// mirrorLinks sends the owner's notices to every paired channel, with the
// choices numbered.
func (a *App) mirrorLinks(ctx context.Context, n explore.Notice) {
	if people.Norm(n.To) != people.OwnerID {
		return
	}
	linksMu.Lock()
	runs := map[string]*linkRun{}
	for k, r := range a.links {
		runs[k] = r
	}
	linksMu.Unlock()
	for kind, run := range runs {
		owner, _ := a.Events.Get(ctx, linkOwnerKey(kind))
		if owner == "" {
			continue
		}
		text := n.Text
		if len(n.Actions) > 0 {
			var opts []string
			for i, act := range n.Actions {
				opts = append(opts, fmt.Sprintf("%d = %s", i+1, act.Label))
			}
			text += "\n\n" + i18n.T(ctx, "msg.link.choose", "options", strings.Join(opts, " · "))
			run.mu.Lock()
			run.pending = slices.Clone(n.Actions)
			run.mu.Unlock()
		}
		go func(kind string, run *linkRun) {
			a.health.report(kind, run.link.Send(context.WithoutCancel(ctx), owner, text))
		}(kind, run)
	}
}

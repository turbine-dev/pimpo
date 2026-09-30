package app

import (
	"context"
	"fmt"
	"github.com/turbine-dev/pimpo/internal/i18n"
	ownerpkg "github.com/turbine-dev/pimpo/internal/owner"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/chatlink"
	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/people"
)

// Conversation channels beyond Telegram and WhatsApp: Discord, Slack and
// Signal. The owner pairs by sending "pimpo <code>" in a private message;
// strangers get nothing. These services have no buttons, so choices come
// numbered and the owner answers with the number.
//
// A bare number is easy to send by mistake or for the wrong notice, so it
// only answers the latest notice, once, within choiceLife; when a newer
// notice has just replaced unanswered choices, the number is not taken
// and the new choices are shown again. Choices that make a lasting rule
// ("always") are never offered by number: those are made in the app.
//
// Where the service has replies (Discord, Slack, Signal), replying to a
// notice answers that notice, however old and whatever came after it. A
// reply names its notice exactly, so it needs none of the guesses above.
// A reply in words to a question is checked against its options.

const (
	choiceLife   = 10 * time.Minute
	choiceSettle = time.Minute
	// noticesKept is how many sent notices a reply can still answer.
	noticesKept = 100
)

// sentNotice is a notice sent with choices, answered by replying to it.
type sentNotice struct {
	choices  []explore.Action
	answered bool
}

type linkRun struct {
	link   chatlink.Link
	cancel context.CancelFunc
	// pending are the choices of the last notice, answered by number.
	mu       sync.Mutex
	pending  []explore.Action
	asked    string
	at       time.Time
	replaced bool
	// sent are the notices with choices, by the ids of their messages.
	sent  map[string]*sentNotice
	order []string
}

// remember keeps a sent notice so a reply to any of its messages answers
// it, forgetting the oldest beyond noticesKept.
func (r *linkRun) remember(ids []string, choices []explore.Action) {
	if len(ids) == 0 || len(choices) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sent == nil {
		r.sent = map[string]*sentNotice{}
	}
	n := &sentNotice{choices: slices.Clone(choices)}
	for _, id := range ids {
		r.sent[id] = n
		r.order = append(r.order, id)
	}
	for len(r.order) > noticesKept {
		delete(r.sent, r.order[0])
		r.order = r.order[1:]
	}
}

// replyChoice takes the n-th choice of the notice a reply names and uses
// it up. known says whether that message is a notice Pimpo still has;
// answered says it was already answered.
func (r *linkRun) replyChoice(id string, n int) (act explore.Action, known, answered bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sn := r.sent[id]
	if sn == nil {
		return act, false, false
	}
	if sn.answered {
		return act, true, true
	}
	if n < 1 || n > len(sn.choices) {
		return act, true, false
	}
	sn.answered = true
	act = sn.choices[n-1]
	// The same choices waiting for a bare number are answered too.
	if slices.EqualFunc(r.pending, sn.choices, func(x, y explore.Action) bool { return x.Data == y.Data }) {
		r.pending = nil
	}
	return act, true, false
}

// replied are the choices of the notice a reply names, while it is still
// known and unanswered.
func (r *linkRun) replied(id string) []explore.Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sn := r.sent[id]; sn != nil && !sn.answered {
		return slices.Clone(sn.choices)
	}
	return nil
}

// offer makes a notice's choices the ones a number answers; a notice
// without choices clears them.
func (r *linkRun) offer(text string, actions []explore.Action) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// The same choices again (a notice mirrored twice) leave nothing to
	// mix up.
	same := slices.EqualFunc(r.pending, actions, func(x, y explore.Action) bool { return x.Data == y.Data })
	r.replaced = len(r.pending) > 0 && !same && time.Since(r.at) < choiceLife
	r.pending, r.asked, r.at = slices.Clone(actions), text, time.Now()
	if len(actions) == 0 {
		r.pending, r.replaced = nil, false
	}
}

// choose takes the n-th choice and uses it up. again is set when the
// number may have been meant for the notice before: send it and wait.
func (r *linkRun) choose(n int) (act explore.Action, again string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) == 0 || time.Since(r.at) > choiceLife {
		r.pending = nil
		return act, "", false
	}
	if n < 1 || n > len(r.pending) {
		return act, "", false
	}
	if r.replaced && time.Since(r.at) < choiceSettle {
		r.replaced = false
		return act, r.asked, false
	}
	act = r.pending[n-1]
	r.pending = nil
	return act, "", true
}

// numbered are the choices a number may answer, and how they read.
func numbered(ctx context.Context, actions []explore.Action) ([]explore.Action, string) {
	var keep []explore.Action
	var opts []string
	for _, act := range actions {
		if strings.HasPrefix(act.Data, "always:") {
			continue
		}
		keep = append(keep, act)
		opts = append(opts, fmt.Sprintf("%d = %s", len(keep), act.Label))
	}
	if len(keep) == 0 {
		return nil, ""
	}
	return keep, i18n.T(ctx, "msg.link.choose", "options", strings.Join(opts, " · "))
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
	k, found := services.Get(kind)
	if !found {
		return // iMessage off a Mac
	}
	if ok, _ := a.catalogConfigured(ctx, k); !ok {
		return
	}
	// The unofficial WhatsApp runs only when the owner chose it in Labs,
	// knowing the number can be banned.
	if kind == "wapersonal" && !a.chose(ctx, "whatsapp_personal") {
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
		// A wrong code gets no answer: guessing teaches nothing.
		sender := kind + ":" + in.From
		if owner != "" || a.Channel.Blocked(sender) {
			return
		}
		if !a.Channel.ClaimOwner(code) {
			a.Channel.Wrong(sender)
			return
		}
		a.Events.Put(ctx, linkOwnerKey(kind), in.From)
		a.Events.Append(ctx, "channel.paired", "human:owner", map[string]string{"channel": kind})
		reply(i18n.T(ctx, "msg.pair.link"))
		return
	}
	if owner == "" || in.From != owner {
		return
	}
	pctx := people.With(ctx, people.OwnerID)
	h := handler{a}
	if _, err := strconv.Atoi(text); err != nil && in.ReplyTo != "" && !noApprovals(run.link) {
		// Words in reply to a question are checked against its options.
		if out, ok := h.Reply(pctx, run.replied(in.ReplyTo), text); ok {
			reply(out)
			return
		}
	}
	if n, err := strconv.Atoi(text); err == nil && in.ReplyTo != "" && !noApprovals(run.link) {
		act, known, answered := run.replyChoice(in.ReplyTo, n)
		switch {
		case answered:
			reply(i18n.T(ctx, "msg.link.answered"))
		case !known:
			// Never fall back to the latest notice: the reply meant
			// another one.
			reply(i18n.T(ctx, "msg.link.unknownNotice"))
		case act.Data == "":
			reply(i18n.T(ctx, "msg.link.noSuchChoice"))
		default:
			action, id, _ := strings.Cut(act.Data, ":")
			out, err := h.Button(pctx, action, id)
			if err != nil {
				out = "⚠️ " + err.Error()
			}
			reply(out)
		}
		return
	}
	if n, err := strconv.Atoi(text); err == nil && !noApprovals(run.link) {
		act, again, ok := run.choose(n)
		if again != "" {
			reply(again)
			return
		}
		if ok {
			action, id, _ := strings.Cut(act.Data, ":")
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

func noApprovals(l chatlink.Link) bool {
	n, ok := l.(chatlink.NoApprovals)
	return ok && n.NoApprovals()
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
		if len(n.Actions) > 0 && noApprovals(run.link) {
			// A channel that can break or be taken over is never the way
			// to approve: the choice waits for the app or another channel.
			text += "\n\n" + i18n.T(ctx, "msg.link.approveElsewhere")
		}
		var choices []explore.Action
		if len(n.Actions) > 0 && !noApprovals(run.link) {
			var line string
			if choices, line = numbered(ctx, n.Actions); len(choices) > 0 {
				text += "\n\n" + line
			}
		}
		if !noApprovals(run.link) {
			run.offer(text, choices)
		}
		go func(kind string, run *linkRun, text string, choices []explore.Action) {
			ctx := context.WithoutCancel(ctx)
			if r, ok := run.link.(chatlink.Replier); ok {
				ids, err := r.SendMessage(ctx, owner, text)
				run.remember(ids, choices)
				a.health.report(kind, err)
				return
			}
			a.health.report(kind, run.link.Send(ctx, owner, text))
		}(kind, run, text, choices)
	}
}

package app

import (
	"context"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/chatlink"
	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Slack events for routines: the Slack link is the owner's, and over its
// Socket Mode connection Slack already sends messages in the channels the
// app was added to and mentions of it. Those are kept only while one of
// the owner's routines watches slack.messages, and reach routines as data
// (event.items), never as a conversation with Pimpo.

const slackKeep = 14 * 24 * time.Hour

// slackUp says whether the Slack link is connected.
func (a *App) slackUp() bool {
	linksMu.Lock()
	run := a.links["slackchat"]
	linksMu.Unlock()
	if run == nil {
		return false
	}
	down, _ := a.health.down("slackchat")
	return !down
}

// slackWatched says whether any of the owner's active routines watches
// Slack, so channel messages are kept only when something uses them.
func (a *App) slackWatched(ctx context.Context) bool {
	routines, _ := a.Store.Routines(ctx)
	for _, r := range routines {
		if r.State == store.RoutineActive && pushKind(r) == "slack" && people.Norm(r.Person) == people.OwnerID {
			return true
		}
	}
	return false
}

// slackEvent keeps a channel message or mention and checks at once the
// owner's routines that watch Slack with push on.
func (a *App) slackEvent(ctx context.Context, e chatlink.Event) {
	if e.TS == "" || !a.slackWatched(ctx) {
		return
	}
	mention := ""
	if e.Mention {
		mention = "true"
	}
	a.Events.Append(ctx, "slack.message", "system", map[string]string{"id": e.Channel + ":" + e.TS, "channel": e.Channel, "user": e.User,
		"text": clip(e.Text, 4000), "thread": e.ThreadTS, "mention": mention, "person": people.OwnerID})
	a.pokePushed(people.With(ctx, people.OwnerID), "slack")
}

// slackCap lets the owner's routines read what Slack sent.
type slackCap struct{ a *App }

func (slackCap) Capabilities() []string { return []string{"slack.messages"} }

func (c slackCap) Call(ctx context.Context, _, _ string, args any) (any, error) {
	var in struct {
		Channel  string `json:"channel"`
		Mentions bool   `json:"mentions"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	out := []map[string]any{}
	// The Slack link is the owner's: nobody else reads its channels.
	if people.From(ctx) != people.OwnerID {
		return out, nil
	}
	evs, err := c.a.Events.List(ctx, event.Query{Types: []string{"slack.message"}, Newest: true, Limit: 400})
	if err != nil {
		return nil, err
	}
	since := time.Now().Add(-slackKeep)
	byID := map[string]map[string]any{}
	for _, e := range evs {
		if e.Time.Before(since) || len(out) == 50 {
			break
		}
		var d map[string]string
		e.Decode(&d)
		if d["person"] != people.OwnerID || (in.Channel != "" && !strings.EqualFold(in.Channel, d["channel"])) {
			continue
		}
		// A mention arrives both as a message and as a mention.
		if seen := byID[d["id"]]; seen != nil {
			seen["mention"] = seen["mention"] == true || d["mention"] == "true"
			continue
		}
		item := map[string]any{"id": d["id"], "channel": d["channel"], "user": d["user"], "text": d["text"], "thread": d["thread"],
			"mention": d["mention"] == "true", "time": e.Time.Format(time.RFC3339)}
		byID[d["id"]] = item
		out = append(out, item)
	}
	if in.Mentions {
		kept := out[:0]
		for _, it := range out {
			if it["mention"] == true {
				kept = append(kept, it)
			}
		}
		out = kept
	}
	return out, nil
}

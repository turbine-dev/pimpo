package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/chatlink"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/scheduler"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Lasting progress: every long job and routine run keeps a record of
// where it is (state, parts or steps done, the step it is on by Pimpo's
// own names, the cost so far), saved as it goes, so a reload, the phone
// or a restart shows the same thing. Each change also goes out on the
// live stream as "progress.updated" with the person in its data, so only
// that person sees it whole. A job the person follows keeps one message
// up to date on their channels: edited in place where the service can
// (Telegram, Discord, Slack), at most every progressEvery; elsewhere
// only the start, and the notice at the end.

const (
	EventProgress = "progress.updated"
	// progressEvery is how often a followed job's message is edited.
	progressEvery = 10 * time.Second
	// progressSaveEvery is how often a routine's steps are saved; the
	// start and the end always are.
	progressSaveEvery = time.Second
	progressLabel     = 60
)

type progressState struct {
	mu    sync.Mutex
	saved map[string]time.Time
	// follows are the channel messages of followed jobs, by record id.
	follows map[string]*follower
}

func (a *App) editEvery() time.Duration {
	if a.ProgressEvery > 0 {
		return a.ProgressEvery
	}
	return progressEvery
}

// saveProgress keeps a record and tells the stream. A step that comes
// after the end, or a routine step too soon after the last one saved,
// is dropped: the end is what counts.
func (a *App) saveProgress(ctx context.Context, p store.Progress, follow bool) {
	ctx = context.WithoutCancel(ctx)
	p.Person = people.Norm(p.Person)
	p.Label = clip(p.Label, progressLabel)
	now := time.Now().UTC()
	a.progress.mu.Lock()
	if a.progress.saved == nil {
		a.progress.saved = map[string]time.Time{}
	}
	if p.Kind == "run" && !p.Final() && p.Done > 0 && now.Sub(a.progress.saved[p.ID]) < progressSaveEvery {
		a.progress.mu.Unlock()
		return
	}
	old, err := a.Store.Progress(ctx, p.ID)
	if err == nil && old.Final() && !p.Final() && p.Kind == "run" {
		a.progress.mu.Unlock()
		return
	}
	if err == nil && !old.StartedAt.IsZero() {
		p.StartedAt = old.StartedAt
	}
	if p.StartedAt.IsZero() {
		p.StartedAt = now
	}
	p.UpdatedAt = now
	if p.Final() && p.EndedAt.IsZero() {
		p.EndedAt = now
	}
	if p.Final() {
		delete(a.progress.saved, p.ID)
	} else {
		a.progress.saved[p.ID] = now
	}
	a.Store.SaveProgress(ctx, p)
	a.progress.mu.Unlock()
	a.Events.Publish(EventProgress, "system", p)
	if follow {
		go a.follow(ctx, p)
	}
}

// jobProgress is a job's record, from the job itself. A planned job has
// none yet.
func (a *App) jobProgress(ctx context.Context, j Job) {
	if j.State == JobPlanned {
		return
	}
	id := "job:" + j.ID
	if j.State == JobStopped {
		if _, err := a.Store.Progress(ctx, id); err != nil {
			return // stopped before it started
		}
	}
	p := store.Progress{ID: id, Person: j.Person, Kind: "job", Job: j.ID, Title: clip(j.Request, 120), Total: len(j.Parts),
		CostUSD: j.SpentUSD, Resumed: !j.Resumed.IsZero(), Error: clip(j.Error, 200)}
	var running []string
	for _, part := range j.Parts {
		switch part.State {
		case PartDone:
			p.Done++
		case PartRunning:
			running = append(running, part.Title)
		}
	}
	p.Label = strings.Join(running, ", ")
	switch j.State {
	case JobRunning:
		p.State = store.ProgressRunning
	case JobReporting:
		p.State, p.Phase, p.Label = store.ProgressRunning, "reporting", ""
	case JobDone:
		p.State = store.ProgressDone
	case JobStopped:
		p.State, p.Phase = store.ProgressFailed, "stopped"
	default:
		p.State = store.ProgressFailed
	}
	a.saveProgress(ctx, p, j.Follow)
}

// runProgress keeps a routine run's record as the scheduler reports it.
func (a *App) runProgress(ctx context.Context, r scheduler.RunProgress) {
	p := store.Progress{ID: fmt.Sprintf("run:%s:%d", r.Routine, r.Run), Person: r.Person, Kind: "run", Routine: r.Routine, Run: r.Run,
		Title: clip(r.Name, 120), Label: r.Step, Done: r.Steps, CostUSD: r.CostUSD, StartedAt: r.Started, Error: clip(r.Error, 200)}
	switch r.State {
	case store.RunRunning:
		p.State = store.ProgressRunning
	case store.RunFailed:
		p.State = store.ProgressFailed
	default:
		p.State = store.ProgressDone
	}
	a.saveProgress(ctx, p, false)
}

// settleProgress, at start, ends the records of runs a restart cut
// short. Jobs pick up again (resumeJobs), and say so.
func (a *App) settleProgress(ctx context.Context) {
	active, _ := a.Store.AllActiveProgress(ctx)
	for _, p := range active {
		if !p.UpdatedAt.Before(a.startedAt) || p.Kind != "run" {
			continue
		}
		p.State, p.Phase, p.Label, p.Error = store.ProgressFailed, "interrupted", "", "interrupted by a restart"
		a.Store.FinishRun(ctx, p.Run, store.RunFailed, p.Error, p.CostUSD, p.Done)
		a.saveProgress(ctx, p, false)
	}
}

func (a *App) progressRoutes() {
	// Only the person's own records: nobody sees another's progress.
	a.Server.Handle("GET /api/progress", func(w http.ResponseWriter, r *http.Request) {
		me := people.Norm(people.From(r.Context()))
		var out []store.Progress
		var err error
		if r.URL.Query().Get("active") != "" {
			out, err = a.Store.ActiveProgress(r.Context(), me)
		} else {
			out, err = a.Store.RecentProgress(r.Context(), me, 30)
		}
		respond(w)(out, err)
	})
}

// progressText is how a record reads on a channel.
func progressText(ctx context.Context, p store.Progress) string {
	icon := "⏳"
	switch p.State {
	case store.ProgressDone:
		icon = "✅"
	case store.ProgressFailed:
		icon = "⚠️"
	}
	var bits []string
	if p.Total > 0 {
		bits = append(bits, i18n.T(ctx, "msg.progress.parts", "done", p.Done, "total", p.Total))
	} else if p.Done > 0 {
		bits = append(bits, i18n.T(ctx, "msg.progress.steps", "count", p.Done))
	}
	switch {
	case p.Phase != "":
		bits = append(bits, i18n.T(ctx, "msg.progress."+p.Phase))
	case p.State == store.ProgressDone:
		bits = append(bits, i18n.T(ctx, "msg.progress.done"))
	case p.State == store.ProgressFailed:
		bits = append(bits, i18n.T(ctx, "msg.progress.failed"))
	case p.Label != "":
		bits = append(bits, p.Label)
	}
	bits = append(bits, fmt.Sprintf("$%.2f", p.CostUSD))
	text := icon + " " + p.Title + "\n" + strings.Join(bits, " · ")
	if p.Resumed && !p.Final() {
		text += "\n" + i18n.T(ctx, "msg.progress.resumed")
	}
	return text
}

// A progressSink is one channel a person follows work on. edit is nil
// where the service cannot change a message it sent.
type progressSink struct {
	name string
	send func(ctx context.Context, text string) (string, error)
	edit func(ctx context.Context, id, text string) error
}

// progressSinks are the person's own channels: their Telegram chat and
// WhatsApp number, and, for the administrator, the paired links.
func (a *App) progressSinks(ctx context.Context, person string) []progressSink {
	if a.ProgressSinks != nil {
		return a.ProgressSinks(ctx, person)
	}
	person = people.Norm(person)
	var out []progressSink
	if bot := a.bot(ctx); bot != nil {
		if chat := a.Channel.ChatOf(ctx, person); chat != 0 {
			out = append(out, progressSink{name: "telegram",
				send: func(ctx context.Context, text string) (string, error) {
					m, err := bot.Send(ctx, chat, text)
					return strconv.FormatInt(m.ID, 10), err
				},
				edit: func(ctx context.Context, id, text string) error {
					n, _ := strconv.ParseInt(id, 10, 64)
					return bot.Edit(ctx, chat, n, text)
				}})
		}
	}
	if c := a.wa(ctx); c != nil {
		if p, err := a.People.Get(ctx, person); err == nil && p.WhatsApp != "" {
			out = append(out, progressSink{name: "whatsapp", send: func(ctx context.Context, text string) (string, error) {
				return "", c.Send(ctx, p.WhatsApp, text)
			}})
		}
	}
	if person != people.OwnerID {
		return out
	}
	linksMu.Lock()
	runs := map[string]*linkRun{}
	for k, r := range a.links {
		runs[k] = r
	}
	linksMu.Unlock()
	for kind, run := range runs {
		to, _ := a.Events.Get(ctx, linkOwnerKey(kind))
		if to == "" {
			continue
		}
		out = append(out, linkSink(kind, run.link, to))
	}
	return out
}

func linkSink(kind string, l chatlink.Link, to string) progressSink {
	s := progressSink{name: kind, send: func(ctx context.Context, text string) (string, error) {
		if r, ok := l.(chatlink.Replier); ok {
			ids, err := r.SendMessage(ctx, to, text)
			if len(ids) > 0 {
				return ids[0], err
			}
			return "", err
		}
		return "", l.Send(ctx, to, text)
	}}
	if e, ok := l.(chatlink.Editor); ok {
		s.edit = func(ctx context.Context, id, text string) error { return e.Edit(ctx, to, id, text) }
	}
	return s
}

// A follower keeps a followed job's messages: which were sent, what they
// show and when they last changed.
type follower struct {
	mu      sync.Mutex
	posts   map[string]string
	started bool
	shown   string
	want    string
	final   bool
	updated time.Time
	last    time.Time
	timer   *time.Timer
}

func postsKey(id string) string { return "progress.posts." + id }

func (a *App) follower(ctx context.Context, id string) *follower {
	a.progress.mu.Lock()
	defer a.progress.mu.Unlock()
	if a.progress.follows == nil {
		a.progress.follows = map[string]*follower{}
	}
	f := a.progress.follows[id]
	if f == nil {
		f = &follower{posts: map[string]string{}}
		// After a restart the same messages go on being edited.
		if raw, _ := a.Events.Get(ctx, postsKey(id)); raw != "" {
			f.started = json.Unmarshal([]byte(raw), &f.posts) == nil
		}
		a.progress.follows[id] = f
	}
	return f
}

// follow brings a followed job's messages up to date: the first sends
// them, then edits wait until progressEvery has passed since the last,
// and the end is shown at once.
func (a *App) follow(ctx context.Context, p store.Progress) {
	f := a.follower(ctx, p.ID)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.final || p.UpdatedAt.Before(f.updated) {
		return
	}
	f.want, f.final, f.updated = progressText(ctx, p), p.Final(), p.UpdatedAt
	person := p.Person
	flush := func() { a.flushFollow(ctx, p.ID, person, f) }
	wait := a.editEvery() - time.Since(f.last)
	switch {
	case !f.started || f.final || wait <= 0:
		if f.timer != nil {
			f.timer.Stop()
			f.timer = nil
		}
		go flush()
	case f.timer == nil:
		f.timer = time.AfterFunc(wait, flush)
	}
}

func (a *App) flushFollow(ctx context.Context, id, person string, f *follower) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timer = nil
	if f.want == f.shown {
		return
	}
	sinks := a.progressSinks(ctx, person)
	if !f.started {
		for _, s := range sinks {
			// A failed send is about that message, not the channel.
			if msg, err := s.send(ctx, f.want); err == nil && msg != "" {
				f.posts[s.name] = msg
			}
		}
		f.started = true
	} else {
		for _, s := range sinks {
			// A channel that cannot edit had the start; the notice at the
			// end tells it the rest.
			if msg := f.posts[s.name]; msg != "" && s.edit != nil {
				s.edit(ctx, msg, f.want)
			}
		}
	}
	f.shown, f.last = f.want, time.Now()
	if f.final {
		// The follower stays, ended, so a late step cannot start over.
		a.Events.Put(ctx, postsKey(id), "")
		return
	}
	b, _ := json.Marshal(f.posts)
	a.Events.Put(ctx, postsKey(id), string(b))
}

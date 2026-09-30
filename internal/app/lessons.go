package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/memory"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Lessons are what Pimpo noticed and proposes to keep, in one place per
// person: a preference it learned, a note it took, a task asked often
// enough to become a routine, a repair that fixed a routine. A lesson is
// only a proposal; accepting it goes through the same path as doing it by
// hand (confirming the fact, compiling the exploration with its checks),
// editing changes the text first, and rejecting it is remembered so the
// same lesson never comes back.
//
// Lessons are read from where they already live (memory, explorations,
// suggestions), so the Memory page, the inbox and this list always agree;
// only the decisions and the evidence of learned preferences are kept here.

const (
	lessonPreference = "preference"
	lessonRoutine    = "routine"
	lessonFix        = "fix"
	lessonFact       = "fact"

	lessonProposed = "proposed"
	lessonAccepted = "accepted"
	lessonEdited   = "edited"
	lessonRejected = "rejected"

	lessonsKey       = "lessons"
	lessonDigestKey  = "lessons.digest"
	lessonDigestHour = 10
	lessonKeepSeen   = 1000
	lessonKeepDone   = 50
)

// lessonLink points at what a lesson came from: an app path.
type lessonLink struct {
	Kind string `json:"kind"` // exploration, routine, memory, suggestion
	To   string `json:"to"`
}

type lesson struct {
	ID     string `json:"id"`
	Person string `json:"person"`
	Kind   string `json:"kind"`
	// From says which producer proposed it: learned, note, repeated,
	// suggestion or repair.
	From     string       `json:"from"`
	Title    string       `json:"title"`
	Detail   string       `json:"detail,omitempty"`
	Evidence []lessonLink `json:"evidence"`
	// Change is exactly what accepting keeps: the preference or fact's
	// text, the request a routine does, what the repaired routine did.
	Change string `json:"change"`
	// Ref is what the change applies to: a fact, an exploration or a
	// suggestion; Routine is the routine a fix is for.
	Ref     string     `json:"ref"`
	Routine string     `json:"routine,omitempty"`
	State   string     `json:"state"`
	Edited  string     `json:"edited,omitempty"`
	Result  string     `json:"result,omitempty"`
	Created time.Time  `json:"created"`
	Decided *time.Time `json:"decided,omitempty"`
	Print   string     `json:"-"`
}

// lessonBook is what is kept per person: the fingerprints of every lesson
// decided, the latest decisions to show, and the requests a learned
// preference came from.
type lessonBook struct {
	Seen     []string                `json:"seen"`
	Done     []lesson                `json:"done"`
	Evidence map[string][]lessonLink `json:"evidence,omitempty"`
}

var lessonsMu sync.Mutex

func (a *App) lessonBook(ctx context.Context) lessonBook {
	raw, _ := a.Events.Get(ctx, personal(ctx, lessonsKey))
	var b lessonBook
	json.Unmarshal([]byte(raw), &b)
	return b
}

func (a *App) saveLessonBook(ctx context.Context, b lessonBook) {
	if len(b.Seen) > lessonKeepSeen {
		b.Seen = b.Seen[len(b.Seen)-lessonKeepSeen:]
	}
	if len(b.Done) > lessonKeepDone {
		b.Done = b.Done[:lessonKeepDone]
	}
	raw, _ := json.Marshal(b)
	a.Events.Put(ctx, personal(ctx, lessonsKey), string(raw))
}

// fingerprint names a lesson by what it would keep, so the same one
// proposed again, even worded with other capitals or punctuation, is
// known.
func fingerprint(kind, key string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(key) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		default:
			space = true
		}
	}
	return kind + ":" + b.String()
}

func lessonID(person, print string) string {
	h := sha256.Sum256([]byte(people.Norm(person) + "\x00" + print))
	return hex.EncodeToString(h[:6])
}

// lessonDecided says whether the person already decided this lesson.
func (a *App) lessonDecided(ctx context.Context, print string) bool {
	for _, s := range a.lessonBook(ctx).Seen {
		if s == print {
			return true
		}
	}
	return false
}

// rejectLesson remembers a lesson the person turned down elsewhere (a
// suggestion declined in the inbox), so it is not proposed here either.
func (a *App) rejectLesson(ctx context.Context, print string) {
	lessonsMu.Lock()
	defer lessonsMu.Unlock()
	b := a.lessonBook(ctx)
	b.Seen = append(b.Seen, print)
	a.saveLessonBook(ctx, b)
}

// noteLessonEvidence keeps the explorations a learned preference came
// from, to show with its lesson.
func (a *App) noteLessonEvidence(ctx context.Context, print string, links []lessonLink) {
	if len(links) == 0 {
		return
	}
	lessonsMu.Lock()
	defer lessonsMu.Unlock()
	b := a.lessonBook(ctx)
	if b.Evidence == nil {
		b.Evidence = map[string][]lessonLink{}
	}
	b.Evidence[print] = links
	if len(b.Evidence) > 200 {
		for k := range b.Evidence {
			delete(b.Evidence, k)
			if len(b.Evidence) <= 200 {
				break
			}
		}
	}
	a.saveLessonBook(ctx, b)
}

// factOwner is how memory stores the person's own facts.
func factOwner(person string) string {
	if person == people.OwnerID {
		return ""
	}
	return person
}

// proposedLessons are the person's lessons waiting for them, newest first.
func (a *App) proposedLessons(ctx context.Context) []lesson {
	me := people.From(ctx)
	book := a.lessonBook(ctx)
	seen := map[string]bool{}
	for _, s := range book.Seen {
		seen[s] = true
	}
	var out []lesson
	add := func(l lesson) {
		if seen[l.Print] {
			return
		}
		seen[l.Print] = true
		l.Person, l.State, l.ID = me, lessonProposed, lessonID(me, l.Print)
		if l.Evidence == nil {
			l.Evidence = []lessonLink{}
		}
		out = append(out, l)
	}
	for _, l := range a.memoryLessons(ctx, book) {
		add(l)
	}
	for _, l := range a.explorationLessons(ctx) {
		add(l)
	}
	if me == people.OwnerID {
		// Suggestions are the owner's: they come from the owner's mail.
		for _, s := range a.suggestions(ctx) {
			add(lesson{Kind: lessonRoutine, From: "suggestion", Title: s.Title, Detail: s.Why, Change: s.Request, Ref: s.ID,
				Evidence: []lessonLink{{Kind: "suggestion", To: "/inbox"}}, Created: s.Made, Print: fingerprint(lessonRoutine, s.Request)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// memoryLessons are the person's unconfirmed facts: preferences learned
// from their own words, and notes the agent took during a task.
func (a *App) memoryLessons(ctx context.Context, book lessonBook) []lesson {
	if a.Memory == nil {
		return nil
	}
	who := factOwner(people.From(ctx))
	facts, _ := a.Memory.List()
	var out []lesson
	for _, f := range facts {
		if f.Person != who {
			continue
		}
		switch {
		case f.Trust == memory.Learned:
			p := fingerprint(lessonPreference, f.Text)
			links := book.Evidence[p]
			out = append(out, lesson{Kind: lessonPreference, From: "learned", Title: f.Text, Change: f.Text, Ref: f.ID,
				Detail: strings.TrimPrefix(f.Source, "aprendido: "), Evidence: append([]lessonLink{{Kind: "memory", To: "/memory"}}, links...),
				Created: f.Created, Print: p})
		case f.Trust == memory.Low:
			// Only notes taken during a task; facts imported from another
			// assistant are not something Pimpo noticed.
			src, id, ok := strings.Cut(f.Source, ":")
			if !ok || (src != "exploration" && src != "chat") {
				continue
			}
			out = append(out, lesson{Kind: lessonFact, From: "note", Title: f.Text, Change: f.Text, Ref: f.ID, Detail: f.Topic,
				Evidence: []lessonLink{{Kind: "exploration", To: "/explorations/" + id}, {Kind: "memory", To: "/memory"}},
				Created:  f.Created, Print: fingerprint(lessonFact, f.Text)})
		}
	}
	return out
}

// explorationLessons are tasks the person asked at least twice that
// worked and are not a routine yet, and repairs of their routines ready
// to keep.
func (a *App) explorationLessons(ctx context.Context) []lesson {
	ready, err := a.myExplorations(ctx, store.ExplorationReady)
	if err != nil {
		return nil
	}
	compiled := map[string]bool{}
	if done, err := a.myExplorations(ctx, store.ExplorationDone); err == nil {
		for _, e := range done {
			if e.Routine != "" {
				compiled[fingerprint(lessonRoutine, e.Request)] = true
			}
		}
	}
	routines := map[string]store.Routine{}
	if list, err := a.myRoutines(ctx); err == nil {
		for _, r := range list {
			routines[r.ID] = r
		}
	}
	var out []lesson
	groups := map[string][]store.Exploration{}
	var order []string
	for _, e := range ready {
		if e.Routine != "" {
			r, ok := routines[e.Routine]
			if !ok {
				continue
			}
			detail := ""
			if runs, err := a.Store.Runs(ctx, r.ID, 1); err == nil && len(runs) > 0 && runs[0].Outcome == store.RunFailed {
				detail = clip(runs[0].Error, 300)
			}
			out = append(out, lesson{Kind: lessonFix, From: "repair", Title: r.Body.Name, Detail: detail, Change: clip(e.Summary, 600),
				Ref: e.ID, Routine: r.ID, Created: e.UpdatedAt, Print: "fix:" + e.ID,
				Evidence: []lessonLink{{Kind: "routine", To: "/routines/" + r.ID}, {Kind: "exploration", To: "/explorations/" + e.ID}}})
			continue
		}
		p := fingerprint(lessonRoutine, e.Request)
		if compiled[p] || e.Trace == nil {
			continue
		}
		if _, ok := groups[p]; !ok {
			order = append(order, p)
		}
		groups[p] = append(groups[p], e)
	}
	for _, p := range order {
		g := groups[p]
		if len(g) < 2 {
			continue
		}
		sort.SliceStable(g, func(i, j int) bool { return g[i].UpdatedAt.After(g[j].UpdatedAt) })
		var links []lessonLink
		for i, e := range g {
			if i == 5 {
				break
			}
			links = append(links, lessonLink{Kind: "exploration", To: "/explorations/" + e.ID})
		}
		out = append(out, lesson{Kind: lessonRoutine, From: "repeated", Title: clip(g[0].Request, 120), Detail: clip(g[0].Summary, 300),
			Change: g[0].Request, Ref: g[0].ID, Created: g[0].UpdatedAt, Evidence: links, Print: p})
	}
	return out
}

func (a *App) findLesson(ctx context.Context, id string) (lesson, bool) {
	for _, l := range a.proposedLessons(ctx) {
		if l.ID == id {
			return l, true
		}
	}
	return lesson{}, false
}

var errLessonGone = errors.New("that lesson is no longer waiting")

// decideLesson accepts, edits or rejects one of the person's lessons.
// text is the edited change, for edit.
func (a *App) decideLesson(ctx context.Context, id, action, text string) (lesson, error) {
	l, ok := a.findLesson(ctx, id)
	if !ok {
		return lesson{}, server.StatusError{Status: 404, Msg: errLessonGone.Error()}
	}
	text = strings.TrimSpace(text)
	state := map[string]string{"accept": lessonAccepted, "edit": lessonEdited, "reject": lessonRejected}[action]
	switch {
	case state == "":
		return lesson{}, server.StatusError{Status: 404, Msg: "unknown action"}
	case state == lessonEdited && text == "":
		return lesson{}, server.StatusError{Status: 400, Msg: "write the lesson as you want it kept"}
	case state == lessonEdited && strings.EqualFold(text, strings.TrimSpace(l.Change)):
		state = lessonAccepted
	}
	result, err := a.applyLesson(ctx, l, state, text)
	if err != nil {
		return lesson{}, err
	}
	now := time.Now()
	l.State, l.Result, l.Decided = state, result, &now
	if state == lessonEdited {
		l.Edited = text
	}
	lessonsMu.Lock()
	b := a.lessonBook(ctx)
	b.Seen = append(b.Seen, l.Print)
	if state == lessonEdited && l.Kind != lessonFix {
		// The edited version is the person's; a close copy of the
		// original is not proposed again.
		b.Seen = append(b.Seen, fingerprint(l.Kind, text))
	}
	delete(b.Evidence, l.Print)
	b.Done = append([]lesson{l}, b.Done...)
	a.saveLessonBook(ctx, b)
	lessonsMu.Unlock()
	a.Events.Append(ctx, "lesson."+state, actor(ctx), map[string]string{"id": l.ID, "kind": l.Kind, "from": l.From, "ref": l.Ref, "result": result})
	return l, nil
}

// applyLesson does what the decision means, through the usual paths. It
// returns what came of it: a fact, a routine or an exploration.
func (a *App) applyLesson(ctx context.Context, l lesson, state, text string) (string, error) {
	switch l.Kind {
	case lessonPreference, lessonFact:
		return a.applyFactLesson(ctx, l, state, text)
	case lessonFix:
		switch state {
		case lessonAccepted:
			r, err := a.Explore.Approve(context.WithoutCancel(ctx), l.Ref, actor(ctx))
			if err != nil {
				return "", server.StatusError{Status: 422, Msg: err.Error()}
			}
			return r.ID, nil
		case lessonEdited:
			// Try the repair again with the person's words on what to fix.
			eid, err := a.Explore.Repair(context.WithoutCancel(ctx), l.Routine, clip(text, 600), actor(ctx))
			if err != nil {
				return "", server.StatusError{Status: 400, Msg: err.Error()}
			}
			a.Explore.Discard(ctx, l.Ref, actor(ctx))
			return eid, nil
		default:
			return "", a.Explore.Discard(ctx, l.Ref, actor(ctx))
		}
	}
	// A routine: a suggestion or a task asked again and again.
	if l.From == "suggestion" {
		switch state {
		case lessonAccepted:
			eid, err := a.acceptSuggestion(ctx, l.Ref)
			if err != nil {
				return "", server.StatusError{Status: 404, Msg: err.Error()}
			}
			return eid, nil
		case lessonEdited:
			a.dropSuggestion(ctx, l.Ref)
			return a.startLesson(ctx, text)
		default:
			return "", a.dismissSuggestion(ctx, l.Ref)
		}
	}
	switch state {
	case lessonAccepted:
		r, err := a.Explore.Approve(context.WithoutCancel(ctx), l.Ref, actor(ctx))
		if err != nil {
			return "", server.StatusError{Status: 422, Msg: err.Error()}
		}
		return r.ID, nil
	case lessonEdited:
		// A different request is a new task: it is done once while the
		// person watches, and becomes a routine only when they approve.
		return a.startLesson(ctx, text)
	}
	return "", nil
}

func (a *App) startLesson(ctx context.Context, request string) (string, error) {
	eid, err := a.Explore.Start(context.WithoutCancel(ctx), clip(request, 2000), actor(ctx))
	if err != nil {
		return "", server.StatusError{Status: 400, Msg: err.Error()}
	}
	return eid, nil
}

// applyFactLesson confirms, rewrites or removes an unconfirmed fact, as
// the Memory page does.
func (a *App) applyFactLesson(ctx context.Context, l lesson, state, text string) (string, error) {
	if a.Memory == nil {
		return "", server.StatusError{Status: 503, Msg: "memory is not available"}
	}
	me := people.From(ctx)
	f, ok := a.Memory.Get(l.Ref)
	if !ok || f.Person != factOwner(me) {
		return "", server.StatusError{Status: 404, Msg: errLessonGone.Error()}
	}
	switch state {
	case lessonAccepted:
		if err := a.Memory.Confirm(f.ID); err != nil {
			return "", err
		}
		a.Events.Append(ctx, "memory.changed", actor(ctx), map[string]string{"confirmed": f.ID})
		return f.ID, nil
	case lessonEdited:
		nf, err := a.Memory.AddFor(clip(text, 300), f.Topic, me, memory.High, me)
		if err != nil {
			return "", server.StatusError{Status: 400, Msg: err.Error()}
		}
		a.Memory.Remove(f.ID)
		a.Events.Append(ctx, "memory.changed", actor(ctx), map[string]string{"added": nf.ID, "removed": f.ID})
		return nf.ID, nil
	}
	if err := a.Memory.Remove(f.ID); err != nil {
		return "", err
	}
	a.Events.Append(ctx, "memory.changed", actor(ctx), map[string]string{"removed": f.ID})
	return "", nil
}

func (a *App) lessonRoutes() {
	a.Server.Handle("GET /api/lessons", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		proposed := a.proposedLessons(ctx)
		if proposed == nil {
			proposed = []lesson{}
		}
		done := a.lessonBook(ctx).Done
		if done == nil {
			done = []lesson{}
		}
		server.WriteJSON(w, 200, map[string]any{"proposed": proposed, "decided": done})
	})
	a.Server.Handle("POST /api/lessons/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if r.ContentLength > 0 {
			if err := server.Decode(r, &req); err != nil {
				server.WriteError(w, err)
				return
			}
		}
		l, err := a.decideLesson(r.Context(), r.PathValue("id"), r.PathValue("action"), req.Text)
		if err != nil {
			server.WriteError(w, err)
			return
		}
		server.WriteJSON(w, 200, l)
	})
}

// The weekly digest: a short notice, only when something waits, with a
// link to the list. It carries no buttons and no numbers to answer: a
// lesson is decided on the Lessons page, where the person sees it whole.

func (a *App) lessonDigestLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.lessonDigests(ctx, time.Now())
		}
	}
}

// lessonDigests sends each person due for one their digest, and returns
// how many were sent.
func (a *App) lessonDigests(ctx context.Context, now time.Time) int {
	s := a.Settings(ctx)
	if s.LessonDigestOff {
		return 0
	}
	if zone, err := time.LoadLocation(s.Zone); err == nil {
		now = now.In(zone)
	}
	if now.Hour() < lessonDigestHour {
		return 0
	}
	list, err := a.People.List(ctx)
	if err != nil {
		return 0
	}
	sent := 0
	for _, p := range list {
		if p.Role == people.Guest {
			continue
		}
		pctx := people.With(ctx, p.ID)
		last, _ := a.Events.Get(pctx, personal(pctx, lessonDigestKey))
		if t, err := time.Parse(time.RFC3339, last); err == nil && now.Sub(t) < 7*24*time.Hour-time.Hour {
			continue
		}
		n := len(a.proposedLessons(pctx))
		if n == 0 {
			continue
		}
		a.Events.Put(pctx, personal(pctx, lessonDigestKey), now.UTC().Format(time.RFC3339))
		text := i18n.T(pctx, "msg.lessons.digest", "count", n)
		if link := a.appLink("/lessons"); link != "" {
			text += "\n" + link
		}
		a.Channel.Notify(pctx, explore.Notice{Text: text, To: factOwner(p.ID), Kind: "task"})
		sent++
	}
	return sent
}

// appLink is a page of the app at an address a phone can open: the
// Tailscale one, else the home network's; "" when there is neither.
func (a *App) appLink(path string) string {
	if a.Remote != nil {
		if st := a.Remote.Status(); st.State == "running" && st.URL != "" {
			return strings.TrimRight(st.URL, "/") + path
		}
	}
	if lan := a.lanURL(); lan != "" {
		return strings.TrimRight(lan, "/") + path
	}
	return ""
}

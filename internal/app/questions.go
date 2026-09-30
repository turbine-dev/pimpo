package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// A routine may ask the person it works for something and act on the
// answer: "did you work out today?". ask.owner sends the question with
// its options as buttons (numbered on channels without buttons, and in
// the inbox); the answer runs the same routine again with event.answer.

type question struct {
	ID       string    `json:"id"`
	Routine  string    `json:"routine,omitempty"`
	Person   string    `json:"person,omitempty"`
	Key      string    `json:"key"`
	Question string    `json:"question"`
	Options  []string  `json:"options"`
	Asked    time.Time `json:"asked"`
	Expires  time.Time `json:"expires"`
}

const questionsKey = "questions"

func (a *App) questions(ctx context.Context) []question {
	raw, _ := a.Events.Get(ctx, questionsKey)
	var qs []question
	json.Unmarshal([]byte(raw), &qs)
	now := time.Now()
	live := qs[:0]
	for _, q := range qs {
		if q.Expires.After(now) {
			live = append(live, q)
		}
	}
	return live
}

func (a *App) saveQuestions(ctx context.Context, qs []question) {
	if qs == nil {
		qs = []question{}
	}
	b, _ := json.Marshal(qs)
	a.Events.Put(ctx, questionsKey, string(b))
}

type askCap struct{ a *App }

func (askCap) Capabilities() []string { return []string{"ask.owner"} }

func (c askCap) Call(ctx context.Context, _, _ string, args any) (any, error) {
	var in struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
		Key      string   `json:"key"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	in.Question = strings.TrimSpace(in.Question)
	var opts []string
	for _, o := range in.Options {
		if o = strings.TrimSpace(o); o != "" {
			if r := []rune(o); len(r) > 40 {
				o = string(r[:40])
			}
			opts = append(opts, o)
		}
	}
	if in.Question == "" || len(opts) < 2 || len(opts) > 6 {
		return nil, errors.New("ask.owner needs a question and 2 to 6 options")
	}
	routine := ""
	if src := host.SourceOf(ctx); strings.HasPrefix(src, "routine:") {
		routine, _, _ = strings.Cut(strings.TrimPrefix(src, "routine:"), "#")
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = in.Question
	}
	b := make([]byte, 6)
	rand.Read(b)
	now := time.Now()
	q := question{ID: hex.EncodeToString(b), Routine: routine, Person: people.Norm(people.From(ctx)), Key: key, Question: in.Question, Options: opts, Asked: now, Expires: now.Add(24 * time.Hour)}
	qs := c.a.questions(ctx)
	kept := qs[:0]
	for _, old := range qs {
		if !(old.Routine == q.Routine && old.Key == q.Key && old.Person == q.Person) {
			kept = append(kept, old)
		}
	}
	c.a.saveQuestions(ctx, append(kept, q))
	var actions []explore.Action
	for i, o := range opts {
		actions = append(actions, explore.Action{Label: o, Data: "answer:" + q.ID + "." + strconv.Itoa(i)})
	}
	c.a.Channel.Notify(ctx, explore.Notice{Text: "❓ " + q.Question, Actions: actions, To: q.Person})
	c.a.Events.Append(ctx, "question.asked", "system", map[string]any{"id": q.ID, "routine": routine, "key": key})
	return map[string]any{"asked": q.ID}, nil
}

// answer records an answer and runs the routine that asked with it.
func (a *App) answer(ctx context.Context, id string, index int) (string, error) {
	qs := a.questions(ctx)
	for i, q := range qs {
		if q.ID != id {
			continue
		}
		if people.Norm(people.From(ctx)) != people.Norm(q.Person) {
			return "", errors.New(i18n.T(ctx, "msg.ownerOnly"))
		}
		if index < 0 || index >= len(q.Options) {
			return "", fmt.Errorf("there is no option %d", index+1)
		}
		a.saveQuestions(ctx, append(qs[:i:i], qs[i+1:]...))
		choice := q.Options[index]
		a.Events.Append(ctx, "question.answered", actor(ctx), map[string]any{"id": q.ID, "routine": q.Routine, "key": q.Key, "choice": choice})
		if q.Routine != "" {
			// Like a webhook, an answer does not wake a paused routine.
			if r, err := a.Store.Routine(ctx, q.Routine); err == nil && r.State != store.RoutineActive {
				return i18n.T(ctx, "msg.answer.paused", "choice", choice, "name", r.Body.Name), nil
			}
			ev := map[string]any{"answer": map[string]any{"key": q.Key, "question": q.Question, "choice": choice, "index": index, "asked": q.Asked.Format(time.RFC3339)}}
			go a.Scheduler.RunWith(context.WithoutCancel(ctx), q.Routine, "answer", ev)
		}
		return i18n.T(ctx, "msg.answer.noted", "choice", choice), nil
	}
	return "", errors.New(i18n.T(ctx, "msg.answer.gone"))
}

// errNoMatch is a typed answer that is none of the options; its text
// shows them again.
type errNoMatch struct{ msg string }

func (e errNoMatch) Error() string { return e.msg }

// answerText answers question id with typed text, or says which options
// there are when the text is none of them.
func (a *App) answerText(ctx context.Context, id, text string) (string, error) {
	for _, q := range a.questions(ctx) {
		if q.ID != id {
			continue
		}
		if people.Norm(people.From(ctx)) != people.Norm(q.Person) {
			return "", errors.New(i18n.T(ctx, "msg.ownerOnly"))
		}
		i, ok := matchOption(text, q.Options)
		if !ok {
			return "", errNoMatch{i18n.T(ctx, "msg.answer.pick", "question", q.Question, "options", optionList(q.Options))}
		}
		return a.answer(ctx, id, i)
	}
	return "", errors.New(i18n.T(ctx, "msg.answer.gone"))
}

// bareAnswer takes a message sent without replying as the answer to the
// person's latest question, only when it is exactly one of its options,
// the question is recent and nothing was said on that channel since it
// was asked. Otherwise the message is an ordinary one.
func (a *App) bareAnswer(ctx context.Context, via, text string) (string, bool) {
	person := people.Norm(people.From(ctx))
	var latest *question
	qs := a.questions(ctx)
	for i := range qs {
		if people.Norm(qs[i].Person) == person && (latest == nil || qs[i].Asked.After(latest.Asked)) {
			latest = &qs[i]
		}
	}
	if latest == nil || time.Since(latest.Asked) > choiceLife {
		return "", false
	}
	if raw, _ := a.Events.Get(ctx, "conv."+via+"."+people.From(ctx)); raw != "" {
		_, at, _ := strings.Cut(raw, "|")
		if sec, err := strconv.ParseInt(at, 10, 64); err == nil && !time.Unix(sec, 0).Before(latest.Asked.Truncate(time.Second)) {
			return "", false
		}
	}
	want := foldAnswer(text)
	for i, o := range latest.Options {
		if foldAnswer(o) == want {
			out, err := a.answer(ctx, latest.ID, i)
			if err != nil {
				out = "⚠️ " + err.Error()
			}
			return out, true
		}
	}
	return "", false
}

// replyAnswer answers a reply to a notice whose choices are a question's
// options. handled is false when the notice is not a question.
func (a *App) replyAnswer(ctx context.Context, choices []explore.Action, text string) (reply string, handled bool) {
	id := questionOf(choices)
	if id == "" {
		return "", false
	}
	out, err := a.answerText(ctx, id, text)
	if err != nil {
		out = err.Error()
		if _, ok := err.(errNoMatch); !ok {
			out = "⚠️ " + out
		}
	}
	return out, true
}

// questionOf is the question a notice's choices answer, or "".
func questionOf(choices []explore.Action) string {
	id := ""
	for _, c := range choices {
		data, ok := strings.CutPrefix(c.Data, "answer:")
		if !ok {
			return ""
		}
		qid, _, _ := strings.Cut(data, ".")
		if id != "" && qid != id {
			return ""
		}
		id = qid
	}
	return id
}

// matchOption finds the option typed text means: its number, its label,
// or the start of exactly one label, ignoring case, accents, spaces and
// end punctuation.
func matchOption(text string, options []string) (int, bool) {
	want := foldAnswer(text)
	if want == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(want); err == nil {
		// A number is a position, unless an option reads that way.
		for i, o := range options {
			if foldAnswer(o) == want {
				return i, true
			}
		}
		return n - 1, n >= 1 && n <= len(options)
	}
	found := -1
	for i, o := range options {
		f := foldAnswer(o)
		if f == want {
			return i, true
		}
		if strings.HasPrefix(f, want) {
			if found >= 0 {
				return 0, false
			}
			found = i
		}
	}
	return found, found >= 0
}

var accents = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// foldAnswer is text as answers compare: lowercase, without accents,
// single spaces, and no punctuation at the ends.
func foldAnswer(s string) string {
	out, _, err := transform.String(accents, s)
	if err != nil {
		out = s
	}
	out = strings.Join(strings.Fields(strings.ToLower(out)), " ")
	return strings.TrimFunc(out, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSpace(r) })
}

// optionList is how the options read in a message: "1 = Yes · 2 = No".
func optionList(options []string) string {
	var parts []string
	for i, o := range options {
		parts = append(parts, fmt.Sprintf("%d = %s", i+1, o))
	}
	return strings.Join(parts, " · ")
}

func parseAnswer(data string) (string, int, error) {
	id, n, ok := strings.Cut(data, ".")
	i, err := strconv.Atoi(n)
	if !ok || err != nil {
		return "", 0, errors.New("unreadable answer")
	}
	return id, i, nil
}

func (a *App) questionRoutes() {
	a.Server.Handle("GET /api/questions", func(w http.ResponseWriter, r *http.Request) {
		me := people.Norm(people.From(r.Context()))
		out := []question{}
		for _, q := range a.questions(r.Context()) {
			if people.Norm(q.Person) == me {
				out = append(out, q)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Asked.After(out[j].Asked) })
		server.WriteJSON(w, 200, out)
	})
	a.Server.Handle("POST /api/questions/{id}/answer", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Index int `json:"index"`
			// Text is a typed answer, checked against the options.
			Text *string `json:"text"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		var text string
		var err error
		if req.Text != nil {
			text, err = a.answerText(r.Context(), r.PathValue("id"), *req.Text)
		} else {
			text, err = a.answer(r.Context(), r.PathValue("id"), req.Index)
		}
		var miss errNoMatch
		if errors.As(err, &miss) {
			server.WriteError(w, server.StatusError{Status: 422, Msg: miss.msg})
			return
		}
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, map[string]string{"text": text})
	})
}

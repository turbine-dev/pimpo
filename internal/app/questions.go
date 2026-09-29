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
		if people.Norm(people.From(ctx)) != q.Person && people.From(ctx) != people.OwnerID {
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
			if q.Person == me || people.From(r.Context()) == people.OwnerID {
				out = append(out, q)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Asked.After(out[j].Asked) })
		server.WriteJSON(w, 200, out)
	})
	a.Server.Handle("POST /api/questions/{id}/answer", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Index int `json:"index"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		text, err := a.answer(r.Context(), r.PathValue("id"), req.Index)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, map[string]string{"text": text})
	})
}

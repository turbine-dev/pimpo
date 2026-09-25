package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/zodim/internal/capability"
	"github.com/denerFernandes/zodim/internal/explore"
	"github.com/denerFernandes/zodim/internal/host"
	"github.com/denerFernandes/zodim/internal/people"
	"github.com/denerFernandes/zodim/internal/policy"
	"github.com/denerFernandes/zodim/internal/server"
	"github.com/denerFernandes/zodim/internal/store"
)

// The chat in the app: each message is an exploration that sees the
// conversation so far. Changes are rehearsed and listed under the answer;
// "Confirm and do it" performs exactly those, once, each still under the
// rules and approvals.

func (a *App) chatRoutes() {
	s := a.Server
	s.Handle("GET /api/chats", a.listChats)
	s.Handle("POST /api/chats", a.newChat)
	s.Handle("GET /api/chats/{id}", a.getChat)
	s.Handle("DELETE /api/chats/{id}", a.deleteChat)
	s.Handle("POST /api/chats/{id}/messages", a.chatMessage)
	s.Handle("POST /api/chats/{id}/turns/{exp}/do", a.chatDo)
}

type chatAction struct {
	Capability string `json:"capability"`
	Text       string `json:"text"`
	Risk       string `json:"risk"`
	Args       any    `json:"args"`
}

type chatResult struct {
	Capability string `json:"capability"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

type chatDone struct {
	State   string       `json:"state"` // running, done or failed
	At      time.Time    `json:"at"`
	Results []chatResult `json:"results"`
}

type chatTurn struct {
	ID        string       `json:"id"`
	Request   string       `json:"request"`
	State     string       `json:"state"`
	Summary   string       `json:"summary,omitempty"`
	Error     string       `json:"error,omitempty"`
	CostUSD   float64      `json:"cost_usd"`
	Routine   string       `json:"routine,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	Actions   []chatAction `json:"actions"`
	Done      *chatDone    `json:"done,omitempty"`
}

func chatID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// rehearsed lists the changes an exploration only simulated.
func rehearsed(e store.Exploration) []chatAction {
	out := []chatAction{}
	if e.Trace == nil {
		return out
	}
	for _, c := range e.Trace.Calls {
		spec, ok := capability.Catalog[c.Capability]
		if !ok || spec.Risk < capability.Reversible {
			continue
		}
		var args any
		json.Unmarshal(c.Args, &args)
		text := actionPhrase(policy.Action{Capability: c.Capability, Args: args, Risk: spec.Risk, Source: "chat"})
		if r := []rune(text); len(r) > 0 {
			text = strings.ToUpper(string(r[0])) + string(r[1:])
		}
		out = append(out, chatAction{Capability: c.Capability, Text: text, Risk: spec.Risk.String(), Args: args})
	}
	return out
}

func doneKey(exp string) string { return "chat.done." + exp }

func (a *App) turn(ctx context.Context, id string) (chatTurn, error) {
	e, err := a.Store.Exploration(ctx, id)
	if err != nil {
		return chatTurn{}, err
	}
	t := chatTurn{ID: e.ID, Request: e.Request, State: e.State, Summary: e.Summary, Error: e.Error, CostUSD: e.CostUSD, Routine: e.Routine, CreatedAt: e.CreatedAt, Actions: rehearsed(e)}
	if raw, _ := a.Events.Get(ctx, doneKey(id)); raw != "" {
		var d chatDone
		if json.Unmarshal([]byte(raw), &d) == nil {
			t.Done = &d
		}
	}
	return t, nil
}

// ownChat loads a chat the caller may use.
func (a *App) ownChat(w http.ResponseWriter, r *http.Request) (store.Chat, []string, bool) {
	c, ids, err := a.Store.Chat(r.Context(), r.PathValue("id"))
	if err != nil || c.Person != chatPerson(people.From(r.Context())) {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such conversation"})
		return c, nil, false
	}
	return c, ids, true
}

// chatPerson stores the owner's chats with an empty person.
func chatPerson(person string) string {
	if people.Norm(person) == people.OwnerID {
		return ""
	}
	return person
}

func (a *App) listChats(w http.ResponseWriter, r *http.Request) {
	list, err := a.Store.Chats(r.Context(), chatPerson(people.From(r.Context())))
	if err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, list)
}

func (a *App) getChat(w http.ResponseWriter, r *http.Request) {
	c, ids, ok := a.ownChat(w, r)
	if !ok {
		return
	}
	turns := []chatTurn{}
	for _, id := range ids {
		if t, err := a.turn(r.Context(), id); err == nil {
			turns = append(turns, t)
		}
	}
	server.WriteJSON(w, 200, map[string]any{"chat": c, "turns": turns})
}

func messageText(r *http.Request) (string, error) {
	var req struct {
		Text string `json:"text"`
	}
	if err := server.Decode(r, &req); err != nil {
		return "", err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return "", server.StatusError{Status: 400, Msg: "write something first"}
	}
	if len(text) > 8000 {
		return "", server.StatusError{Status: 400, Msg: "that message is too long"}
	}
	return text, nil
}

func (a *App) newChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	text, err := messageText(r)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	title := text
	if runes := []rune(title); len(runes) > 60 {
		title = string(runes[:59]) + "…"
	}
	c := store.Chat{ID: chatID(), Title: title, Person: chatPerson(people.From(ctx))}
	if err := a.Store.CreateChat(ctx, c); err != nil {
		server.WriteError(w, err)
		return
	}
	exp, err := a.Explore.StartWith(context.WithoutCancel(ctx), text, actor(ctx), explore.Options{Quiet: true})
	if err != nil {
		a.Store.DeleteChat(ctx, c.ID)
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	a.Store.AddTurn(ctx, c.ID, exp)
	server.WriteJSON(w, 201, map[string]string{"chat": c.ID, "turn": exp})
}

// history is the conversation so far, for the agent: the last turns,
// each shortened.
func (a *App) history(ctx context.Context, ids []string) string {
	if len(ids) > 8 {
		ids = ids[len(ids)-8:]
	}
	short := func(s string) string {
		if r := []rune(s); len(r) > 800 {
			return string(r[:799]) + "…"
		}
		return s
	}
	var b strings.Builder
	for _, id := range ids {
		e, err := a.Store.Exploration(ctx, id)
		if err != nil {
			continue
		}
		answer := e.Summary
		if answer == "" {
			answer = "(no answer: " + e.Error + ")"
		}
		b.WriteString("Owner: " + short(e.Request) + "\nZodim: " + short(answer) + "\n")
	}
	return strings.TrimSpace(b.String())
}

func (a *App) chatMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, ids, ok := a.ownChat(w, r)
	if !ok {
		return
	}
	text, err := messageText(r)
	if err != nil {
		server.WriteError(w, err)
		return
	}
	for _, id := range ids {
		if e, err := a.Store.Exploration(ctx, id); err == nil && e.State == store.ExplorationRunning {
			server.WriteError(w, server.StatusError{Status: 409, Msg: "wait for the answer to the last message"})
			return
		}
	}
	exp, err := a.Explore.StartWith(context.WithoutCancel(ctx), text, actor(ctx), explore.Options{Context: a.history(ctx, ids), Quiet: true})
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	a.Store.AddTurn(ctx, c.ID, exp)
	server.WriteJSON(w, 201, map[string]string{"chat": c.ID, "turn": exp})
}

var chatDoMu sync.Mutex

// chatDo performs the rehearsed changes of one answer, once.
func (a *App) chatDo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ids, ok := a.ownChat(w, r)
	if !ok {
		return
	}
	exp := r.PathValue("exp")
	found := false
	for _, id := range ids {
		found = found || id == exp
	}
	if !found {
		server.WriteError(w, server.StatusError{Status: 404, Msg: "no such answer in this conversation"})
		return
	}
	e, err := a.Store.Exploration(ctx, exp)
	if err != nil || e.State != store.ExplorationReady {
		server.WriteError(w, server.StatusError{Status: 409, Msg: "this answer has nothing to do yet"})
		return
	}
	actions := rehearsed(e)
	if len(actions) == 0 {
		server.WriteError(w, server.StatusError{Status: 409, Msg: "this answer has nothing to do"})
		return
	}
	chatDoMu.Lock()
	if raw, _ := a.Events.Get(ctx, doneKey(exp)); raw != "" {
		chatDoMu.Unlock()
		server.WriteError(w, server.StatusError{Status: 409, Msg: "this was already done"})
		return
	}
	d := chatDone{State: "running", At: time.Now(), Results: []chatResult{}}
	b, _ := json.Marshal(d)
	a.Events.Put(ctx, doneKey(exp), string(b))
	chatDoMu.Unlock()

	who := actor(ctx)
	a.Events.Append(ctx, "chat.confirmed", who, map[string]any{"exploration": exp, "actions": len(actions)})
	go a.perform(people.With(context.WithoutCancel(ctx), people.From(ctx)), e, actions)
	t, _ := a.turn(ctx, exp)
	server.WriteJSON(w, 202, t)
}

// perform runs the actions for real; approvals may make it wait for the
// owner, so it runs in the background.
func (a *App) perform(ctx context.Context, e store.Exploration, actions []chatAction) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	h := &host.Host{Env: a.Explore.Env, Source: "chat:" + e.ID, Person: e.Person}
	d := chatDone{State: "done", At: time.Now(), Results: []chatResult{}}
	for _, act := range actions {
		_, err := h.Call(ctx, act.Capability, "", act.Args)
		res := chatResult{Capability: act.Capability, OK: err == nil}
		if err != nil {
			res.Error = err.Error()
			d.State = "failed"
		}
		d.Results = append(d.Results, res)
		if errors.Is(err, host.ErrBlocked) {
			break
		}
	}
	b, _ := json.Marshal(d)
	a.Events.Put(ctx, doneKey(e.ID), string(b))
}

func (a *App) deleteChat(w http.ResponseWriter, r *http.Request) {
	c, _, ok := a.ownChat(w, r)
	if !ok {
		return
	}
	if err := a.Store.DeleteChat(r.Context(), c.ID); err != nil {
		server.WriteError(w, err)
		return
	}
	server.WriteJSON(w, 200, map[string]string{"deleted": c.ID})
}

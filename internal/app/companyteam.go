package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Members work together through tasks and questions. A task carries its
// objective, how to know it is done and a dossier of where it came from;
// a question stops a piece of work until it is answered, by a person or
// by the agent above, and the work then goes on from where it was.

// driftThreshold is how sure the judge must be that a task still serves
// its root for it to start without asking.
const driftThreshold = 0.5

func init() {
	for _, s := range []capability.Spec{
		{Name: "company.assign", Risk: capability.Notify, Signature: "company.assign({assignee, title, objective, acceptance, constraints, out_of_scope, due, priority, links})",
			Returns: "{task: id}; hands a task to a member below you (or in your department, when the company allows it); objective and acceptance (how to know it is done) are required; links are [{kind, ref, title}] to where it comes from",
			Schema:  `{"type":"object","properties":{"assignee":{"type":"string"},"title":{"type":"string"},"objective":{"type":"string"},"acceptance":{"type":"string"},"constraints":{"type":"string"},"out_of_scope":{"type":"string"},"due":{"type":"string"},"priority":{"type":"integer"},"links":{"type":"array"}},"required":["assignee","title","objective","acceptance"]}`},
		{Name: "company.report", Risk: capability.Notify, Signature: "company.report({task, status, summary, links})",
			Returns: "{ok}; tells whoever gave you a task that it is done or blocked (status done or blocked), with what you did",
			Schema:  `{"type":"object","properties":{"task":{"type":"string"},"status":{"type":"string","enum":["done","blocked"]},"summary":{"type":"string"},"links":{"type":"array"}},"required":["task","status","summary"]}`},
		{Name: "company.ask", Risk: capability.Notify, Signature: "company.ask({question, options, recommendation, context, to, kind, amount_usd, public})",
			Returns: "{asked: id}; stops this work until the answer comes: to your boss to decide (to: boss, the default), or to whoever gave you the task to clarify it (to: requester). End your turn right after, saying where you are; you go on from there with the answer",
			Schema:  `{"type":"object","properties":{"question":{"type":"string"},"options":{"type":"array","items":{"type":"string"}},"recommendation":{"type":"string"},"context":{"type":"string"},"to":{"type":"string","enum":["boss","requester"]},"kind":{"type":"string"},"amount_usd":{"type":"number"},"public":{"type":"boolean"}},"required":["question"]}`},
		{Name: "company.answer", Risk: capability.Notify, Signature: "company.answer({question, choice, reason})",
			Returns: "{ok}; answers a question one of your reports asked you; the choice is one of its options",
			Schema:  `{"type":"object","properties":{"question":{"type":"string"},"choice":{"type":"string"},"reason":{"type":"string"}},"required":["question","choice"]}`},
	} {
		capability.Register(s)
	}
}

func newTeamID(prefix string) string {
	b := make([]byte, 6)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

type teamCap struct{ a *App }

func (teamCap) Capabilities() []string {
	return []string{"company.assign", "company.report", "company.ask", "company.answer"}
}

// caller is the company, member and piece of work a call comes from.
func (a *App) caller(ctx context.Context) (company.Org, string, company.Work, error) {
	member := host.MemberOf(ctx)
	if member == "" {
		return company.Org{}, "", company.Work{}, errors.New("only a company member can do this")
	}
	co, id, _ := strings.Cut(member, "/")
	o, err := a.Companies.Org(ctx, co)
	if err != nil {
		return company.Org{}, "", company.Work{}, err
	}
	exp, _ := strings.CutPrefix(host.SourceOf(ctx), "exploration:")
	w, _ := a.Companies.WorkFor(ctx, exp)
	return o, id, w, nil
}

func (c teamCap) Call(ctx context.Context, name, _ string, args any) (any, error) {
	o, me, work, err := c.a.caller(ctx)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(args)
	switch name {
	case "company.assign":
		var in struct {
			Assignee, Title, Objective, Acceptance, Constraints, OutOfScope, Due string
			Priority                                                             int
			Links                                                                []company.Link
		}
		json.Unmarshal(b, &in)
		var raw map[string]any
		json.Unmarshal(b, &raw)
		in.OutOfScope, _ = raw["out_of_scope"].(string)
		t := company.Task{Assignee: in.Assignee, Title: in.Title, Objective: in.Objective, Acceptance: in.Acceptance, Constraints: in.Constraints,
			OutOfScope: in.OutOfScope, Due: in.Due, Priority: in.Priority, Dossier: in.Links}
		t, err := c.a.assign(ctx, o, me, work, t)
		if err != nil {
			return nil, err
		}
		return map[string]string{"task": t.ID}, nil
	case "company.report":
		var in struct {
			Task, Status, Summary string
			Links                 []company.Link
		}
		json.Unmarshal(b, &in)
		return map[string]bool{"ok": true}, c.a.report(ctx, o, me, in.Task, in.Status, in.Summary, in.Links)
	case "company.ask":
		var in struct {
			Question, Recommendation, Context, To, Kind string
			Options                                     []string
			Public                                      bool
		}
		json.Unmarshal(b, &in)
		var raw map[string]any
		json.Unmarshal(b, &raw)
		amount, _ := raw["amount_usd"].(float64)
		q, err := c.a.ask(ctx, o, me, work, in.Question, in.Options, in.Recommendation, in.Context, in.To, company.Matter{Kind: in.Kind, AmountUSD: amount, Public: in.Public, Text: in.Question})
		if err != nil {
			return nil, err
		}
		return map[string]string{"asked": q.ID, "note": "Stop here: end your turn now, saying where you are. You go on from there once it is answered."}, nil
	case "company.answer":
		var in struct{ Question, Choice, Reason string }
		json.Unmarshal(b, &in)
		q, err := c.a.Companies.Question(ctx, in.Question)
		if err == nil && q.Company == o.ID && q.To != me && slices.Contains(q.Consulted, me) {
			// A boss on the way up gives an opinion; it decides nothing.
			_, err = c.a.Companies.AddOpinion(ctx, q.ID, company.Opinion{Member: me, Choice: clip(in.Choice, 200), Reason: clip(in.Reason, 1000)})
			return map[string]bool{"ok": err == nil}, err
		}
		if err != nil || q.Company != o.ID || q.To != me {
			return nil, errors.New("there is no such question for you")
		}
		_, err = c.a.answerQuestion(ctx, q, in.Choice, in.Reason, "member:"+o.ID+"/"+me)
		return map[string]bool{"ok": err == nil}, err
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

// assign hands a task from a member (or the CEO, from the screen) to
// another and gives the assignee the work.
func (a *App) assign(ctx context.Context, o company.Org, from string, work company.Work, t company.Task) (company.Task, error) {
	if err := o.CanAssign(from, t.Assignee); err != nil {
		return company.Task{}, err
	}
	if err := t.CheckHandoff(); err != nil {
		return company.Task{}, err
	}
	now := time.Now().UTC()
	t.ID, t.Company, t.Requester, t.State, t.Created, t.Updated = newTeamID("t_"), o.ID, from, company.TaskTodo, now, now
	t.Root, t.Depth = t.ID, 1
	var chain []company.Task
	if work.Task != "" {
		parent, err := a.Companies.Task(ctx, work.Task)
		if err == nil {
			t.Parent, t.Root, t.Depth, t.LowTrust = parent.ID, parent.Root, parent.Depth+1, parent.LowTrust
			t.Dossier = append(append([]company.Link{}, parent.Dossier...), append(t.Dossier, company.Link{Kind: "task", Ref: parent.ID, Title: parent.Title})...)
			chain = append([]company.Task{parent}, a.Companies.TaskChain(ctx, parent)...)
		}
	}
	if work.ID != "" {
		t.Dossier = append(t.Dossier, company.Link{Kind: "work", Ref: work.ID})
		t.LowTrust = t.LowTrust || len(work.Data) > 0
	}
	if len(t.Dossier) > 30 {
		t.Dossier = t.Dossier[len(t.Dossier)-30:]
	}
	open, _ := a.Companies.Tasks(ctx, o.ID)
	if err := company.Guard(t, chain, open); err != nil {
		return company.Task{}, err
	}
	if t.Parent != "" {
		root, _ := a.Companies.Task(ctx, t.Root)
		ans, err := a.judge(ctx, "Does this task serve the objective of the root task?", map[string]string{"root_objective": root.Objective, "task": t.Title + ": " + t.Objective})
		if err == nil {
			a.Budget.Record(people.With(ctx, o.Person), budget.Cost{USD: ans.CostUSD, Source: "judgment", Ref: "company:" + o.ID, Member: o.ID + "/" + t.Assignee})
		}
		// Without a judge to ask, the task starts as it would have before.
		t.Drift = err == nil && ans.P < driftThreshold
	}
	if err := a.Companies.SaveTask(ctx, t); err != nil {
		return company.Task{}, err
	}
	a.Events.Append(ctx, "company.task.assigned", actorFor(o, from), map[string]any{"company": o.ID, "task": t.ID, "from": from, "to": t.Assignee, "drift": t.Drift, "person": o.Person})
	w, err := a.enqueueTask(ctx, o, t, t.Drift)
	if err != nil {
		return t, err
	}
	if t.Drift {
		// It may have strayed: the assignee's boss says whether it starts.
		boss, _ := o.Boss(t.Assignee)
		root, _ := a.Companies.Task(ctx, t.Root)
		q := company.Question{From: t.Assignee, To: boss.ID, Kind: company.Decide, Work: w.ID, Task: t.ID, Drift: true,
			Text:    fmt.Sprintf("The task %q may not serve the objective it comes from (%q). Start it?", t.Title, root.Objective),
			Options: []string{"Start it", "Drop it"}}
		if _, err := a.putQuestion(ctx, o, q); err != nil {
			return t, err
		}
	}
	return t, nil
}

// enqueueTask queues a task's work, already waiting when it must not
// start before someone says so.
func (a *App) enqueueTask(ctx context.Context, o company.Org, t company.Task, hold bool) (company.Work, error) {
	return a.enqueue(ctx, o, t.Assignee, t.Handoff(o), nil, "task:"+t.ID, 0, func(w *company.Work) {
		w.Task = t.ID
		if hold {
			w.State = company.WorkWaiting
		}
	})
}

func actorFor(o company.Org, member string) string { return "member:" + o.ID + "/" + member }

// report closes or blocks a task and tells whoever gave it.
func (a *App) report(ctx context.Context, o company.Org, me, id, status, summary string, links []company.Link) error {
	t, err := a.Companies.Task(ctx, id)
	if err != nil || t.Company != o.ID || t.Assignee != me {
		return errors.New("there is no such task of yours")
	}
	state := map[string]string{"done": company.TaskDone, "blocked": company.TaskBlocked}[status]
	if state == "" || strings.TrimSpace(summary) == "" {
		return errors.New("say done or blocked, and what you did")
	}
	t, err = a.Companies.UpdateTask(ctx, id, func(x *company.Task) {
		x.State, x.Report = state, clip(summary, 4000)
		x.Dossier = append(x.Dossier, links...)
	})
	if err != nil {
		return err
	}
	a.Events.Append(ctx, "company.task.reported", actorFor(o, me), map[string]any{"company": o.ID, "task": t.ID, "state": t.State, "person": o.Person})
	from, _ := o.Member(me)
	text := fmt.Sprintf("%s reported on task %s (%s): %s\n\n%s", from.Name, t.ID, t.Title, status, summary)
	requester, ok := o.Member(t.Requester)
	if ok && requester.Kind == company.Agent {
		w, err := a.enqueue(ctx, o, requester.ID, text, nil, "report:"+t.ID, 0)
		if err == nil && t.Parent != "" {
			a.Companies.UpdateWork(ctx, w.ID, func(x *company.Work) { x.Task = t.Parent })
		}
		return err
	}
	a.Channel.Notify(ctx, explore.Notice{Text: o.Name + " · " + text, To: o.Person, Kind: "task"})
	return nil
}

// ask stops a piece of work on a question until it is answered.
func (a *App) ask(ctx context.Context, o company.Org, me string, work company.Work, text string, options []string, recommendation, context, to string, matter company.Matter) (company.Question, error) {
	if work.ID == "" {
		return company.Question{}, errors.New("a question is asked during a piece of work")
	}
	if strings.TrimSpace(text) == "" || len(text) > 2000 || len(options) > 8 {
		return company.Question{}, errors.New("ask in up to 2000 characters, with at most 8 options")
	}
	q := company.Question{From: me, Kind: company.Decide, Work: work.ID, Task: work.Task, Text: strings.TrimSpace(text), Options: options,
		Recommendation: clip(recommendation, 1000), Context: clip(context, 4000)}
	switch to {
	case "", "boss":
		boss, ok := o.Boss(me)
		if !ok {
			return company.Question{}, errors.New("you have no boss to ask")
		}
		q.To = boss.ID
		if level, why := a.levelOf(ctx, o, matter); level > 0 {
			l, _ := o.Levels.At(level)
			q.Level, q.Why = level, why
			if decider, ok := o.DeciderAt(me, l); ok && decider.ID != me {
				q.To = decider.ID
			}
			if q.To == company.CEO && l.Route == company.RouteOpinions {
				for _, up := range o.Chain(me) {
					if up.Kind == company.Agent {
						q.Consulted = append(q.Consulted, up.ID)
					}
				}
			}
		}
	case "requester":
		t, err := a.Companies.Task(ctx, work.Task)
		if err != nil {
			return company.Question{}, errors.New("only a task has someone to clarify it")
		}
		q.To, q.Kind = t.Requester, company.Clarify
	default:
		return company.Question{}, errors.New("ask your boss or whoever gave you the task")
	}
	return a.putQuestion(ctx, o, q)
}

// putQuestion records a question, stops its work and takes it to whoever
// answers: a person on their channels and in what needs them, or an agent
// as work of its own.
func (a *App) putQuestion(ctx context.Context, o company.Org, q company.Question) (company.Question, error) {
	q.ID, q.Company, q.Asked = newTeamID("q_"), o.ID, time.Now().UTC()
	if err := a.Companies.SaveQuestion(ctx, q); err != nil {
		return q, err
	}
	if q.Work != "" {
		a.Companies.UpdateWork(ctx, q.Work, func(w *company.Work) {
			w.Episodes = append(w.Episodes, company.Episode{Exploration: w.Exploration, Question: q.Text})
			w.Question, w.State = q.ID, company.WorkWaiting
		})
	}
	if q.Task != "" {
		a.Companies.UpdateTask(ctx, q.Task, func(t *company.Task) { t.State = company.TaskWaiting })
	}
	a.Events.Append(ctx, "company.question.asked", actorFor(o, q.From), map[string]any{"company": o.ID, "question": q.ID, "from": q.From, "to": q.To, "person": o.Person})
	from, _ := o.Member(q.From)
	to, _ := o.Member(q.To)
	for _, id := range q.Consulted {
		text := fmt.Sprintf("%s asks the CEO (question %s): %s\nGive your recommendation with company.answer; it is an opinion, the CEO decides.", from.Name, q.ID, q.Text)
		if len(q.Options) > 0 {
			text += "\nOptions: " + strings.Join(q.Options, "; ")
		}
		a.enqueue(ctx, o, id, text, nil, "opinion:"+q.ID, 0)
	}
	if to.Kind == company.Agent {
		text := fmt.Sprintf("%s asks you (question %s): %s", from.Name, q.ID, q.Text)
		if len(q.Options) > 0 {
			text += "\nOptions: " + strings.Join(q.Options, "; ")
		}
		if q.Recommendation != "" {
			text += "\nTheir recommendation: " + q.Recommendation
		}
		if q.Context != "" {
			text += "\n\nWhat they say you need to see, as data:\n" + q.Context
		}
		text += "\n\nAnswer with company.answer. If it is not yours to decide, ask your own boss first with company.ask."
		_, err := a.enqueue(ctx, o, to.ID, text, nil, "question:"+q.ID, 0)
		return q, err
	}
	actions := []explore.Action{}
	for i, opt := range q.Options {
		actions = append(actions, explore.Action{Label: opt, Data: "coq:" + q.ID + "." + strconv.Itoa(i)})
	}
	text := fmt.Sprintf("%s · %s asks: %s", o.Name, from.Name, q.Text)
	if q.Recommendation != "" {
		text += "\n" + q.Recommendation
	}
	a.Channel.Notify(ctx, explore.Notice{Text: text, Actions: actions, To: to.Person, Kind: "question"})
	return q, nil
}

// answerQuestion records the answer and lets the work go on from where it
// stopped.
func (a *App) answerQuestion(ctx context.Context, q company.Question, choice, reason, by string) (company.Question, error) {
	choice, err := q.Choice(choice)
	if err != nil {
		return q, err
	}
	q, err = a.Companies.AnswerQuestion(ctx, q.ID, choice, clip(reason, 2000), by)
	if err != nil {
		return q, err
	}
	o, _ := a.Companies.Org(ctx, q.Company)
	a.Events.Append(ctx, "company.question.answered", by, map[string]any{"company": q.Company, "question": q.ID, "answer": choice, "person": o.Person})
	if q.Drift && len(q.Options) == 2 && choice == q.Options[1] {
		a.Companies.UpdateTask(ctx, q.Task, func(t *company.Task) { t.State = company.TaskDropped })
		if w, err := a.Companies.Work(ctx, q.Work); err == nil {
			a.Companies.UpdateWork(ctx, w.ID, func(x *company.Work) {
				x.State, x.Error, x.Ended = company.WorkStopped, "dropped: "+choice, time.Now().UTC()
			})
		}
		return q, nil
	}
	answer := choice
	if q.Reason != "" {
		answer += " (" + q.Reason + ")"
	}
	a.Companies.UpdateWork(ctx, q.Work, func(w *company.Work) {
		if n := len(w.Episodes); n > 0 {
			w.Episodes[n-1].Answer = answer
		}
		if w.State == company.WorkWaiting {
			w.State, w.Question, w.Exploration = company.WorkQueued, "", ""
		}
	})
	if q.Task != "" {
		a.Companies.UpdateTask(ctx, q.Task, func(t *company.Task) {
			if t.State == company.TaskWaiting {
				t.State = company.TaskDoing
			}
		})
	}
	go a.pumpWork(context.WithoutCancel(ctx))
	return q, nil
}

// pauseEpisode keeps what a stretch of work did when it stopped on a
// question, so it goes on from there.
func (a *App) pauseEpisode(ctx context.Context, id string, e store.Exploration) {
	var actions []string
	if e.Trace != nil {
		for _, c := range e.Trace.Calls {
			if c.Capability == "company.ask" {
				continue
			}
			s := c.Capability
			if c.Error != "" {
				s += " (refused or failed)"
			}
			actions = append(actions, s)
			if len(actions) == 30 {
				break
			}
		}
	}
	a.Companies.UpdateWork(ctx, id, func(w *company.Work) {
		w.CostUSD += e.CostUSD
		if n := len(w.Episodes); n > 0 {
			w.Episodes[n-1].Exploration, w.Episodes[n-1].Summary, w.Episodes[n-1].Actions, w.Episodes[n-1].Kept = e.ID, clip(e.Summary, 1500), actions, true
		}
		if w.Question == "" && w.State == company.WorkQueued {
			return
		}
		w.Exploration = ""
	})
}

// companyQuestionNeeds are the company questions the person ctx acts for
// answers: as the CEO, or as a partner who may approve.
func (a *App) companyQuestionNeeds(ctx context.Context) []need {
	me := people.From(ctx)
	out := []need{}
	pending, _ := a.Companies.Questions(ctx, "", true)
	orgs := map[string]company.Org{}
	for _, q := range pending {
		o, ok := orgs[q.Company]
		if !ok {
			var err error
			if o, err = a.Companies.Org(ctx, q.Company); err != nil {
				continue
			}
			orgs[q.Company] = o
		}
		to, _ := o.Member(q.To)
		if to.Kind != company.Person || !company.Allows(o.Grant(me), company.Approve) || to.Person != me && o.Person != me {
			continue
		}
		from, _ := o.Member(q.From)
		out = append(out, need{ID: q.ID, Title: from.Name + ": " + q.Text, Detail: q.Recommendation, Created: q.Asked, Urgency: urgencyAnswer,
			Options: q.Options, Actions: []string{"answer", "type"}, Link: "/companies/" + o.ID})
	}
	return out
}

func (a *App) companyTeamRoutes() {
	a.Server.Handle("GET /api/companies/{id}/tasks", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.Tasks(r.Context(), o.ID)
	}))
	a.Server.Handle("POST /api/companies/{id}/tasks", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var t company.Task
		if err := server.Decode(r, &t); err != nil {
			return nil, err
		}
		return a.assign(r.Context(), o, company.CEO, company.Work{}, company.Task{Assignee: t.Assignee, Title: strings.TrimSpace(t.Title), Objective: t.Objective,
			Acceptance: t.Acceptance, Constraints: t.Constraints, OutOfScope: t.OutOfScope, Due: t.Due, Priority: t.Priority, Dossier: t.Dossier})
	}))
	a.Server.Handle("POST /api/companies/{id}/tasks/{part}/drop", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		t, err := a.Companies.Task(r.Context(), r.PathValue("part"))
		if err != nil || t.Company != o.ID {
			return nil, company.ErrNotFound
		}
		return a.Companies.UpdateTask(r.Context(), t.ID, func(x *company.Task) { x.State = company.TaskDropped })
	}))
	a.Server.Handle("GET /api/companies/{id}/questions", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.Questions(r.Context(), o.ID, r.URL.Query().Get("all") == "")
	}))
	a.Server.Handle("POST /api/companies/{id}/questions/{part}/answer", a.companyRoute(company.Approve, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var in struct {
			Choice string `json:"choice"`
			Reason string `json:"reason"`
		}
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		q, err := a.Companies.Question(r.Context(), r.PathValue("part"))
		if err != nil || q.Company != o.ID {
			return nil, company.ErrNotFound
		}
		if to, _ := o.Member(q.To); to.Kind != company.Person {
			return nil, server.StatusError{Status: 409, Msg: "this question is for " + to.Name + " to answer"}
		}
		return a.answerQuestion(r.Context(), q, in.Choice, in.Reason, actor(r.Context()))
	}))
}

// answerCompanyButton answers a company question from a channel's button.
func (a *App) answerCompanyButton(ctx context.Context, id string) (string, error) {
	qid, i, err := parseAnswer(id)
	if err != nil {
		return "", err
	}
	q, err := a.Companies.Question(ctx, qid)
	if err != nil {
		return "", err
	}
	o, err := a.myCompany(ctx, q.Company, company.Approve)
	if err != nil {
		return "", err
	}
	if to, _ := o.Member(q.To); to.Kind != company.Person || i < 0 || i >= len(q.Options) {
		return "", errors.New("this question is not yours to answer")
	}
	if _, err := a.answerQuestion(ctx, q, q.Options[i], "", actor(ctx)); err != nil {
		return "", err
	}
	return "✅ " + q.Options[i], nil
}

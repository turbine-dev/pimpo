package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// A company remembers what it decided and learned, holds meetings with
// minutes, and tells its CEO each week what was done.

const (
	memoryInBrief  = 3000
	recallLimit    = 10
	meetingTurnUSD = 0.10
	digestKey      = "company.digest."
)

const minutesSchema = `{"type":"object","properties":{"minutes":{"type":"string"},"decisions":{"type":"array","items":{"type":"string"}}},"required":["minutes","decisions"]}`

func init() {
	for _, s := range []capability.Spec{
		{Name: "company.remember", Risk: capability.Notify, Signature: "company.remember({kind, title, text})",
			Returns: "{note: id}; keeps something in the company's memory for every member: kind is fact, decision or lesson; your words, never a rule",
			Schema:  `{"type":"object","properties":{"kind":{"type":"string","enum":["fact","decision","lesson"]},"title":{"type":"string"},"text":{"type":"string"}},"required":["title","text"]}`},
		{Name: "company.recall", Risk: capability.Read, Signature: "company.recall({query})",
			Returns: "[{kind, title, text, by, date}]: notes of the company's memory with every word of the query",
			Schema:  `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`},
		{Name: "company.meet", Risk: capability.Notify, Signature: "company.meet({title, agenda, participants, rounds})",
			Returns: "{meeting: id}; calls a meeting you chair with other agents (ids), up to 4 rounds; its minutes and decisions go to the company's memory",
			Schema:  `{"type":"object","properties":{"title":{"type":"string"},"agenda":{"type":"string"},"participants":{"type":"array","items":{"type":"string"}},"rounds":{"type":"integer"}},"required":["title","agenda","participants"]}`},
	} {
		capability.Register(s)
	}
}

type memoryCap struct{ a *App }

func (memoryCap) Capabilities() []string {
	return []string{"company.remember", "company.recall", "company.meet"}
}

func (c memoryCap) Call(ctx context.Context, name, _ string, args any) (any, error) {
	o, me, work, err := c.a.caller(ctx)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(args)
	switch name {
	case "company.remember":
		var in struct{ Kind, Title, Text string }
		json.Unmarshal(b, &in)
		if in.Kind == "" {
			in.Kind = company.NoteFact
		}
		if in.Kind != company.NoteFact && in.Kind != company.NoteDecision && in.Kind != company.NoteLesson {
			return nil, errors.New("kind is fact, decision or lesson")
		}
		n, err := c.a.keepNote(ctx, o, company.Note{Kind: in.Kind, Title: in.Title, Body: in.Text, By: actorFor(o, me), Source: "work:" + work.ID})
		if err != nil {
			return nil, err
		}
		return map[string]string{"note": n.ID}, nil
	case "company.recall":
		var in struct{ Query string }
		json.Unmarshal(b, &in)
		notes, err := c.a.Companies.Recall(ctx, o.ID, in.Query, recallLimit)
		if err != nil {
			return nil, err
		}
		out := []map[string]string{}
		for _, n := range notes {
			out = append(out, map[string]string{"kind": n.Kind, "title": n.Title, "text": n.Body, "by": n.By, "date": n.Created.Format("2006-01-02")})
		}
		return out, nil
	case "company.meet":
		var in struct {
			Title, Agenda string
			Participants  []string
			Rounds        int
		}
		json.Unmarshal(b, &in)
		if in.Rounds == 0 {
			in.Rounds = 2
		}
		parts := in.Participants
		if !slices.Contains(parts, me) {
			parts = append([]string{me}, parts...)
		}
		m, err := c.a.meet(ctx, o, company.Meeting{Title: in.Title, Agenda: in.Agenda, Chair: me, Participants: parts, Rounds: in.Rounds, MaxUSD: 1, CalledBy: actorFor(o, me)})
		if err != nil {
			return nil, err
		}
		return map[string]string{"meeting": m.ID}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

func (a *App) keepNote(ctx context.Context, o company.Org, n company.Note) (company.Note, error) {
	n.Title, n.Body = strings.TrimSpace(n.Title), strings.TrimSpace(n.Body)
	if n.Title == "" || n.Body == "" || len([]rune(n.Title)) > 120 || len(n.Body) > 8000 {
		return n, errors.New("a note needs a title of up to 120 characters and a text under 8000")
	}
	n.ID, n.Company, n.Created = newTeamID("n_"), o.ID, time.Now().UTC()
	if err := a.Companies.SaveNote(ctx, n); err != nil {
		return n, err
	}
	a.Events.Append(ctx, "company.note.saved", n.By, map[string]string{"company": o.ID, "note": n.ID, "kind": n.Kind, "person": o.Person})
	return n, nil
}

// meet starts a meeting in the background.
func (a *App) meet(ctx context.Context, o company.Org, m company.Meeting) (company.Meeting, error) {
	if m.MaxUSD == 0 {
		m.MaxUSD = 1
	}
	if err := o.CheckMeeting(m); err != nil {
		return m, err
	}
	for _, p := range m.Participants {
		if ok, why := o.Working(p); !ok {
			return m, errors.New(why)
		}
	}
	m.ID, m.Company, m.State, m.Created, m.Transcript = newTeamID("m_"), o.ID, company.MeetingRunning, time.Now().UTC(), []company.Turn{}
	if err := a.Companies.SaveMeeting(ctx, m); err != nil {
		return m, err
	}
	a.Events.Append(ctx, "company.meeting.started", m.CalledBy, map[string]string{"company": o.ID, "meeting": m.ID, "person": o.Person})
	go a.holdMeeting(context.WithoutCancel(people.With(ctx, o.Person)), o, m)
	return m, nil
}

// holdMeeting lets each participant speak in turn for the rounds, then
// has the chair write the minutes. Talking uses no tools; it stops at the
// meeting's cost cap.
func (a *App) holdMeeting(ctx context.Context, o company.Org, m company.Meeting) {
	fail := func(err error) {
		m.State, m.Error, m.Ended = company.MeetingFailed, err.Error(), time.Now().UTC()
		a.Companies.SaveMeeting(ctx, m)
		a.Events.Append(ctx, "company.meeting.ended", "system", map[string]any{"company": o.ID, "meeting": m.ID, "state": m.State, "person": o.Person})
	}
	say := func(member, system, prompt string, schema string) (llm.Response, error) {
		left := m.MaxUSD - m.CostUSD
		if left <= 0.005 {
			return llm.Response{}, errors.New("the meeting reached its cost cap")
		}
		req := llm.Request{System: system, Prompt: prompt, MaxCostUSD: min(meetingTurnUSD, left)}
		if schema != "" {
			req.Schema = json.RawMessage(schema)
		}
		mem, _ := o.Member(member)
		role, _ := o.Role(mem.Role)
		if models := firstNonEmptyList(mem.Models, role.Models); len(models) > 0 {
			req.Model = models[0]
		}
		resp, err := a.generate(withAssistant(ctx, firstNonEmptyList(mem.Models, role.Models)), req)
		if resp.CostUSD > 0 {
			m.CostUSD += resp.CostUSD
			a.Budget.Record(ctx, budget.Cost{USD: resp.CostUSD, Source: "meeting", Ref: "company:" + o.ID + "/meeting:" + m.ID, Member: o.ID + "/" + member})
		}
		return resp, err
	}
	notes, _ := a.Companies.Notes(ctx, o.ID, 50)
	memory := company.Memory(notes, memoryInBrief)
	for round := 1; round <= m.Rounds; round++ {
		for _, p := range m.Participants {
			mem, _ := o.Member(p)
			system := o.Brief(p) + "\n\n" + memory + "\n\nYou are in a meeting of the company. Speak briefly (at most 150 words), as yourself, to move the agenda forward. You cannot act from a meeting; say what you would do."
			prompt := fmt.Sprintf("Meeting: %s\nAgenda: %s\nRound %d of %d.\n\nSo far:\n%s\n\nYour turn, %s.", m.Title, m.Agenda, round, m.Rounds, transcript(o, m.Transcript), mem.Name)
			resp, err := say(p, system, prompt, "")
			if err != nil {
				fail(err)
				return
			}
			m.Transcript = append(m.Transcript, company.Turn{Member: p, Text: clip(strings.TrimSpace(resp.Text), 2000)})
			a.Companies.SaveMeeting(ctx, m)
		}
	}
	resp, err := say(m.Chair, o.Brief(m.Chair), fmt.Sprintf("You chaired the meeting %q on: %s\n\n%s\n\nWrite its minutes (a short paragraph) and the decisions taken, one per item; none if nothing was decided.", m.Title, m.Agenda, transcript(o, m.Transcript)), minutesSchema)
	if err != nil {
		fail(err)
		return
	}
	var out struct {
		Minutes   string   `json:"minutes"`
		Decisions []string `json:"decisions"`
	}
	json.Unmarshal(resp.Structured, &out)
	m.Minutes, m.Decisions, m.State, m.Ended = clip(out.Minutes, 4000), out.Decisions, company.MeetingDone, time.Now().UTC()
	a.Companies.SaveMeeting(ctx, m)
	body := m.Minutes
	if len(m.Decisions) > 0 {
		body += "\n\nDecisions:\n- " + strings.Join(m.Decisions, "\n- ")
	}
	a.keepNote(ctx, o, company.Note{Kind: company.NoteMinutes, Title: m.Title, Body: body, By: actorFor(o, m.Chair), Source: "meeting:" + m.ID})
	a.Events.Append(ctx, "company.meeting.ended", "system", map[string]any{"company": o.ID, "meeting": m.ID, "state": m.State, "cost_usd": m.CostUSD, "person": o.Person})
}

func firstNonEmptyList(lists ...[]string) []string {
	for _, l := range lists {
		if len(l) > 0 {
			return l
		}
	}
	return nil
}

func transcript(o company.Org, turns []company.Turn) string {
	if len(turns) == 0 {
		return "(nothing yet)"
	}
	var b strings.Builder
	for _, t := range turns {
		m, _ := o.Member(t.Member)
		fmt.Fprintf(&b, "%s: %s\n", m.Name, t.Text)
	}
	return b.String()
}

// digest is what a company did since a time, without a model: work done
// and failed by member, tasks closed, questions waiting and the cost.
func (a *App) digest(ctx context.Context, o company.Org, since time.Time) string {
	works, _ := a.Companies.Works(ctx, o.ID, 500)
	type tally struct {
		done, failed int
		cost         float64
		last         []string
	}
	by := map[string]*tally{}
	total := 0.0
	for _, w := range works {
		if w.Queued.Before(since) {
			continue
		}
		t := by[w.Member]
		if t == nil {
			t = &tally{}
			by[w.Member] = t
		}
		t.cost += w.CostUSD
		total += w.CostUSD
		switch w.State {
		case company.WorkDone:
			t.done++
			if len(t.last) < 3 && w.Summary != "" {
				t.last = append(t.last, clip(w.Summary, 200))
			}
		case company.WorkFailed:
			t.failed++
		}
	}
	var b strings.Builder
	for _, m := range o.Members {
		t := by[m.ID]
		if t == nil {
			continue
		}
		fmt.Fprintf(&b, "%s: %d done, %d failed, $%.2f\n", m.Name, t.done, t.failed, t.cost)
		for _, l := range t.last {
			b.WriteString("  - " + l + "\n")
		}
	}
	tasks, _ := a.Companies.Tasks(ctx, o.ID)
	closed := 0
	for _, t := range tasks {
		if t.State == company.TaskDone && !t.Updated.Before(since) {
			closed++
		}
	}
	waiting, _ := a.Companies.Questions(ctx, o.ID, true)
	fmt.Fprintf(&b, "Tasks closed: %d. Questions waiting: %d. Cost: $%.2f.", closed, len(waiting), total)
	return b.String()
}

// digestLoop sends each company's CEO a summary of the week on Mondays.
func (a *App) digestLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.sendDigests(ctx, time.Now())
		}
	}
}

func (a *App) sendDigests(ctx context.Context, now time.Time) {
	if !a.chose(ctx, companiesLab) {
		return
	}
	all, _ := a.Companies.List(ctx)
	for _, c := range all {
		zone, err := time.LoadLocation(c.Zone)
		if err != nil {
			zone = time.Local
		}
		local := now.In(zone)
		week := local.Format("2006-01-02")
		if local.Weekday() != time.Monday || local.Hour() < 9 {
			continue
		}
		if last, _ := a.Events.Get(ctx, digestKey+c.ID); last == week {
			continue
		}
		o, err := a.Companies.Org(ctx, c.ID)
		if err != nil {
			continue
		}
		a.Events.Put(ctx, digestKey+c.ID, week)
		text := a.digest(ctx, o, local.AddDate(0, 0, -7))
		a.Channel.Notify(people.With(ctx, o.Person), explore.Notice{Text: o.Name + " · " + text, To: o.Person, Kind: "task"})
	}
}

func (a *App) companyMemoryRoutes() {
	a.Server.Handle("GET /api/companies/{id}/notes", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.Notes(r.Context(), o.ID, 200)
	}))
	a.Server.Handle("POST /api/companies/{id}/notes", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var n company.Note
		if err := server.Decode(r, &n); err != nil {
			return nil, err
		}
		if n.Kind != company.NoteFact && n.Kind != company.NoteDecision && n.Kind != company.NoteLesson {
			n.Kind = company.NoteFact
		}
		return a.keepNote(r.Context(), o, company.Note{Kind: n.Kind, Title: n.Title, Body: n.Body, By: actor(r.Context())})
	}))
	a.Server.Handle("DELETE /api/companies/{id}/notes/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return map[string]bool{"ok": true}, a.Companies.DeleteNote(r.Context(), o.ID, r.PathValue("part"))
	}))
	a.Server.Handle("GET /api/companies/{id}/meetings", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.Meetings(r.Context(), o.ID)
	}))
	a.Server.Handle("POST /api/companies/{id}/meetings", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var m company.Meeting
		if err := server.Decode(r, &m); err != nil {
			return nil, err
		}
		return a.meet(r.Context(), o, company.Meeting{Title: m.Title, Agenda: m.Agenda, Chair: m.Chair, Participants: m.Participants, Rounds: m.Rounds, MaxUSD: m.MaxUSD, CalledBy: actor(r.Context())})
	}))
	a.Server.Handle("GET /api/companies/{id}/digest", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		days := 7
		if d := r.URL.Query().Get("days"); d == "1" {
			days = 1
		}
		return map[string]string{"text": a.digest(r.Context(), o, time.Now().AddDate(0, 0, -days))}, nil
	}))
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/budget"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"golang.org/x/text/transform"
)

// The CEO talks with the company's agents: one of them, to talk over its
// work or a question it asked, or several in a meeting, as in a real
// company. Each agent answers what the CEO says when it is named, or all
// of them when nobody is, in turn, each hearing the others. Talking uses
// no tools; anything said can become a task.

const (
	sayMax       = 4000
	meetDefault  = 1.0
	recentWorkIn = 5
)

var meetMu sync.Map // meeting id → *sync.Mutex, one exchange at a time

func meetLock(id string) *sync.Mutex {
	m, _ := meetMu.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// openMeeting starts a meeting the CEO takes part in.
func (a *App) openMeeting(ctx context.Context, o company.Org, m company.Meeting) (company.Meeting, error) {
	m.WithCEO, m.Chair, m.Rounds = true, company.CEO, 0
	if m.MaxUSD == 0 {
		m.MaxUSD = meetDefault
	}
	if m.Question != "" {
		q, err := a.Companies.Question(ctx, m.Question)
		if err != nil || q.Company != o.ID {
			return m, errors.New("no such question")
		}
		if m.Agenda == "" {
			m.Agenda = q.Text
		}
		if !slices.Contains(m.Participants, q.From) {
			m.Participants = append([]string{q.From}, m.Participants...)
		}
	}
	if err := o.CheckMeeting(m); err != nil {
		return m, err
	}
	m.ID, m.Company, m.State, m.Created, m.Transcript = newTeamID("m_"), o.ID, company.MeetingOpen, time.Now().UTC(), []company.Turn{}
	if err := a.Companies.SaveMeeting(ctx, m); err != nil {
		return m, err
	}
	a.Events.Append(ctx, "company.meeting.started", actor(ctx), map[string]string{"company": o.ID, "meeting": m.ID, "person": o.Person})
	return m, nil
}

// named are the participants the CEO speaks to: those in to, or those
// named in the text with @, or everyone.
func named(o company.Org, m company.Meeting, text string, to []string) []string {
	var out []string
	for _, p := range m.Participants {
		if slices.Contains(to, p) {
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		return out
	}
	lower := foldName(text)
	for _, p := range m.Participants {
		mem, _ := o.Member(p)
		if strings.Contains(lower, "@"+foldName(mem.Name)) || strings.Contains(lower, "@"+p) {
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		return out
	}
	return m.Participants
}

func foldName(s string) string {
	out, _, err := transform.String(accents, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(out)
}

// say records what the CEO said and has the agents spoken to answer, in
// the background.
func (a *App) say(ctx context.Context, o company.Org, id, text string, to []string) (company.Meeting, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > sayMax {
		return company.Meeting{}, errors.New("say something, in under 4000 characters")
	}
	lock := meetLock(id)
	if !lock.TryLock() {
		return company.Meeting{}, server.StatusError{Status: 409, Msg: "the agents are still answering"}
	}
	m, err := a.Companies.Meeting(ctx, id)
	if err != nil || m.Company != o.ID {
		lock.Unlock()
		return company.Meeting{}, company.ErrNotFound
	}
	if m.State != company.MeetingOpen {
		lock.Unlock()
		return m, errors.New("this meeting is over")
	}
	m.Transcript = append(m.Transcript, company.Turn{Member: company.CEO, Text: text})
	a.Companies.SaveMeeting(ctx, m)
	a.Events.Append(ctx, "company.meeting.turn", actor(ctx), map[string]string{"company": o.ID, "meeting": m.ID, "person": o.Person})
	who := named(o, m, text, to)
	a.goWork(people.With(ctx, o.Person), func(c context.Context) {
		defer lock.Unlock()
		a.answerCEO(c, o, m.ID, who)
	})
	return m, nil
}

// answerCEO has each agent spoken to answer, in turn.
func (a *App) answerCEO(ctx context.Context, o company.Org, id string, who []string) {
	m, err := a.Companies.Meeting(ctx, id)
	if err != nil {
		return
	}
	notes, _ := a.Companies.Notes(ctx, o.ID, 300)
	ceo, _ := o.Member(company.CEO)
	for _, p := range who {
		left := m.MaxUSD - m.CostUSD
		if left <= 0.005 {
			m.State, m.Error, m.Ended = company.MeetingDone, "the meeting reached its cost cap", time.Now().UTC()
			break
		}
		mem, _ := o.Member(p)
		role, _ := o.Role(mem.Role)
		models := firstNonEmptyList(mem.Models, role.Models)
		system := o.Brief(p) + "\n\n" + company.Memory(company.Visible(notes, p, nil), memoryInBrief) + a.recentWork(ctx, o, p) + a.questionContext(ctx, m) +
			fmt.Sprintf("\n\nYou are talking with %s, the CEO, about: %s. Answer as yourself, plainly and briefly (at most 200 words). You cannot act from a conversation; say what you would do, and the CEO may give it to you as a task.", ceo.Name, m.Agenda)
		req := llm.Request{System: system, Prompt: "The conversation so far:\n" + transcript(o, m.Transcript) + "\n\nYour turn, " + mem.Name + ".", MaxCostUSD: min(meetingTurnUSD, left)}
		if len(models) > 0 {
			req.Model = models[0]
		}
		resp, err := a.generate(withAssistant(ctx, models), req)
		if resp.CostUSD > 0 {
			m.CostUSD += resp.CostUSD
			a.Budget.Record(ctx, budget.Cost{USD: resp.CostUSD, Source: "meeting", Ref: "company:" + o.ID + "/meeting:" + m.ID, Member: o.ID + "/" + p})
		}
		text := strings.TrimSpace(resp.Text)
		if err != nil {
			text = "(" + err.Error() + ")"
		}
		m.Transcript = append(m.Transcript, company.Turn{Member: p, Text: clip(text, 2000)})
		a.Companies.SaveMeeting(ctx, m)
		a.Events.Append(ctx, "company.meeting.turn", "member:"+o.ID+"/"+p, map[string]string{"company": o.ID, "meeting": m.ID, "person": o.Person})
	}
	a.Companies.SaveMeeting(ctx, m)
}

// recentWork is what a member did lately, so it can talk about its work.
func (a *App) recentWork(ctx context.Context, o company.Org, member string) string {
	works, _ := a.Companies.Works(ctx, o.ID, 100)
	var lines []string
	for _, w := range works {
		if w.Member == member && w.Summary != "" && len(lines) < recentWorkIn {
			lines = append(lines, fmt.Sprintf("- %s: %s", clip(w.Request, 120), clip(w.Summary, 300)))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n\n## Your recent work\n\n" + strings.Join(lines, "\n")
}

func (a *App) questionContext(ctx context.Context, m company.Meeting) string {
	if m.Question == "" {
		return ""
	}
	q, err := a.Companies.Question(ctx, m.Question)
	if err != nil {
		return ""
	}
	out := "\n\n## The question you asked, waiting for the CEO\n\n" + q.Text
	if len(q.Options) > 0 {
		out += "\nOptions: " + strings.Join(q.Options, "; ")
	}
	if q.Recommendation != "" {
		out += "\nYour recommendation: " + q.Recommendation
	}
	return out
}

// endMeeting closes a meeting the CEO is in and keeps its minutes.
func (a *App) endMeeting(ctx context.Context, o company.Org, id string) (company.Meeting, error) {
	lock := meetLock(id)
	lock.Lock()
	defer lock.Unlock()
	m, err := a.Companies.Meeting(ctx, id)
	if err != nil || m.Company != o.ID {
		return company.Meeting{}, company.ErrNotFound
	}
	if !m.WithCEO || m.Minutes != "" {
		return m, errors.New("this meeting is over")
	}
	m.State, m.Ended = company.MeetingDone, time.Now().UTC()
	if len(m.Transcript) > 1 {
		resp, err := a.generate(people.With(ctx, o.Person), llm.Request{System: "You keep the minutes of a company's meetings.",
			Prompt: fmt.Sprintf("Meeting %q about: %s\n\n%s\n\nWrite its minutes (a short paragraph) and the decisions taken, one per item; none if nothing was decided.", m.Title, m.Agenda, transcript(o, m.Transcript)),
			Schema: json.RawMessage(minutesSchema), MaxCostUSD: meetingTurnUSD})
		if resp.CostUSD > 0 {
			m.CostUSD += resp.CostUSD
			a.Budget.Record(people.With(ctx, o.Person), budget.Cost{USD: resp.CostUSD, Source: "meeting", Ref: "company:" + o.ID + "/meeting:" + m.ID, Member: o.ID + "/"})
		}
		var out struct {
			Minutes   string   `json:"minutes"`
			Decisions []string `json:"decisions"`
		}
		if err == nil && json.Unmarshal(resp.Structured, &out) == nil {
			m.Minutes, m.Decisions = clip(out.Minutes, 4000), out.Decisions
			body := m.Minutes
			if len(m.Decisions) > 0 {
				body += "\n\nDecisions:\n- " + strings.Join(m.Decisions, "\n- ")
			}
			a.keepNote(ctx, o, company.Note{Kind: company.NoteMinutes, Title: m.Title, Body: body, By: actor(ctx), Source: "meeting:" + m.ID})
		}
	}
	a.Companies.SaveMeeting(ctx, m)
	a.Events.Append(ctx, "company.meeting.ended", actor(ctx), map[string]any{"company": o.ID, "meeting": m.ID, "state": m.State, "person": o.Person})
	return m, nil
}

func (a *App) companyMeetRoutes() {
	a.Server.Handle("GET /api/companies/{id}/meetings/{part}", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		m, err := a.Companies.Meeting(r.Context(), r.PathValue("part"))
		if err != nil || m.Company != o.ID {
			return nil, company.ErrNotFound
		}
		return m, nil
	}))
	a.Server.Handle("POST /api/companies/{id}/meetings/{part}/say", a.companyRoute(company.Approve, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var in struct {
			Text string   `json:"text"`
			To   []string `json:"to"`
		}
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		return a.say(r.Context(), o, r.PathValue("part"), in.Text, in.To)
	}))
	a.Server.Handle("POST /api/companies/{id}/meetings/{part}/end", a.companyRoute(company.Approve, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.endMeeting(r.Context(), o, r.PathValue("part"))
	}))
	a.Server.Handle("POST /api/companies/{id}/meetings/{part}/task", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var in struct {
			Member  string `json:"member"`
			Request string `json:"request"`
		}
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		m, err := a.Companies.Meeting(r.Context(), r.PathValue("part"))
		if err != nil || m.Company != o.ID {
			return nil, company.ErrNotFound
		}
		// The conversation goes with the task, as what it came from.
		return a.enqueue(r.Context(), o, in.Member, in.Request, map[string]string{"meeting": m.Title, "conversation": transcript(o, m.Transcript)}, "meeting:"+m.ID, 0)
	}))
}

package app

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/turbine-dev/pimpo/internal/approval"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// What needs you: everything waiting for the person asking, in one list,
// the most urgent first and, within the same urgency, the newest first.
// Each kind comes from a small source that lists only the caller's own
// items through the same scoped queries its own page uses, so the owner
// never sees a member's approvals or questions here, and nobody sees the
// owner's.
//
// Adding a kind is one more source in needSources: list the caller's own
// items, give them a kind, an urgency and the actions the list may offer,
// and teach the UI's Needs component that kind's buttons.

// Urgency ranks, highest first.
const (
	urgencyNow      = 4 // an approval about to expire
	urgencyDecide   = 3 // an approval: something is waiting on it
	urgencyAnswer   = 2 // a question from a routine
	urgencyFailure  = 1 // something stopped working
	urgencyWhenFree = 0 // ready to look at when there is time
)

// soonWindow is how close to its deadline an approval counts as urgent.
const soonWindow = 10 * time.Minute

// jobErrorWindow is how long a job that went wrong stays on the list.
const jobErrorWindow = 7 * 24 * time.Hour

type need struct {
	Kind    string    `json:"kind"`
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Detail  string    `json:"detail,omitempty"`
	Created time.Time `json:"created,omitzero"`
	Urgency int       `json:"urgency"`
	Expires time.Time `json:"expires,omitzero"`
	// Link is the page that shows the item whole.
	Link string `json:"link,omitempty"`
	// Actions are what may be done from the list, by name; the UI gives
	// them their labels.
	Actions []string `json:"actions"`
	// Options are a question's answers, in order.
	Options []string `json:"options,omitempty"`
	// Proposal is what saying yes would start, such as a suggestion's
	// request.
	Proposal string `json:"proposal,omitempty"`
	// Risk is an approval's risk, for its color.
	Risk int `json:"risk,omitempty"`
	// Amount is what an approval that may be granted for its routine
	// moves, when it moves one: the least limit such a grant may have.
	Amount *float64 `json:"amount,omitempty"`
	// Count is how many a summary item stands for, such as lessons.
	Count int `json:"count,omitempty"`
}

// needSource lists one kind of what waits for the person ctx acts for.
// minRole is the least role that sees the kind at all.
type needSource struct {
	kind    string
	minRole people.Role
	list    func(context.Context) []need
}

func (a *App) needSources() []needSource {
	return []needSource{
		{"approval", people.Member, a.approvalNeeds},
		{"credential_request", people.Member, a.credentialNeeds},
		{"question", people.Guest, a.questionNeeds},
		{"company_question", people.Member, a.companyQuestionNeeds},
		{"company_note", people.Member, a.pendingNoteNeeds},
		{"failed_routine", people.Member, a.failedRoutineNeeds},
		{"job_error", people.Member, a.jobNeeds(true)},
		{"job_planned", people.Member, a.jobNeeds(false)},
		{"exploration_ready", people.Guest, a.readyNeeds},
		{"suggestion", people.Owner, a.suggestionNeeds},
		{"system", people.Owner, a.systemNeeds},
		{"lesson", people.Member, a.lessonNeeds},
	}
}

// roleRank orders roles so a source can say who may see it.
func roleRank(r people.Role) int {
	switch r {
	case people.Owner:
		return 2
	case people.Member:
		return 1
	}
	return 0
}

// needs merges every source the caller may see into one ordered list.
func (a *App) needs(ctx context.Context) ([]need, map[string]int) {
	role := roleRank(a.roleOf(ctx))
	out := []need{}
	counts := map[string]int{}
	for _, s := range a.needSources() {
		if role < roleRank(s.minRole) {
			continue
		}
		items := s.list(ctx)
		for i := range items {
			items[i].Kind = s.kind
			if items[i].Actions == nil {
				items[i].Actions = []string{}
			}
		}
		counts[s.kind] = len(items)
		out = append(out, items...)
	}
	sortNeeds(out)
	return out, counts
}

// sortNeeds puts the most urgent first and, within an urgency, the newest.
func sortNeeds(list []need) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Urgency != list[j].Urgency {
			return list[i].Urgency > list[j].Urgency
		}
		return list[i].Created.After(list[j].Created)
	})
}

func (a *App) needRoutes() {
	a.Server.Handle("GET /api/needs", func(w http.ResponseWriter, r *http.Request) {
		items, counts := a.needs(r.Context())
		server.WriteJSON(w, 200, map[string]any{"items": items, "counts": counts, "total": len(items)})
	})
}

func (a *App) approvalNeeds(ctx context.Context) []need {
	timeout := a.Approvals.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	now := time.Now()
	out := []need{}
	for _, q := range a.myApprovals(ctx) {
		n := need{ID: q.ID, Title: q.Text, Detail: q.Reason, Created: q.Created, Expires: q.Created.Add(timeout),
			Urgency: urgencyDecide, Risk: int(q.Action.Risk), Actions: []string{"once", "run", "always", "deny"}}
		if n.Expires.Sub(now) < soonWindow {
			n.Urgency = urgencyNow
		}
		// A member's "always" would be a lasting rule, which is the owner's.
		if people.From(ctx) != people.OwnerID {
			n.Actions = []string{"once", "run", "deny"}
		}
		// "For this routine" repeats exactly this operation without asking,
		// up to the amount it moves or a higher limit the person sets.
		if q.Grantable {
			n.Actions = append(n.Actions, "routine")
			for _, v := range approval.OperationOf(q.Action.Capability, q.Action.Args).Amounts {
				if n.Amount == nil || v > *n.Amount {
					n.Amount = &v
				}
			}
		}
		out = append(out, n)
	}
	return out
}

func (a *App) questionNeeds(ctx context.Context) []need {
	me := people.Norm(people.From(ctx))
	out := []need{}
	for _, q := range a.questions(ctx) {
		if people.Norm(q.Person) != me {
			continue
		}
		n := need{ID: q.ID, Title: q.Question, Created: q.Asked, Expires: q.Expires, Urgency: urgencyAnswer,
			Options: q.Options, Actions: []string{"answer", "type"}}
		if q.Routine != "" {
			n.Link = "/routines/" + q.Routine
		}
		out = append(out, n)
	}
	return out
}

func (a *App) failedRoutineNeeds(ctx context.Context) []need {
	routines, _ := a.myRoutines(ctx)
	out := []need{}
	for _, rt := range routines {
		if rt.State != store.RoutineBroken {
			continue
		}
		n := need{ID: rt.ID, Title: rt.Body.Name, Created: rt.UpdatedAt, Urgency: urgencyFailure,
			Link: "/routines/" + rt.ID, Actions: []string{"run", "repair", "open"}}
		if runs, err := a.Store.Runs(ctx, rt.ID, 1); err == nil && len(runs) > 0 {
			n.Created, n.Detail = runs[0].StartedAt, clip(runs[0].Error, 200)
		}
		out = append(out, n)
	}
	return out
}

// jobNeeds lists the caller's jobs that went wrong (failed, stopped by
// the budget, or with parts that failed) or, with broken false, the plans
// waiting to be started.
func (a *App) jobNeeds(broken bool) func(context.Context) []need {
	return func(ctx context.Context) []need {
		me := people.Norm(people.From(ctx))
		byOwner := i18n.T(ctx, "msg.job.byOwner")
		out := []need{}
		for _, id := range a.jobIDs(ctx) {
			j, ok := a.job(ctx, id)
			if !ok || people.Norm(j.Person) != me {
				continue
			}
			n := need{ID: j.ID, Title: clip(j.Request, 160), Created: j.Updated, Link: "/jobs/" + j.ID, Actions: []string{"open"}}
			if !broken {
				if j.State == JobPlanned {
					n.Urgency, n.Created = urgencyWhenFree, j.Created
					out = append(out, n)
				}
				continue
			}
			if time.Since(j.Updated) > jobErrorWindow {
				continue
			}
			n.Urgency = urgencyFailure
			switch {
			case j.State == JobFailed:
				n.Detail = j.Error
			case j.State == JobStopped && j.Error != byOwner:
				n.Detail = j.Error
			default:
				i := slices.IndexFunc(j.Parts, func(p JobPart) bool { return p.State == PartFailed })
				if i < 0 {
					continue
				}
				n.Detail = j.Parts[i].Title + ": " + j.Parts[i].Error
			}
			n.Detail = clip(n.Detail, 200)
			out = append(out, n)
		}
		return out
	}
}

func (a *App) readyNeeds(ctx context.Context) []need {
	ready, _ := a.myExplorations(ctx, store.ExplorationReady)
	out := []need{}
	for _, e := range ready {
		out = append(out, need{ID: e.ID, Title: clip(e.Request, 160), Created: e.UpdatedAt, Urgency: urgencyWhenFree,
			Link: "/explorations/" + e.ID, Actions: []string{"open"}})
	}
	return out
}

// suggestionNeeds are the owner's: suggestions are made from the owner's
// own mail and routines.
func (a *App) suggestionNeeds(ctx context.Context) []need {
	out := []need{}
	for _, s := range a.suggestions(ctx) {
		out = append(out, need{ID: s.ID, Title: s.Title, Detail: s.Why, Proposal: s.Request, Created: s.Made, Expires: s.Expires,
			Urgency: urgencyWhenFree, Actions: []string{"accept", "dismiss"}})
	}
	return out
}

// systemNeeds are the parts of the house that stopped talking to the
// outside, for the owner who administers them.
func (a *App) systemNeeds(ctx context.Context) []need {
	out := []need{}
	for _, c := range a.components(ctx) {
		if c.State == "error" {
			out = append(out, need{ID: c.ID, Title: c.Name, Detail: c.Detail, Urgency: urgencyFailure, Link: "/settings"})
		}
	}
	return out
}

// credentialNeeds are the keys the caller was asked for privately: a
// routine or task waits on each, so they rank with approvals.
func (a *App) credentialNeeds(ctx context.Context) []need {
	out := []need{}
	for _, c := range a.myCredentialRequests(ctx) {
		out = append(out, need{ID: c.ID, Title: c.Description, Created: c.Asked, Urgency: urgencyDecide,
			Link: "/credentials/" + c.ID, Actions: []string{"open"}})
	}
	return out
}

// lessonNeeds is one item for the caller's lessons waiting for review,
// with how many there are; they are looked at when there is time.
func (a *App) lessonNeeds(ctx context.Context) []need {
	proposed := a.proposedLessons(ctx)
	if len(proposed) == 0 {
		return []need{}
	}
	return []need{{ID: "lessons", Title: proposed[0].Title, Created: proposed[0].Created, Count: len(proposed),
		Urgency: urgencyWhenFree, Link: "/lessons", Actions: []string{"open"}}}
}

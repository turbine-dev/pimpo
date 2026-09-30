// Package usage reports how routines did in real use over some days: how
// often each ran and failed, which scheduled runs never happened without
// anyone being told (silent failures), what was repaired, what waited for
// approval and what it cost. It is what validating Pimpo on real accounts
// is measured with (docs/ROADMAP.md, item 1.3).
package usage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/store"
)

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// A run counts for a scheduled time when it starts up to this long after
// it (the scheduler catches up late runs) or a little before.
const (
	lateOK  = 15 * time.Minute
	earlyOK = 2 * time.Minute
)

type Report struct {
	Since       time.Time   `json:"since"`
	Until       time.Time   `json:"until"`
	Routines    []Routine   `json:"routines"`
	Totals      Totals      `json:"totals"`
	Suggestions Suggestions `json:"suggestions"`
}

// Suggestions counts the routines Pimpo offered on its own and what the
// owner did with them.
type Suggestions struct {
	Made     int `json:"made"`
	Accepted int `json:"accepted"`
	Declined int `json:"declined"`
	Ignored  int `json:"ignored"`
}

type Totals struct {
	Runs      int     `json:"runs"`
	Failed    int     `json:"failed"`
	Silent    int     `json:"silent"`
	WhileOff  int     `json:"while_off"`
	Repaired  int     `json:"repaired"`
	Approvals int     `json:"approvals"`
	CostUSD   float64 `json:"cost_usd"`
	// Success is the share of runs that finished, from 0 to 1.
	Success float64 `json:"success"`
}

type Routine struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	State    string         `json:"state"`
	Runs     int            `json:"runs"`
	OK       int            `json:"ok"`
	Failed   int            `json:"failed"`
	Triggers map[string]int `json:"triggers"`
	Expected int            `json:"expected"`
	Silent   []time.Time    `json:"silent"`
	// WhileOff counts scheduled times that fell while Pimpo was not
	// running: missed, but not Pimpo failing.
	WhileOff  int       `json:"while_off"`
	Late      int       `json:"late"`
	Failures  []Failure `json:"failures"`
	Repaired  int       `json:"repaired"`
	Approvals Approvals `json:"approvals"`
	CostUSD   float64   `json:"cost_usd"`
	LastRun   time.Time `json:"last_run,omitzero"`
}

type Failure struct {
	At    time.Time `json:"at"`
	Error string    `json:"error"`
	// Fixed says a later run of the routine finished.
	Fixed bool `json:"fixed"`
}

type Approvals struct {
	Asked    int `json:"asked"`
	Approved int `json:"approved"`
	Denied   int `json:"denied"`
	Expired  int `json:"expired"`
}

// Build reads the log and the store for the days before now, for the
// owner's own routines.
func Build(ctx context.Context, ev *event.Store, st *store.Store, now time.Time, days int, zone *time.Location) (Report, error) {
	return BuildFor(ctx, ev, st, now, days, zone, people.OwnerID)
}

// BuildFor reports only on the person's own routines: nobody, the owner
// included, learns how anyone else's routines did.
func BuildFor(ctx context.Context, ev *event.Store, st *store.Store, now time.Time, days int, zone *time.Location, person string) (Report, error) {
	if days <= 0 {
		days = 21
	}
	if zone == nil {
		zone = time.Local
	}
	since := now.Add(-time.Duration(days) * 24 * time.Hour)
	rep := Report{Since: since, Until: now, Routines: []Routine{}}
	all, err := st.Routines(ctx)
	if err != nil {
		return rep, err
	}
	var routines []store.Routine
	for _, r := range all {
		if people.Norm(r.Person) == people.Norm(person) {
			routines = append(routines, r)
		}
	}
	events, err := ev.List(ctx, event.Query{Types: []string{"routine.run.started", "routine.run.finished", "routine.run.failed", "routine.run.missed",
		"routine.paused", "routine.resumed", "routine.created", "approval.requested", "approval.resolved", "system.started", "system.gap"}, Newest: true, Limit: 50000})
	if err != nil {
		return rep, err
	}
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	if sugg, err := ev.List(ctx, event.Query{Types: []string{"suggestion.made", "suggestion.accepted", "suggestion.dismissed", "suggestion.expired"}, Newest: true, Limit: 5000}); err == nil {
		for _, e := range sugg {
			if e.Time.Before(since) {
				continue
			}
			switch e.Type {
			case "suggestion.made":
				rep.Suggestions.Made++
			case "suggestion.accepted":
				rep.Suggestions.Accepted++
			case "suggestion.dismissed":
				rep.Suggestions.Declined++
			case "suggestion.expired":
				rep.Suggestions.Ignored++
			}
		}
	}

	down := offline(events)
	byRoutine := map[string][]event.Event{}
	approvalOf := map[string]string{}
	for _, e := range events {
		var d struct {
			Routine string `json:"routine"`
			ID      string `json:"id"`
		}
		e.Decode(&d)
		id := d.Routine
		if e.Type == "approval.requested" {
			id = routineOf(e.Actor)
			approvalOf[d.ID] = id
		}
		if e.Type == "approval.resolved" {
			id = approvalOf[d.ID]
		}
		if id != "" {
			byRoutine[id] = append(byRoutine[id], e)
		}
	}

	for _, r := range routines {
		u := Routine{ID: r.ID, Name: r.Body.Name, State: r.State, Triggers: map[string]int{}, Silent: []time.Time{}, Failures: []Failure{}}
		runs, _ := st.Runs(ctx, r.ID, 5000)
		for _, run := range runs {
			if run.StartedAt.Before(since) {
				continue
			}
			u.Runs++
			u.CostUSD += run.CostUSD
			switch run.Outcome {
			case store.RunOK:
				u.OK++
			case store.RunFailed:
				u.Failed++
			}
			if run.StartedAt.After(u.LastRun) {
				u.LastRun = run.StartedAt
			}
		}
		starts := []time.Time{}
		active := activeSpans(byRoutine[r.ID], r, now)
		for _, e := range byRoutine[r.ID] {
			if e.Time.Before(since) {
				continue
			}
			switch e.Type {
			case "routine.run.started":
				var d struct {
					Trigger string `json:"trigger"`
				}
				e.Decode(&d)
				u.Triggers[firstNonEmpty(d.Trigger, "schedule")]++
				starts = append(starts, e.Time)
			case "routine.run.missed":
				// The scheduler said so, late: not silent. It names the
				// time the run was due.
				var d struct {
					Due time.Time `json:"due"`
				}
				e.Decode(&d)
				u.Late++
				starts = append(starts, firstTime(d.Due, e.Time))
			case "routine.run.failed":
				var d struct {
					Error string `json:"error"`
				}
				e.Decode(&d)
				if len(u.Failures) < 10 {
					u.Failures = append(u.Failures, Failure{At: e.Time, Error: short(d.Error)})
				}
			case "routine.created":
				var d struct {
					Repair bool `json:"repair"`
				}
				if e.Decode(&d); d.Repair {
					u.Repaired++
				}
			case "approval.requested":
				u.Approvals.Asked++
			case "approval.resolved":
				var d struct {
					Answer string `json:"answer"`
				}
				e.Decode(&d)
				switch d.Answer {
				case "deny":
					u.Approvals.Denied++
				case "expired":
					u.Approvals.Expired++
				default:
					u.Approvals.Approved++
				}
			}
		}
		for i := range u.Failures {
			for _, run := range runs {
				if run.Outcome == store.RunOK && run.StartedAt.After(u.Failures[i].At) {
					u.Failures[i].Fixed = true
					break
				}
			}
		}
		if sched, err := parser.Parse(r.Schedule()); err == nil && r.Schedule() != "" {
			from := since
			if r.CreatedAt.After(from) {
				from = r.CreatedAt
			}
			for t := sched.Next(from.In(zone)); t.Before(now.Add(-lateOK)); t = sched.Next(t) {
				if !within(active, t) {
					continue
				}
				if within(down, t) {
					u.WhileOff++
					continue
				}
				u.Expected++
				switch {
				case ranNear(starts, t, t.Add(lateOK)):
				case ranNear(starts, t, sched.Next(t)):
					// It ran, late, before the next time: a computer that
					// woke up runs what was due while it slept.
					u.Late++
				default:
					if len(u.Silent) < 1000 {
						u.Silent = append(u.Silent, t)
					}
				}
			}
		}
		rep.Totals.Runs += u.Runs
		rep.Totals.Failed += u.Failed
		rep.Totals.Silent += len(u.Silent)
		rep.Totals.WhileOff += u.WhileOff
		rep.Totals.Repaired += u.Repaired
		rep.Totals.Approvals += u.Approvals.Asked
		rep.Totals.CostUSD += u.CostUSD
		if u.Runs > 0 || u.Expected > 0 || r.State == store.RoutineActive {
			rep.Routines = append(rep.Routines, u)
		}
	}
	if rep.Totals.Runs > 0 {
		rep.Totals.Success = float64(rep.Totals.Runs-rep.Totals.Failed) / float64(rep.Totals.Runs)
	}
	sort.Slice(rep.Routines, func(i, j int) bool {
		a, b := rep.Routines[i], rep.Routines[j]
		if len(a.Silent)+a.Failed != len(b.Silent)+b.Failed {
			return len(a.Silent)+a.Failed > len(b.Silent)+b.Failed
		}
		return a.Name < b.Name
	})
	return rep, nil
}

type span struct{ from, to time.Time }

// activeSpans are the stretches in which the routine was expected to run
// on its schedule: not paused, and not stopped after a failure. Paused and
// stopped are kept apart: running a paused routine by hand does not make
// it active.
func activeSpans(events []event.Event, r store.Routine, now time.Time) []span {
	var trans []event.Event
	for _, e := range events {
		switch e.Type {
		case "routine.paused", "routine.resumed", "routine.run.failed", "routine.run.finished", "routine.created":
			trans = append(trans, e)
		}
	}
	// Before the first change, the routine was paused if the first pause
	// or resume is a resume, not after a failure; with no change at all, it is as it is now
	// (routines can arrive paused, as imported ones do).
	paused := r.State == store.RoutinePaused
	broken := r.State == store.RoutineBroken && len(trans) == 0
	for _, e := range trans {
		if e.Type == "routine.run.failed" {
			break // a later resume only undoes the failure
		}
		if e.Type == "routine.paused" || e.Type == "routine.resumed" {
			paused = e.Type == "routine.resumed"
			break
		}
	}
	var out []span
	cur := r.CreatedAt
	on := !paused && !broken
	for _, e := range trans {
		switch e.Type {
		case "routine.paused":
			paused = true
		case "routine.resumed":
			paused, broken = false, false
		case "routine.run.failed":
			broken = true
		case "routine.run.finished", "routine.created":
			broken = false
		}
		next := !paused && !broken
		if on && !next {
			out = append(out, span{cur, e.Time})
		}
		if !on && next {
			cur = e.Time
		}
		on = next
	}
	if on && r.State == store.RoutineActive {
		out = append(out, span{cur, now})
	}
	return out
}

// offline are the stretches Pimpo was not running: from the last
// heartbeat before a start, and gaps such as the computer asleep.
func offline(events []event.Event) []span {
	var out []span
	for _, e := range events {
		var d struct {
			LastAlive string `json:"last_alive"`
			From      string `json:"from"`
			To        string `json:"to"`
		}
		e.Decode(&d)
		switch e.Type {
		case "system.started":
			if t, err := time.Parse(time.RFC3339, d.LastAlive); err == nil {
				out = append(out, span{t, e.Time})
			}
		case "system.gap":
			from, err1 := time.Parse(time.RFC3339, d.From)
			to, err2 := time.Parse(time.RFC3339, d.To)
			if err1 == nil && err2 == nil {
				out = append(out, span{from, to})
			}
		}
	}
	return out
}

func within(spans []span, t time.Time) bool {
	for _, s := range spans {
		if !t.Before(s.from) && t.Before(s.to) {
			return true
		}
	}
	return false
}

// ranNear says a run started from a little before t up to until.
func ranNear(starts []time.Time, t, until time.Time) bool {
	for _, s := range starts {
		if !s.Before(t.Add(-earlyOK)) && s.Before(until) {
			return true
		}
	}
	return false
}

func routineOf(source string) string {
	id, ok := strings.CutPrefix(source, "routine:")
	if !ok {
		return ""
	}
	id, _, _ = strings.Cut(id, "#")
	return id
}

func short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 200 {
		s = string(r[:199]) + "…"
	}
	return s
}

func firstTime(ts ...time.Time) time.Time {
	for _, t := range ts {
		if !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// Markdown renders the report for a person or an issue.
func (r Report) Markdown(zone *time.Location) string {
	if zone == nil {
		zone = time.Local
	}
	var b strings.Builder
	day := func(t time.Time) string { return t.In(zone).Format("2006-01-02") }
	at := func(t time.Time) string { return t.In(zone).Format("2006-01-02 15:04") }
	fmt.Fprintf(&b, "# Pimpo in real use, %s to %s\n\n", day(r.Since), day(r.Until))
	t := r.Totals
	fmt.Fprintf(&b, "**%d runs, %.1f%% finished. %d failed, %d silent failures, %d repaired, %d approvals asked, $%.2f.**\n\n", t.Runs, 100*t.Success, t.Failed, t.Silent, t.Repaired, t.Approvals, t.CostUSD)
	b.WriteString("A silent failure is a scheduled run that did not happen and was not reported while Pimpo was running: the thing Pimpo must never do.")
	if t.WhileOff > 0 {
		fmt.Fprintf(&b, " Another %d scheduled times fell while Pimpo was not running (stopped, or the computer asleep).", t.WhileOff)
	}
	b.WriteString("\n\n")
	if s := r.Suggestions; s.Made > 0 {
		fmt.Fprintf(&b, "Pimpo suggested %d routines on its own: %d accepted, %d declined, %d left unanswered.\n\n", s.Made, s.Accepted, s.Declined, s.Ignored)
	}
	b.WriteString("| Routine | State | Runs | Finished | Failed | Silent | Late | Repaired | Approvals | Cost |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, u := range r.Routines {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d of %d | %d | %d | %d asked, %d denied, %d expired | $%.2f |\n",
			u.Name, u.State, u.Runs, u.OK, u.Failed, len(u.Silent), u.Expected, u.Late, u.Repaired, u.Approvals.Asked, u.Approvals.Denied, u.Approvals.Expired, u.CostUSD)
	}
	for _, u := range r.Routines {
		if len(u.Silent) == 0 && len(u.Failures) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", u.Name)
		for i, s := range u.Silent {
			if i == 10 {
				fmt.Fprintf(&b, "- … and %d more silent\n", len(u.Silent)-10)
				break
			}
			fmt.Fprintf(&b, "- **silent:** due %s, no run and no notice\n", at(s))
		}
		for _, f := range u.Failures {
			fixed := "not yet fixed"
			if f.Fixed {
				fixed = "a later run finished"
			}
			fmt.Fprintf(&b, "- **failed** %s: %s (%s)\n", at(f.At), f.Error, fixed)
		}
	}
	return b.String()
}

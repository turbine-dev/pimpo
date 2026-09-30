// Package scheduler runs routines on their schedules, records every run,
// and makes sure no failure goes unnoticed.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/pause"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/store"
)

const (
	EventRunStarted  = "routine.run.started"
	EventRunFinished = "routine.run.finished"
	EventRunFailed   = "routine.run.failed"
	EventMissed      = "routine.run.missed"
)

type Scheduler struct {
	Env    host.Env
	Store  *store.Store
	Notify explore.Notifier
	Zone   *time.Location
	// Now is replaceable in tests.
	Now func() time.Time
	// Progress hears how each run goes: when it starts, each step, and
	// how it ended. It is kept so a reload or a restart still shows it.
	Progress func(ctx context.Context, p RunProgress)

	mu      sync.Mutex
	cron    *cron.Cron
	entries map[string]cron.EntryID
	running map[string]bool
	polled  map[string]time.Time
	wg      sync.WaitGroup
}

// RunProgress is where a run is: its state, how many steps it took and
// the step it is on, by Pimpo's own names (a capability, a judgment).
type RunProgress struct {
	Routine string
	Name    string
	Person  string
	Run     int64
	State   string
	Steps   int
	Step    string
	CostUSD float64
	Started time.Time
	Error   string
}

func (s *Scheduler) progress(ctx context.Context, p RunProgress) {
	if s.Progress != nil {
		s.Progress(context.WithoutCancel(ctx), p)
	}
}

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

func (s *Scheduler) zone() *time.Location {
	if s.Zone != nil {
		return s.Zone
	}
	return time.Local
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Start schedules every active routine and runs the ones whose last
// scheduled time passed while Pimpo was off.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	s.cron = cron.New(cron.WithLocation(s.zone()), cron.WithParser(parser))
	s.entries = map[string]cron.EntryID{}
	s.running = map[string]bool{}
	s.mu.Unlock()
	routines, err := s.Store.Routines(ctx)
	if err != nil {
		return err
	}
	for _, r := range routines {
		s.Changed(ctx, r.ID)
		s.catchUp(ctx, r)
	}
	s.cron.Start()
	go s.watchLoop(ctx, time.Minute)
	go func() {
		<-ctx.Done()
		<-s.cron.Stop().Done()
	}()
	return nil
}

// Changed reschedules one routine after it was created, repaired, paused
// or resumed.
func (s *Scheduler) Changed(ctx context.Context, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return
	}
	if old, ok := s.entries[id]; ok {
		s.cron.Remove(old)
		delete(s.entries, id)
	}
	r, err := s.Store.Routine(ctx, id)
	if err != nil || r.State != store.RoutineActive {
		return
	}
	// Without a schedule a routine starts only by its watch, its webhook,
	// an answer or by hand.
	if r.Schedule() == "" {
		return
	}
	sched, err := parser.Parse(r.Schedule())
	if err != nil {
		s.Env.Events.Append(ctx, EventRunFailed, "routine:"+id, map[string]string{"routine": id, "error": "invalid schedule: " + err.Error()})
		return
	}
	s.entries[id] = s.cron.Schedule(sched, cron.FuncJob(func() { s.RunNow(context.WithoutCancel(ctx), id, "schedule") }))
}

// Next is the next scheduled time of a routine, or zero.
func (s *Scheduler) Next(id string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return time.Time{}
	}
	if e, ok := s.entries[id]; ok {
		return s.cron.Entry(e).Next
	}
	return time.Time{}
}

func (s *Scheduler) catchUp(ctx context.Context, r store.Routine) {
	if r.State != store.RoutineActive {
		return
	}
	sched, err := parser.Parse(r.Schedule())
	if err != nil {
		return
	}
	runs, _ := s.Store.Runs(ctx, r.ID, 1)
	last := r.CreatedAt
	if len(runs) > 0 {
		last = runs[0].StartedAt
	}
	due := sched.Next(last.In(s.zone()))
	now := s.now()
	// Catch up once for a run missed in the last 12 hours; older misses are
	// reported, not replayed, so a laptop that slept for a week does not
	// send seven morning briefs at once.
	if due.Before(now) {
		if now.Sub(due) < 12*time.Hour {
			s.Env.Events.Append(ctx, EventMissed, "routine:"+r.ID, map[string]any{"routine": r.ID, "due": due, "action": "ran late"})
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.RunNow(context.WithoutCancel(ctx), r.ID, "catch-up")
			}()
		} else {
			s.Env.Events.Append(ctx, EventMissed, "routine:"+r.ID, map[string]any{"routine": r.ID, "due": due, "action": "skipped"})
		}
	}
}

func stateKey(id string) string { return "routine.state." + id }

// State is what a routine kept from its earlier runs.
func (s *Scheduler) State(ctx context.Context, id string) map[string]any {
	out := map[string]any{}
	if raw, _ := s.Env.Events.Get(ctx, stateKey(id)); raw != "" {
		json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// SaveState keeps a routine's state; an empty one is forgotten.
func (s *Scheduler) SaveState(ctx context.Context, id string, state map[string]any) error {
	if len(state) == 0 {
		return s.Env.Events.Put(ctx, stateKey(id), "")
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.Env.Events.Put(ctx, stateKey(id), string(b))
}

// Library finds a routine another one uses, with its settings and state.
func (s *Scheduler) Library(ctx context.Context, id string) (runtime.Helper, error) {
	r, err := s.Store.Routine(ctx, id)
	if err != nil {
		return runtime.Helper{}, fmt.Errorf("routine %s is not installed", id)
	}
	if r.State == store.RoutineBroken {
		return runtime.Helper{}, fmt.Errorf("routine %s is stopped after a failure", id)
	}
	return runtime.Helper{Code: r.Body.Code, Manifest: r.Body.Manifest, Params: r.Settings.Params, State: s.State(ctx, id)}, nil
}

// Wait blocks until catch-up runs finish.
func (s *Scheduler) Wait() { s.wg.Wait() }

// RunWith runs a routine once with an event, such as what a webhook
// brought.
func (s *Scheduler) RunWith(ctx context.Context, id, trigger string, event any) (store.Run, error) {
	return s.run(ctx, id, trigger, event)
}

// RunNow runs a routine once. A routine never runs twice at the same time.
func (s *Scheduler) RunNow(ctx context.Context, id, trigger string) (store.Run, error) {
	return s.run(ctx, id, trigger, nil)
}

func (s *Scheduler) run(ctx context.Context, id, trigger string, event any) (store.Run, error) {
	s.mu.Lock()
	if s.running == nil {
		s.running = map[string]bool{}
	}
	if s.running[id] {
		s.mu.Unlock()
		return store.Run{}, fmt.Errorf("%s is already running", id)
	}
	s.running[id] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
	}()

	r, err := s.Store.Routine(ctx, id)
	if err != nil {
		return store.Run{}, err
	}
	runID, err := s.Store.StartRun(ctx, id, r.Version)
	if err != nil {
		return store.Run{}, err
	}
	source := fmt.Sprintf("routine:%s#%d", id, runID)
	s.Env.Events.Append(ctx, EventRunStarted, source, map[string]any{"routine": id, "run": runID, "version": r.Version, "trigger": trigger})
	h := &host.Host{Env: s.Env, Source: source, Person: r.Person}
	prog := RunProgress{Routine: id, Name: r.Body.Name, Person: r.Person, Run: runID, State: store.RunRunning, Started: time.Now().UTC()}
	s.progress(ctx, prog)
	var stepMu sync.Mutex
	h.OnStep = func(label string) {
		stepMu.Lock()
		prog.Steps++
		prog.Step = label
		p := prog
		stepMu.Unlock()
		p.CostUSD = h.Cost()
		s.progress(ctx, p)
	}
	if params, err := r.Body.Manifest.ResolveParams(r.Settings.Params); err == nil {
		h.Destinations = r.Body.Manifest.Destinations(params)
	}
	// The routine's own code has 90 seconds; waiting on services and
	// models, as a judgment per item does, may take longer, up to 15
	// minutes in all.
	// Waiting for the owner's approval does not count.
	ctx, cancel := pause.WithTimeout(host.WithEffort(host.WithModel(ctx, r.Settings.Model), r.Settings.Effort), 15*time.Minute)
	defer cancel()
	res, runErr := runtime.Run(ctx, r.Body.Code, r.Body.Manifest, h, runtime.Options{Now: s.now(), Zone: s.zone(), Timeout: 90 * time.Second,
		Params: r.Settings.Params, Event: event, State: s.State(ctx, id), Library: s.Library, ID: id})
	if runErr == nil && res.Changed {
		if err := s.SaveState(context.WithoutCancel(ctx), id, res.State); err != nil {
			runErr = fmt.Errorf("could not keep the routine's state: %w", err)
		}
	}
	outcome, errText := store.RunOK, ""
	if runErr != nil {
		outcome, errText = store.RunFailed, runErr.Error()
	}
	s.Store.FinishRun(context.WithoutCancel(ctx), runID, outcome, errText, h.Cost(), res.Calls)
	stepMu.Lock()
	prog.State, prog.Error, prog.CostUSD, prog.Step = outcome, errText, h.Cost(), ""
	final := prog
	stepMu.Unlock()
	s.progress(ctx, final)
	run := store.Run{ID: runID, Routine: id, Version: r.Version, Outcome: outcome, Error: errText, CostUSD: h.Cost(), Calls: res.Calls}
	if runErr == nil {
		s.Env.Events.Append(ctx, EventRunFinished, source, map[string]any{"routine": id, "run": runID, "calls": res.Calls, "cost_usd": h.Cost(), "logs": res.Logs})
		return run, nil
	}
	s.Env.Events.Append(ctx, EventRunFailed, source, map[string]any{"routine": id, "run": runID, "error": errText, "calls": res.Calls})
	s.Store.SetRoutineState(context.WithoutCancel(ctx), id, store.RoutineBroken)
	s.Changed(ctx, id)
	s.Notify.Notify(context.WithoutCancel(ctx), explore.Notice{
		Text:    i18n.T(ctx, "msg.routine.failed", "name", r.Body.Name, "error", friendly(ctx, errText)),
		Actions: []explore.Action{{Label: i18n.T(ctx, "btn.runAgain"), Data: "run:" + id}, {Label: i18n.T(ctx, "btn.redo"), Data: "repair:" + id}},
		Kind:    "failure",
	})
	return run, runErr
}

func friendly(ctx context.Context, err string) string {
	if strings.Contains(err, "budget") {
		return i18n.T(ctx, "msg.budgetOver")
	}
	if len(err) > 400 {
		err = err[:400] + "…"
	}
	return err
}

// Package scheduler runs routines on their schedules, records every run,
// and makes sure no failure goes unnoticed.
package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/denerFernandes/zodim/internal/explore"
	"github.com/denerFernandes/zodim/internal/host"
	"github.com/denerFernandes/zodim/internal/runtime"
	"github.com/denerFernandes/zodim/internal/store"
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

	mu      sync.Mutex
	cron    *cron.Cron
	entries map[string]cron.EntryID
	running map[string]bool
	wg      sync.WaitGroup
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
// scheduled time passed while Zodim was off.
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

// Wait blocks until catch-up runs finish.
func (s *Scheduler) Wait() { s.wg.Wait() }

// RunNow runs a routine once. A routine never runs twice at the same time.
func (s *Scheduler) RunNow(ctx context.Context, id, trigger string) (store.Run, error) {
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
	if params, err := r.Body.Manifest.ResolveParams(r.Settings.Params); err == nil {
		h.Destinations = r.Body.Manifest.Destinations(params)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	res, runErr := runtime.Run(ctx, r.Body.Code, r.Body.Manifest, h, runtime.Options{Now: s.now(), Zone: s.zone(), Timeout: 90 * time.Second, Params: r.Settings.Params})
	outcome, errText := store.RunOK, ""
	if runErr != nil {
		outcome, errText = store.RunFailed, runErr.Error()
	}
	s.Store.FinishRun(context.WithoutCancel(ctx), runID, outcome, errText, h.Cost(), res.Calls)
	run := store.Run{ID: runID, Routine: id, Version: r.Version, Outcome: outcome, Error: errText, CostUSD: h.Cost(), Calls: res.Calls}
	if runErr == nil {
		s.Env.Events.Append(ctx, EventRunFinished, source, map[string]any{"routine": id, "run": runID, "calls": res.Calls, "cost_usd": h.Cost(), "logs": res.Logs})
		return run, nil
	}
	s.Env.Events.Append(ctx, EventRunFailed, source, map[string]any{"routine": id, "run": runID, "error": errText, "calls": res.Calls})
	s.Store.SetRoutineState(context.WithoutCancel(ctx), id, store.RoutineBroken)
	s.Changed(ctx, id)
	s.Notify.Notify(context.WithoutCancel(ctx), explore.Notice{
		Text:    fmt.Sprintf("⚠️ %s não rodou.\n%s\n\nPausei a rotina até você decidir.", r.Body.Name, friendly(errText)),
		Actions: []explore.Action{{Label: "Rodar de novo", Data: "run:" + id}, {Label: "Refazer com o agente", Data: "repair:" + id}},
		Kind:    "failure",
	})
	return run, runErr
}

func friendly(err string) string {
	if strings.Contains(err, "budget") {
		return "O limite de gasto do dia acabou."
	}
	if len(err) > 400 {
		err = err[:400] + "…"
	}
	return err
}

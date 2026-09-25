package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/denerFernandes/zodim/internal/host"
	"github.com/denerFernandes/zodim/internal/store"
)

const (
	EventWatchFound  = "routine.watch.found"
	EventWatchFailed = "routine.watch.failed"
	seenLimit        = 2000
)

func seenKey(id string) string { return "watch.seen." + id }

// watchLoop polls every watching routine when its interval has passed.
func (s *Scheduler) watchLoop(ctx context.Context, tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		s.pollDue(ctx)
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) pollDue(ctx context.Context) {
	routines, err := s.Store.Routines(ctx)
	if err != nil {
		return
	}
	now := s.now()
	for _, r := range routines {
		w := r.Watch()
		if w == nil || r.State != store.RoutineActive {
			continue
		}
		s.mu.Lock()
		if s.polled == nil {
			s.polled = map[string]time.Time{}
		}
		due := now.Sub(s.polled[r.ID]) >= w.Interval()
		if due {
			s.polled[r.ID] = now
		}
		s.mu.Unlock()
		if due {
			s.Poll(ctx, r.ID)
		}
	}
}

// Poll checks one watching routine once and runs it with the items it has
// not seen. The first poll only learns what is already there.
func (s *Scheduler) Poll(ctx context.Context, id string) (int, error) {
	r, err := s.Store.Routine(ctx, id)
	if err != nil {
		return 0, err
	}
	w := r.Watch()
	if w == nil {
		return 0, fmt.Errorf("%s watches nothing", id)
	}
	params, err := r.Body.Manifest.ResolveParams(r.Settings.Params)
	if err != nil {
		return 0, err
	}
	name, scope, _ := strings.Cut(w.Capability, ":")
	h := &host.Host{Env: s.Env, Source: "watch:" + id, Person: r.Person, QuietReads: true}
	out, err := h.Call(ctx, name, scope, w.ArgsWith(params))
	if err != nil {
		s.Env.Events.Append(ctx, EventWatchFailed, "routine:"+id, map[string]string{"routine": id, "error": err.Error()})
		return 0, err
	}
	raw, _ := json.Marshal(out)
	var items []map[string]any
	if json.Unmarshal(raw, &items) != nil {
		err := fmt.Errorf("%s did not return a list", name)
		s.Env.Events.Append(ctx, EventWatchFailed, "routine:"+id, map[string]string{"routine": id, "error": err.Error()})
		return 0, err
	}
	stored, _ := s.Env.Events.Get(ctx, seenKey(id))
	var seen []string
	first := stored == ""
	json.Unmarshal([]byte(stored), &seen)
	known := map[string]bool{}
	for _, k := range seen {
		known[k] = true
	}
	var fresh []any
	for _, it := range items {
		k := fmt.Sprint(it[w.Key])
		if it[w.Key] == nil || known[k] {
			continue
		}
		known[k] = true
		seen = append(seen, k)
		fresh = append(fresh, it)
	}
	if len(seen) > seenLimit {
		seen = seen[len(seen)-seenLimit:]
	}
	b, _ := json.Marshal(seen)
	if seen == nil {
		b = []byte("[]")
	}
	s.Env.Events.Put(ctx, seenKey(id), string(b))
	if first || len(fresh) == 0 {
		return 0, nil
	}
	s.Env.Events.Append(ctx, EventWatchFound, "routine:"+id, map[string]any{"routine": id, "items": len(fresh)})
	_, err = s.run(ctx, id, "event", map[string]any{"items": fresh})
	return len(fresh), err
}

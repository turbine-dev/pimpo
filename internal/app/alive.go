package app

import (
	"context"
	"time"
)

// Pimpo records when it was not running (stopped, or the computer
// asleep), so a routine that did not run then is told apart from one that
// failed silently while Pimpo was up. A heartbeat is kept as a value, not
// logged; only starts and gaps become events.

const aliveKey = "system.alive"

// aliveGap is how long without a heartbeat counts as not running.
const aliveGap = 3 * time.Minute

func (a *App) aliveLoop(ctx context.Context, every time.Duration) {
	now := clock()
	last, _ := a.Events.Get(ctx, aliveKey)
	a.Events.Append(ctx, "system.started", "system", map[string]any{"last_alive": last, "version": a.Version})
	a.Events.Put(ctx, aliveKey, now.UTC().Format(time.RFC3339))
	t := time.NewTicker(every)
	defer t.Stop()
	prev := now
	for {
		select {
		case <-ctx.Done():
			a.Events.Append(context.WithoutCancel(ctx), "system.stopped", "system", map[string]any{})
			return
		case <-t.C:
			now := clock()
			if now.Sub(prev) > aliveGap {
				a.Events.Append(ctx, "system.gap", "system", map[string]any{"from": prev.UTC().Format(time.RFC3339), "to": now.UTC().Format(time.RFC3339)})
			}
			prev = now
			a.Events.Put(ctx, aliveKey, now.UTC().Format(time.RFC3339))
		}
	}
}

// clock is replaced in tests.
var clock = time.Now

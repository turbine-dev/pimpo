// Package pause gives a context a time limit that stops counting while
// the work waits on a person: a routine run has 15 minutes of its own,
// however long the owner takes to answer an approval.
package pause

import (
	"context"
	"sync"
	"time"
)

type clock struct {
	mu      sync.Mutex
	cancel  context.CancelCauseFunc
	left    time.Duration
	started time.Time
	timer   *time.Timer
	paused  int
}

type key struct{}

// WithTimeout is context.WithTimeout whose clock Pause can stop.
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	c := &clock{cancel: cancel, left: d, started: time.Now()}
	c.timer = time.AfterFunc(d, func() { cancel(context.DeadlineExceeded) })
	ctx = timed{context.WithValue(ctx, key{}, c)}
	return ctx, func() {
		c.mu.Lock()
		c.timer.Stop()
		c.mu.Unlock()
		cancel(context.Canceled)
	}
}

// Pause stops the clock of ctx, if it has one, until the returned func is
// called. Pauses nest.
func Pause(ctx context.Context) (resume func()) {
	c, _ := ctx.Value(key{}).(*clock)
	if c == nil {
		return func() {}
	}
	c.mu.Lock()
	if c.paused == 0 && c.timer.Stop() {
		c.left -= time.Since(c.started)
	}
	c.paused++
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.paused--
			if c.paused == 0 && ctx.Err() == nil {
				c.started = time.Now()
				c.timer.Reset(max(c.left, 0))
			}
		})
	}
}

// timed reports running out of time as context.DeadlineExceeded, like
// context.WithTimeout, to its children too.
type timed struct{ context.Context }

func (t timed) Err() error {
	if err := t.Context.Err(); err != nil {
		if context.Cause(t.Context) == context.DeadlineExceeded {
			return context.DeadlineExceeded
		}
		return err
	}
	return nil
}

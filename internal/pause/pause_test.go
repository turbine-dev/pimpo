package pause

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClockStopsWhilePaused(t *testing.T) {
	ctx, cancel := WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	time.Sleep(40 * time.Millisecond)
	resume := Pause(ctx)
	inner := Pause(ctx) // nested
	time.Sleep(150 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("expired while paused")
	}
	inner()
	time.Sleep(20 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("an inner resume restarted the clock")
	}
	resume()
	resume() // idempotent
	time.Sleep(20 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("expired too early: the time before the pause was counted twice")
	}
	select {
	case <-ctx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("never expired")
	}
	child, stop := context.WithCancel(ctx)
	defer stop()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || !errors.Is(child.Err(), context.DeadlineExceeded) {
		t.Fatalf("err %v, child %v", ctx.Err(), child.Err())
	}
}

func TestWithoutAClockPauseIsANoOp(t *testing.T) {
	Pause(context.Background())()
}

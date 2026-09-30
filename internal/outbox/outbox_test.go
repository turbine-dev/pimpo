package outbox

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
)

func TestQueueCancelAndFlush(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	now := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	var sent []any
	o := &Outbox{DB: ev.DB(), Events: ev, Now: func() time.Time { return now }, Send: func(_ context.Context, a any) (any, error) {
		sent = append(sent, a)
		return nil, nil
	}}
	if err := o.Init(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r1, _ := o.Call(ctx, "outbox.send_later", "", map[string]any{"to": "a@x.com", "subject": "1"})
	r2, _ := o.Call(ctx, "outbox.send_later", "", map[string]any{"to": "b@x.com", "subject": "2"})
	id1 := r1.(map[string]any)["outbox_id"].(int64)
	id2 := r2.(map[string]any)["outbox_id"].(int64)
	if n, _ := o.Flush(ctx); n != 0 {
		t.Fatal("sent before the delay")
	}
	if err := o.Cancel(ctx, id1); err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	if n, _ := o.Flush(ctx); n != 1 || len(sent) != 1 {
		t.Fatalf("flush sent %d", n)
	}
	if err := o.Cancel(ctx, id2); !errors.Is(err, ErrTooLate) {
		t.Fatalf("cancel after send: %v", err)
	}
	it, _ := o.Get(ctx, id1)
	if it.State != Cancelled {
		t.Fatalf("cancelled item %+v", it)
	}
}

// A queued email leaves as the person who queued it, and cancelling a
// person's emails leaves everyone else's.
func TestSendsAsWhoeverQueued(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	now := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	sentFor := map[string]int{}
	o := &Outbox{DB: ev.DB(), Events: ev, Now: func() time.Time { return now }, Send: func(ctx context.Context, a any) (any, error) {
		sentFor[people.From(ctx)]++
		return nil, nil
	}}
	// An outbox from before people: its rows become the owner's.
	ev.DB().Exec(schema)
	ev.DB().Exec(`INSERT INTO outbox (args, send_at, state) VALUES ('{}', ?, ?)`, now.Format(time.RFC3339Nano), Queued)
	if err := o.Init(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	o.Call(people.With(ctx, "ana"), "outbox.send_later", "", map[string]any{"to": "a@x.com"})
	o.Call(people.With(ctx, "bia"), "outbox.send_later", "", map[string]any{"to": "b@x.com"})
	o.Call(ctx, "outbox.send_later", "", map[string]any{"to": "c@x.com"})
	o.CancelFor(ctx, "bia")
	now = now.Add(11 * time.Minute)
	if n, _ := o.Flush(ctx); n != 3 {
		t.Fatalf("sent %d", n)
	}
	if sentFor["ana"] != 1 || sentFor["owner"] != 2 || sentFor["bia"] != 0 {
		t.Fatalf("sent for %v", sentFor)
	}
}

package budget

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
)

func TestLimitIsCheckedBeforeEachCall(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	now := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	ev.SetClock(func() time.Time { return now })
	b := &Budget{Events: ev, Zone: time.UTC, Now: func() time.Time { return now }}
	ctx := context.Background()
	if b.Limit(ctx) != 1 {
		t.Fatalf("default limit %v", b.Limit(ctx))
	}
	b.SetLimit(ctx, 0.5, "human:owner")
	b.Record(ctx, Cost{USD: 0.3, Source: "exploration"})
	if err := b.Check(ctx); err != nil {
		t.Fatalf("under the limit: %v", err)
	}
	b.Record(ctx, Cost{USD: 0.25, Source: "compile"})
	if err := b.Check(ctx); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("over the limit: %v", err)
	}
	if r := b.Remaining(ctx); r != 0 {
		t.Fatalf("remaining %v", r)
	}
	// A new day starts fresh.
	now = now.Add(24 * time.Hour)
	if err := b.Check(ctx); err != nil {
		t.Fatalf("next day: %v", err)
	}
	b.SetLimit(ctx, 0, "human:owner")
	if b.Remaining(ctx) != -1 {
		t.Fatal("zero should mean no limit")
	}
	if err := b.SetLimit(ctx, -1, "x"); err == nil {
		t.Fatal("negative limit accepted")
	}
}

// Every cost belongs to the person it was for, and a person's own limit
// stops only them, inside the house's.
func TestEachPersonHasTheirOwnLimit(t *testing.T) {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	defer ev.Close()
	b := &Budget{Events: ev, Zone: time.UTC, PersonLimit: func(_ context.Context, p string) float64 {
		if p == "ana" {
			return 0.3
		}
		return 0
	}}
	ctx := context.Background()
	ana := people.With(ctx, "ana")
	b.SetLimit(ctx, 1, "human:owner")
	b.Record(ana, Cost{USD: 0.2, Source: "exploration"})
	b.Record(ctx, Cost{USD: 0.1, Source: "exploration"})
	b.Record(ctx, Cost{USD: 0.05, Source: "routine", Person: "ana"})
	if got, _ := b.TodayFor(ctx, "ana"); got < 0.249 || got > 0.251 {
		t.Fatalf("Ana spent %v", got)
	}
	if got, _ := b.TodayFor(ctx, people.OwnerID); got < 0.099 || got > 0.101 {
		t.Fatalf("the owner spent %v", got)
	}
	if r := b.Remaining(ana); r < 0.049 || r > 0.051 {
		t.Fatalf("Ana's remaining %v", r)
	}
	if err := b.CheckFor(ana, 0.1); !errors.Is(err, ErrPersonOverBudget) || !errors.Is(err, ErrOverBudget) {
		t.Fatalf("Ana past her limit: %v", err)
	}
	if err := b.CheckFor(ctx, 0.1); err != nil {
		t.Fatalf("the owner: %v", err)
	}
	b.SetLimit(ctx, 0.3, "human:owner")
	if err := b.Check(ctx); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("the house's limit: %v", err)
	}
}

package budget

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/denerFernandes/vigia/internal/event"
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

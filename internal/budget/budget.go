// Package budget enforces the owner's spending limit on models. Every paid
// call records its cost as an event, and every paid call checks the limit
// before it starts, so the limit cannot be passed by more than one call.
package budget

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
)

const CostEvent = "cost.recorded"

var ErrOverBudget = errors.New("daily model budget reached")

type Budget struct {
	Events *event.Store
	Zone   *time.Location
	Now    func() time.Time
}

type Cost struct {
	USD    float64 `json:"usd"`
	Source string  `json:"source"`
	Ref    string  `json:"ref,omitempty"`
}

func (b *Budget) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

func (b *Budget) zone() *time.Location {
	if b.Zone != nil {
		return b.Zone
	}
	return time.Local
}

// Limit is the daily limit in dollars; 0 means no limit.
func (b *Budget) Limit(ctx context.Context) float64 {
	v, _ := b.Events.Get(ctx, "budget.daily_usd")
	if v == "" {
		return 1
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func (b *Budget) SetLimit(ctx context.Context, usd float64, actor string) error {
	if usd < 0 {
		return errors.New("a budget cannot be negative")
	}
	if err := b.Events.Put(ctx, "budget.daily_usd", strconv.FormatFloat(usd, 'f', 2, 64)); err != nil {
		return err
	}
	_, err := b.Events.Append(ctx, "budget.changed", actor, map[string]float64{"daily_usd": usd})
	return err
}

func (b *Budget) startOfDay() time.Time {
	t := b.now().In(b.zone())
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, b.zone())
}

// Spent sums costs since t.
func (b *Budget) Spent(ctx context.Context, since time.Time) (float64, error) {
	evs, err := b.Events.List(ctx, event.Query{Types: []string{CostEvent}})
	if err != nil {
		return 0, err
	}
	total := 0.0
	for _, e := range evs {
		if e.Time.Before(since) {
			continue
		}
		var c Cost
		if e.Decode(&c) == nil {
			total += c.USD
		}
	}
	return total, nil
}

func (b *Budget) Today(ctx context.Context) (float64, error) { return b.Spent(ctx, b.startOfDay()) }

// Check refuses a paid call when today's spending has reached the limit.
func (b *Budget) Check(ctx context.Context) error { return b.CheckFor(ctx, 0) }

// CheckFor refuses a call that could push spending past the limit, given
// the most it may cost.
func (b *Budget) CheckFor(ctx context.Context, estimate float64) error {
	limit := b.Limit(ctx)
	if limit <= 0 {
		return nil
	}
	spent, err := b.Today(ctx)
	if err != nil {
		return err
	}
	if spent >= limit || spent+estimate > limit+1e-9 {
		return fmt.Errorf("%w ($%.2f of $%.2f); raise it in Ajustes › Geral or wait until tomorrow", ErrOverBudget, spent, limit)
	}
	return nil
}

func (b *Budget) Record(ctx context.Context, c Cost) error {
	if c.USD <= 0 {
		return nil
	}
	_, err := b.Events.Append(ctx, CostEvent, "system", c)
	return err
}

// Remaining is what may still be spent today, or -1 without a limit.
func (b *Budget) Remaining(ctx context.Context) float64 {
	limit := b.Limit(ctx)
	if limit <= 0 {
		return -1
	}
	spent, _ := b.Today(ctx)
	return max(0, limit-spent)
}

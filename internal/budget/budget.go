// Package budget enforces the owner's spending limit on models. Every paid
// call records its cost as an event, and every paid call checks the limit
// before it starts, so the limit cannot be passed by more than one call.
// Each cost belongs to the person the call was for, and a person may have
// a daily limit of their own inside the house's.
package budget

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/people"
)

const CostEvent = "cost.recorded"

var ErrOverBudget = errors.New("daily model budget reached")

// ErrPersonOverBudget is a person's own limit reached; such an error is
// also ErrOverBudget, so nothing retries it.
var ErrPersonOverBudget = errors.New("the person's daily budget reached")

// personOver is a person's own limit reached, told in their language.
type personOver struct{ msg string }

func (e personOver) Error() string { return e.msg }

func (e personOver) Is(target error) bool {
	return target == ErrOverBudget || target == ErrPersonOverBudget
}

type Budget struct {
	Events *event.Store
	Zone   *time.Location
	Now    func() time.Time
	// PersonLimit is a person's own daily limit in dollars, 0 for none;
	// nil means nobody has one.
	PersonLimit func(ctx context.Context, person string) float64
}

type Cost struct {
	USD    float64 `json:"usd"`
	Source string  `json:"source"`
	Ref    string  `json:"ref,omitempty"`
	// Person the call was for; costs from before people existed, with
	// none, are the owner's.
	Person string `json:"person,omitempty"`
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
	return b.spent(ctx, since, "")
}

// SpentBy sums one person's costs since t.
func (b *Budget) SpentBy(ctx context.Context, person string, since time.Time) (float64, error) {
	return b.spent(ctx, since, people.Norm(person))
}

// spent sums costs since t, of one person or, with "", of everyone.
func (b *Budget) spent(ctx context.Context, since time.Time, person string) (float64, error) {
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
		if e.Decode(&c) == nil && (person == "" || people.Norm(c.Person) == person) {
			total += c.USD
		}
	}
	return total, nil
}

func (b *Budget) Today(ctx context.Context) (float64, error) { return b.Spent(ctx, b.startOfDay()) }

// TodayFor is what one person spent today.
func (b *Budget) TodayFor(ctx context.Context, person string) (float64, error) {
	return b.SpentBy(ctx, person, b.startOfDay())
}

// LimitFor is a person's own daily limit, 0 for none.
func (b *Budget) LimitFor(ctx context.Context, person string) float64 {
	if b.PersonLimit == nil {
		return 0
	}
	return b.PersonLimit(ctx, people.Norm(person))
}

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
	return b.checkPerson(ctx, estimate)
}

// checkPerson refuses a call that could push the person it is for past
// their own limit.
func (b *Budget) checkPerson(ctx context.Context, estimate float64) error {
	person := people.From(ctx)
	limit := b.LimitFor(ctx, person)
	if limit <= 0 {
		return nil
	}
	spent, err := b.TodayFor(ctx, person)
	if err != nil {
		return err
	}
	if spent >= limit || spent+estimate > limit+1e-9 {
		return personOver{i18n.T(ctx, "budget.person_over", "spent", fmt.Sprintf("$%.2f", spent), "limit", fmt.Sprintf("$%.2f", limit))}
	}
	return nil
}

// Record books a cost for the person it was for, the one ctx acts for
// unless the cost names them.
func (b *Budget) Record(ctx context.Context, c Cost) error {
	if c.USD <= 0 {
		return nil
	}
	if c.Person == "" {
		c.Person = people.From(ctx)
	}
	_, err := b.Events.Append(ctx, CostEvent, "system", c)
	return err
}

// Remaining is what may still be spent today, within the house's limit
// and the person's own, or -1 without either.
func (b *Budget) Remaining(ctx context.Context) float64 {
	left := -1.0
	if limit := b.Limit(ctx); limit > 0 {
		spent, _ := b.Today(ctx)
		left = max(0, limit-spent)
	}
	person := people.From(ctx)
	if limit := b.LimitFor(ctx, person); limit > 0 {
		spent, _ := b.TodayFor(ctx, person)
		if mine := max(0, limit-spent); left < 0 || mine < left {
			left = mine
		}
	}
	return left
}

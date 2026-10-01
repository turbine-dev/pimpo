package company

import "fmt"

// What a company does when it reaches a limit.
const (
	OnLimitPause = "pause"
	OnLimitWarn  = "warn"
)

// A Budget is a limit a day and a month; 0 is none.
type Budget struct {
	DayUSD   float64 `json:"day_usd,omitempty" yaml:"day_usd,omitempty"`
	MonthUSD float64 `json:"month_usd,omitempty" yaml:"month_usd,omitempty"`
	OnLimit  string  `json:"on_limit,omitempty" yaml:"on_limit,omitempty"`
	// Subscription counts work paid by a subscription (Claude Code, Codex)
	// at what it would cost on the API, so the limits hold there too.
	Subscription bool `json:"subscription,omitempty" yaml:"subscription,omitempty"`
}

func (b Budget) check() error {
	switch {
	case b.DayUSD < 0 || b.MonthUSD < 0:
		return fmt.Errorf("a budget is not negative")
	case b.OnLimit != "" && b.OnLimit != OnLimitPause && b.OnLimit != OnLimitWarn:
		return fmt.Errorf("at a limit, members pause or the CEO is warned")
	}
	return nil
}

// Spend is what was spent today and this month.
type Spend struct {
	Day   float64 `json:"day"`
	Month float64 `json:"month"`
}

// Plus is two spends together.
func (s Spend) Plus(o Spend) Spend { return Spend{s.Day + o.Day, s.Month + o.Month} }

// Over says which of a budget's limits a spend reached, or "".
func (b Budget) Over(s Spend) string {
	switch {
	case b.DayUSD > 0 && s.Day >= b.DayUSD:
		return fmt.Sprintf("$%.2f of $%.2f today", s.Day, b.DayUSD)
	case b.MonthUSD > 0 && s.Month >= b.MonthUSD:
		return fmt.Sprintf("$%.2f of $%.2f this month", s.Month, b.MonthUSD)
	}
	return ""
}

// Near says whether a spend reached 80% of a limit.
func (b Budget) Near(s Spend) bool {
	return b.DayUSD > 0 && s.Day >= 0.8*b.DayUSD || b.MonthUSD > 0 && s.Month >= 0.8*b.MonthUSD
}

// Left is what may still be spent, the lower of the day and the month,
// or -1 without a limit.
func (b Budget) Left(s Spend) float64 {
	left := -1.0
	if b.DayUSD > 0 {
		left = max(0, b.DayUSD-s.Day)
	}
	if b.MonthUSD > 0 {
		m := max(0, b.MonthUSD-s.Month)
		if left < 0 || m < left {
			left = m
		}
	}
	return left
}

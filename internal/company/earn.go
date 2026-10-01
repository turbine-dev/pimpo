package company

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Earned autonomy: Pimpo counts, per member and kind of delivery (a
// capability), the approvals in a row a person gave without saying no.
// After enough of them it suggests letting the member do it alone; a
// person decides. A no afterwards takes it back to approval.

const earnSchema = `
CREATE TABLE IF NOT EXISTS company_streaks (
  company    TEXT NOT NULL,
  member     TEXT NOT NULL,
  capability TEXT NOT NULL,
  count      INTEGER NOT NULL DEFAULT 0,
  suggested  INTEGER NOT NULL DEFAULT 0,
  earned     INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (company, member, capability)
);`

// EarnAfter is how many approvals in a row a company asks for by default.
const EarnAfter = 10

// Earning is the approvals a company asks for before suggesting
// autonomy, or 0 when it suggests none.
func (c Company) Earning() int {
	switch {
	case c.EarnOff:
		return 0
	case c.EarnAfter > 0:
		return c.EarnAfter
	}
	return EarnAfter
}

func (c Company) checkEarning() error {
	if c.EarnAfter < 0 || c.EarnAfter > 100 {
		return errors.New("autonomy is suggested after 1 to 100 approvals in a row")
	}
	return nil
}

type Streak struct {
	Company    string    `json:"company"`
	Member     string    `json:"member"`
	Capability string    `json:"capability"`
	Count      int       `json:"count"`
	Suggested  bool      `json:"suggested"`
	Earned     bool      `json:"earned"`
	Updated    time.Time `json:"updated"`
}

func (s *Store) streak(ctx context.Context, tx *sql.Tx, co, member, capability string) (Streak, error) {
	st := Streak{Company: co, Member: member, Capability: capability}
	var updated string
	err := tx.QueryRowContext(ctx, `SELECT count, suggested, earned, updated_at FROM company_streaks WHERE company = ? AND member = ? AND capability = ?`, co, member, capability).
		Scan(&st.Count, &st.Suggested, &st.Earned, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	st.Updated, _ = time.Parse(time.RFC3339Nano, updated)
	return st, err
}

func (s *Store) putStreak(ctx context.Context, tx *sql.Tx, st Streak) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO company_streaks (company, member, capability, count, suggested, earned, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)
	  ON CONFLICT(company, member, capability) DO UPDATE SET count = excluded.count, suggested = excluded.suggested, earned = excluded.earned, updated_at = excluded.updated_at`,
		st.Company, st.Member, st.Capability, st.Count, st.Suggested, st.Earned, s.now().Format(time.RFC3339Nano))
	return err
}

// changeStreak applies f to a streak in one transaction.
func (s *Store) changeStreak(ctx context.Context, co, member, capability string, f func(*Streak)) (Streak, error) {
	var out Streak
	err := s.write(ctx, func(tx *sql.Tx) error {
		st, err := s.streak(ctx, tx, co, member, capability)
		if err != nil {
			return err
		}
		f(&st)
		out = st
		return s.putStreak(ctx, tx, st)
	})
	return out, err
}

// Approved counts one more approval; suggest says this one reached after
// and autonomy should now be offered.
func (s *Store) Approved(ctx context.Context, co, member, capability string, after int) (Streak, bool, error) {
	suggest := false
	st, err := s.changeStreak(ctx, co, member, capability, func(st *Streak) {
		st.Count++
		if after > 0 && st.Count >= after && !st.Suggested && !st.Earned {
			st.Suggested, suggest = true, true
		}
	})
	return st, suggest, err
}

// Denied starts the count again; wasEarned says the member had earned
// this kind, which now goes back to approval.
func (s *Store) Denied(ctx context.Context, co, member, capability string) (bool, error) {
	was := false
	_, err := s.changeStreak(ctx, co, member, capability, func(st *Streak) {
		was = st.Earned
		st.Count, st.Suggested, st.Earned = 0, false, false
	})
	return was, err
}

// Earn records that a person gave the member this kind; Decline that they
// did not, and the count starts again.
func (s *Store) Earn(ctx context.Context, co, member, capability string) error {
	_, err := s.changeStreak(ctx, co, member, capability, func(st *Streak) { st.Suggested, st.Earned = false, true })
	return err
}

func (s *Store) Decline(ctx context.Context, co, member, capability string) error {
	_, err := s.changeStreak(ctx, co, member, capability, func(st *Streak) { st.Count, st.Suggested, st.Earned = 0, false, false })
	return err
}

// Streaks are a company's counts, those waiting for a decision first.
func (s *Store) Streaks(ctx context.Context, co string) ([]Streak, error) {
	r, err := s.DB.QueryContext(ctx, `SELECT member, capability, count, suggested, earned, updated_at FROM company_streaks WHERE company = ? ORDER BY suggested DESC, count DESC`, co)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out := []Streak{}
	for r.Next() {
		st := Streak{Company: co}
		var updated string
		if err := r.Scan(&st.Member, &st.Capability, &st.Count, &st.Suggested, &st.Earned, &updated); err != nil {
			return nil, err
		}
		st.Updated, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, st)
	}
	return out, r.Err()
}

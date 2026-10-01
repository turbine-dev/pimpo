package company

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Hours are when a company works: on these days (0 is Sunday), from
// From to To in its time zone. No days means always. To before From
// spans midnight.
type Hours struct {
	Days []int  `json:"days,omitempty" yaml:"days,omitempty"`
	From string `json:"from,omitempty" yaml:"from,omitempty"`
	To   string `json:"to,omitempty" yaml:"to,omitempty"`
}

func clock(s string) (int, bool) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || h < 0 || h > 24 || m < 0 || m > 59 || h == 24 && m != 0 {
		return 0, false
	}
	return h*60 + m, true
}

func (h Hours) check() error {
	if len(h.Days) == 0 {
		return nil
	}
	for _, d := range h.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("days are 0 (Sunday) to 6")
		}
	}
	from, ok1 := clock(h.From)
	to, ok2 := clock(h.To)
	if !ok1 || !ok2 || from == to {
		return fmt.Errorf("working hours are HH:MM to HH:MM")
	}
	return nil
}

// Open says whether t, in zone, is within the hours.
func (h Hours) Open(t time.Time, zone *time.Location) bool {
	if len(h.Days) == 0 {
		return true
	}
	from, _ := clock(h.From)
	to, _ := clock(h.To)
	t = t.In(zone)
	now := t.Hour()*60 + t.Minute()
	day := int(t.Weekday())
	if from < to {
		return slices.Contains(h.Days, day) && now >= from && now < to
	}
	// Overnight: the evening belongs to today, the early morning to yesterday.
	return slices.Contains(h.Days, day) && now >= from || slices.Contains(h.Days, (day+6)%7) && now < to
}

// An AgentRoutine is work a member does on a schedule with a model:
// instructions, a cron schedule and what one run may spend.
type AgentRoutine struct {
	ID           string  `json:"id" yaml:"id"`
	Member       string  `json:"member" yaml:"member"`
	Name         string  `json:"name" yaml:"name"`
	Instructions string  `json:"instructions" yaml:"instructions"`
	Schedule     string  `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	MaxUSD       float64 `json:"max_usd,omitempty" yaml:"max_usd,omitempty"`
	Off          bool    `json:"off,omitempty" yaml:"off,omitempty"`
}

// MaxWorkUSD is the most one piece of work may spend.
const MaxWorkUSD = 5.0

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// ParseSchedule reads a five-field cron schedule.
func ParseSchedule(s string) (cron.Schedule, error) { return cronParser.Parse(s) }

func (o Org) checkWork() error {
	if err := o.Hours.check(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range o.AgentRoutines {
		m, ok := o.Member(r.Member)
		switch {
		case !ValidID(r.ID) || seen[r.ID]:
			return fmt.Errorf("routine %q needs a unique id", r.ID)
		case !ok || m.Kind != Agent:
			return fmt.Errorf("routine %q is for an agent of the company", r.Name)
		case strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.Instructions) == "":
			return fmt.Errorf("routine %q needs a name and instructions", r.ID)
		case r.MaxUSD < 0 || r.MaxUSD > MaxWorkUSD:
			return fmt.Errorf("one run spends at most $%.0f", MaxWorkUSD)
		}
		if r.Schedule != "" {
			if _, err := ParseSchedule(r.Schedule); err != nil {
				return fmt.Errorf("routine %q: the schedule: %w", r.Name, err)
			}
		}
		seen[r.ID] = true
	}
	for _, m := range o.Members {
		if m.Hours != nil {
			if err := m.Hours.check(); err != nil {
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		}
	}
	return nil
}

// Working says whether a member may work now, and why not: it, its
// department or the company is paused.
func (o Org) Working(member string) (bool, string) {
	m, ok := o.Member(member)
	switch {
	case !ok:
		return false, "no longer in the company"
	case o.Paused:
		return false, "the company is paused"
	case m.State == Paused:
		return false, m.Name + " is paused"
	}
	if d, ok := o.Department(m.Department); ok && d.Paused {
		return false, d.Name + " is paused"
	}
	return true, ""
}

// OnDuty says whether a member's hours are open at t.
func (o Org) OnDuty(member string, t time.Time) bool {
	zone, err := time.LoadLocation(o.Zone)
	if err != nil {
		zone = time.Local
	}
	m, _ := o.Member(member)
	h := o.Hours
	if m.Hours != nil {
		h = *m.Hours
	}
	return h.Open(t, zone)
}

// Work states.
const (
	WorkQueued  = "queued"
	WorkRunning = "running"
	WorkDone    = "done"
	WorkFailed  = "failed"
	WorkStopped = "stopped"
)

// Work is one piece of work a member does with a model: what it was asked,
// what it was handed (data from outside, never instructions), and how it
// went.
type Work struct {
	ID          string          `json:"id"`
	Company     string          `json:"company"`
	Member      string          `json:"member"`
	Request     string          `json:"request"`
	Data        json.RawMessage `json:"data,omitempty"`
	From        string          `json:"from"`
	MaxUSD      float64         `json:"max_usd"`
	State       string          `json:"state"`
	Exploration string          `json:"exploration,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	Error       string          `json:"error,omitempty"`
	CostUSD     float64         `json:"cost_usd"`
	Queued      time.Time       `json:"queued"`
	Started     time.Time       `json:"started,omitzero"`
	Ended       time.Time       `json:"ended,omitzero"`
}

const workSchema = `
CREATE TABLE IF NOT EXISTS company_work (
  id         TEXT PRIMARY KEY,
  company    TEXT NOT NULL,
  state      TEXT NOT NULL,
  data       TEXT NOT NULL,
  queued_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS company_work_state ON company_work (state, queued_at);
CREATE INDEX IF NOT EXISTS company_work_company ON company_work (company, queued_at);`

func (s *Store) SaveWork(ctx context.Context, w Work) error {
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO company_work (id, company, state, data, queued_at) VALUES (?, ?, ?, ?, ?)
	  ON CONFLICT(id) DO UPDATE SET state = excluded.state, data = excluded.data`, w.ID, w.Company, w.State, string(b), w.Queued.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Work(ctx context.Context, id string) (Work, error) {
	list, err := s.works(ctx, `WHERE id = ?`, id)
	if err != nil {
		return Work{}, err
	}
	if len(list) == 0 {
		return Work{}, ErrNotFound
	}
	return list[0], nil
}

// Waiting is the work queued or running in every company, oldest first.
func (s *Store) Waiting(ctx context.Context) ([]Work, error) {
	return s.works(ctx, `WHERE state IN (?, ?) ORDER BY queued_at`, WorkQueued, WorkRunning)
}

// Works is a company's latest work, newest first.
func (s *Store) Works(ctx context.Context, company string, limit int) ([]Work, error) {
	return s.works(ctx, `WHERE company = ? ORDER BY queued_at DESC LIMIT ?`, company, limit)
}

func (s *Store) works(ctx context.Context, where string, args ...any) ([]Work, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM company_work `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Work{}
	for rows.Next() {
		var data string
		var w Work
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// UpdateWork changes one piece of work under the store's lock.
func (s *Store) UpdateWork(ctx context.Context, id string, f func(*Work)) (Work, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.Work(ctx, id)
	if err != nil {
		return Work{}, err
	}
	f(&w)
	return w, s.SaveWork(ctx, w)
}

var errNoAgent = errors.New("only an agent of the company does work")

// CheckWork says whether a member may be given work.
func (o Org) CheckWork(member string) error {
	m, ok := o.Member(member)
	if !ok || m.Kind != Agent {
		return errNoAgent
	}
	return nil
}

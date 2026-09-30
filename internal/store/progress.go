package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Progress is where a long job or a routine run is, kept as it goes so a
// reload, another device or a restart shows the same thing. Its label is
// one of Pimpo's own step names (a capability, a part of a job), short
// and capped, never a model's free text.
type Progress struct {
	// ID is "job:<id>" or "run:<routine>:<run>".
	ID     string `json:"id"`
	Person string `json:"person"`
	// Kind is "job" or "run".
	Kind    string `json:"kind"`
	Job     string `json:"job,omitempty"`
	Routine string `json:"routine,omitempty"`
	Run     int64  `json:"run,omitempty"`
	// Title is what the work is: a routine's name, a job's request.
	Title string `json:"title"`
	// Label is the step it is on.
	Label string `json:"label,omitempty"`
	Done  int    `json:"done"`
	// Total is 0 when the number of steps is not known, as in a routine.
	Total int `json:"total"`
	// State is running, done or failed.
	State string `json:"state"`
	// Phase says more: reporting, stopped, interrupted (by a restart).
	Phase string `json:"phase,omitempty"`
	// Resumed says the work picked up again after a restart.
	Resumed   bool      `json:"resumed,omitempty"`
	CostUSD   float64   `json:"cost_usd"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
}

const (
	ProgressRunning = "running"
	ProgressDone    = "done"
	ProgressFailed  = "failed"

	// progressKept is how many finished records each person keeps.
	progressKept = 50
)

// Final says the work ended.
func (p Progress) Final() bool { return p.State != ProgressRunning }

// sortable keeps every digit, so the times sort as text.
const sortable = "2006-01-02T15:04:05.000000000Z"

const progressSchema = `
CREATE TABLE IF NOT EXISTS progress (
  id         TEXT PRIMARY KEY,
  person     TEXT NOT NULL,
  state      TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  data       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS progress_person ON progress (person, updated_at);`

// SaveProgress keeps a record, replacing the one with its id, and
// forgets a person's oldest finished records beyond what is kept.
func (s *Store) SaveProgress(ctx context.Context, p Progress) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO progress (id, person, state, updated_at, data) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET person = excluded.person, state = excluded.state, updated_at = excluded.updated_at, data = excluded.data`,
		p.ID, p.Person, p.State, p.UpdatedAt.UTC().Format(sortable), string(b))
	if err != nil || !p.Final() {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM progress WHERE person = ? AND state != ? AND id NOT IN
		(SELECT id FROM progress WHERE person = ? AND state != ? ORDER BY updated_at DESC LIMIT ?)`,
		p.Person, ProgressRunning, p.Person, ProgressRunning, progressKept)
	return err
}

// Progress is one record.
func (s *Store) Progress(ctx context.Context, id string) (Progress, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM progress WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Progress{}, ErrNotFound
	}
	if err != nil {
		return Progress{}, err
	}
	var p Progress
	return p, json.Unmarshal([]byte(raw), &p)
}

// ActiveProgress is what is running now for a person, newest first.
func (s *Store) ActiveProgress(ctx context.Context, person string) ([]Progress, error) {
	return s.progressWhere(ctx, `WHERE person = ? AND state = ? ORDER BY updated_at DESC`, person, ProgressRunning)
}

// RecentProgress is a person's running and recently finished work,
// newest first.
func (s *Store) RecentProgress(ctx context.Context, person string, limit int) ([]Progress, error) {
	return s.progressWhere(ctx, `WHERE person = ? ORDER BY updated_at DESC LIMIT ?`, person, limit)
}

// AllActiveProgress is everyone's running work, for the restart to
// settle what it interrupted. Never for an API.
func (s *Store) AllActiveProgress(ctx context.Context) ([]Progress, error) {
	return s.progressWhere(ctx, `WHERE state = ?`, ProgressRunning)
}

func (s *Store) progressWhere(ctx context.Context, where string, args ...any) ([]Progress, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM progress `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Progress{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var p Progress
		if json.Unmarshal([]byte(raw), &p) == nil {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

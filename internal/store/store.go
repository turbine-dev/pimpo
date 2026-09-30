// Package store keeps Pimpo's working state: routines and their versions,
// explorations waiting for approval, and runs. The event log remains the
// record of what happened; these tables are the current picture.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/trace"
)

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS routines (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  version    INTEGER NOT NULL,
  state      TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS routine_versions (
  routine    TEXT NOT NULL REFERENCES routines(id),
  version    INTEGER NOT NULL,
  body       TEXT NOT NULL,
  reason     TEXT NOT NULL,
  approved_by TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (routine, version)
);
CREATE TABLE IF NOT EXISTS explorations (
  id         TEXT PRIMARY KEY,
  request    TEXT NOT NULL,
  state      TEXT NOT NULL,
  trace      TEXT,
  summary    TEXT,
  routine    TEXT,
  cost_usd   REAL NOT NULL DEFAULT 0,
  error      TEXT,
  candidate  TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS runs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  routine    TEXT NOT NULL,
  version    INTEGER NOT NULL,
  started_at TEXT NOT NULL,
  ended_at   TEXT,
  outcome    TEXT NOT NULL,
  error      TEXT,
  cost_usd   REAL NOT NULL DEFAULT 0,
  calls      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS runs_routine ON runs (routine, id);
CREATE TABLE IF NOT EXISTS chats (
  id         TEXT PRIMARY KEY,
  title      TEXT NOT NULL,
  person     TEXT NOT NULL DEFAULT '',
  assistant  TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS chat_turns (
  chat        TEXT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
  seq         INTEGER NOT NULL,
  exploration TEXT NOT NULL,
  PRIMARY KEY (chat, seq)
);`

// columns added after the first release, applied to older databases.
var additions = []string{
	`ALTER TABLE explorations ADD COLUMN candidate TEXT`,
	`ALTER TABLE explorations ADD COLUMN person TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE routines ADD COLUMN person TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE routines ADD COLUMN settings TEXT NOT NULL DEFAULT ''`,
}

func Open(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate store: %w", err)
	}
	for _, stmt := range additions {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return nil, fmt.Errorf("migrate store: %w", err)
		}
	}
	return &Store{db: db}, nil
}

const (
	RoutineActive = "active"
	RoutinePaused = "paused"
	RoutineBroken = "broken"

	ExplorationRunning   = "running"
	ExplorationReady     = "ready"
	ExplorationCompiling = "compiling"
	ExplorationDone      = "done"
	ExplorationFailed    = "failed"
	ExplorationDiscarded = "discarded"
	// ExplorationImported is a task brought from another agent, waiting
	// for the owner to explore it once.
	ExplorationImported = "imported"
)

type Routine struct {
	ID      string          `json:"id"`
	Version int             `json:"version"`
	State   string          `json:"state"`
	Body    routine.Routine `json:"routine"`
	// Person is who the routine works for; empty is the owner.
	Person string `json:"person,omitempty"`
	// Settings are the owner's choices: parameter values and a schedule
	// that replaces the manifest's.
	Settings  Settings  `json:"settings"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Settings struct {
	Schedule string         `json:"schedule,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
	// WatchEvery overrides how often a watching routine checks.
	WatchEvery string `json:"watch_every,omitempty"`
	// Model is the model for this routine's judgments and texts; "" uses
	// the one set for judgments.
	Model string `json:"model,omitempty"`
	// Effort is how hard that model thinks; "" uses the default.
	Effort string `json:"effort,omitempty"`
}

// Watch is what the routine waits for, with the owner's interval, or nil.
func (r Routine) Watch() *runtime.Watch {
	w := r.Body.Manifest.Watch
	if w == nil {
		return nil
	}
	c := *w
	if r.Settings.WatchEvery != "" {
		c.Every = r.Settings.WatchEvery
	}
	return &c
}

// Schedule is when the routine runs: the owner's choice, else the
// manifest's.
func (r Routine) Schedule() string {
	if r.Settings.Schedule != "" {
		return r.Settings.Schedule
	}
	return r.Body.Manifest.Schedule
}

type Version struct {
	Version    int             `json:"version"`
	Body       routine.Routine `json:"routine"`
	Reason     string          `json:"reason"`
	ApprovedBy string          `json:"approved_by"`
	CreatedAt  time.Time       `json:"created_at"`
}

func ts(t time.Time) string    { return t.UTC().Format(time.RFC3339Nano) }
func parse(s string) time.Time { t, _ := time.Parse(time.RFC3339Nano, s); return t }

// SaveRoutine creates a routine or adds a new version to it.
func (s *Store) SaveRoutine(ctx context.Context, id string, body routine.Routine, reason, approvedBy string) (Routine, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return Routine{}, err
	}
	now := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Routine{}, err
	}
	defer tx.Rollback()
	var version int
	err = tx.QueryRowContext(ctx, `SELECT version FROM routines WHERE id = ?`, id).Scan(&version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		version = 1
		_, err = tx.ExecContext(ctx, `INSERT INTO routines (id, name, version, state, created_at, updated_at) VALUES (?, ?, 1, ?, ?, ?)`, id, body.Name, RoutineActive, ts(now), ts(now))
	case err == nil:
		version++
		_, err = tx.ExecContext(ctx, `UPDATE routines SET name = ?, version = ?, state = ?, updated_at = ? WHERE id = ?`, body.Name, version, RoutineActive, ts(now), id)
	}
	if err != nil {
		return Routine{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO routine_versions (routine, version, body, reason, approved_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, id, version, string(b), reason, approvedBy, ts(now)); err != nil {
		return Routine{}, err
	}
	if err := tx.Commit(); err != nil {
		return Routine{}, err
	}
	return s.Routine(ctx, id)
}

func (s *Store) Routine(ctx context.Context, id string) (Routine, error) {
	rows, err := s.routines(ctx, `WHERE r.id = ?`, id)
	if err != nil {
		return Routine{}, err
	}
	if len(rows) == 0 {
		return Routine{}, ErrNotFound
	}
	return rows[0], nil
}

func (s *Store) Routines(ctx context.Context) ([]Routine, error) {
	return s.routines(ctx, `ORDER BY r.created_at`)
}

func (s *Store) routines(ctx context.Context, where string, args ...any) ([]Routine, error) {
	q := `SELECT r.id, r.version, r.state, r.person, r.settings, r.created_at, r.updated_at, v.body FROM routines r JOIN routine_versions v ON v.routine = r.id AND v.version = r.version ` + where
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Routine{}
	for rows.Next() {
		var r Routine
		var created, updated, body string
		var settings string
		if err := rows.Scan(&r.ID, &r.Version, &r.State, &r.Person, &settings, &created, &updated, &body); err != nil {
			return nil, err
		}
		r.CreatedAt, r.UpdatedAt = parse(created), parse(updated)
		json.Unmarshal([]byte(settings), &r.Settings)
		if err := json.Unmarshal([]byte(body), &r.Body); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Versions(ctx context.Context, id string) ([]Version, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version, body, reason, approved_by, created_at FROM routine_versions WHERE routine = ? ORDER BY version DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		var body, created string
		if err := rows.Scan(&v.Version, &body, &v.Reason, &v.ApprovedBy, &created); err != nil {
			return nil, err
		}
		v.CreatedAt = parse(created)
		json.Unmarshal([]byte(body), &v.Body)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SetRoutineSettings saves the owner's parameter values and schedule.
func (s *Store) SetRoutineSettings(ctx context.Context, id string, v Settings) error {
	b, _ := json.Marshal(v)
	return affected(s.db.ExecContext(ctx, `UPDATE routines SET settings = ?, updated_at = ? WHERE id = ?`, string(b), ts(time.Now()), id))
}

// SetRoutinePerson records who a routine works for.
func (s *Store) SetRoutinePerson(ctx context.Context, id, person string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE routines SET person = ? WHERE id = ?`, person, id))
}

func (s *Store) SetRoutineState(ctx context.Context, id, state string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE routines SET state = ?, updated_at = ? WHERE id = ?`, state, ts(time.Now()), id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type Exploration struct {
	ID      string       `json:"id"`
	Request string       `json:"request"`
	State   string       `json:"state"`
	Trace   *trace.Trace `json:"trace,omitempty"`
	Summary string       `json:"summary"`
	Routine string       `json:"routine,omitempty"`
	// Person asked for it; empty is the owner.
	Person  string  `json:"person,omitempty"`
	CostUSD float64 `json:"cost_usd"`
	Error   string  `json:"error,omitempty"`
	// Candidate is the last routine the compiler proposed, kept when it
	// failed its checks so the owner can see why.
	Candidate *routine.Routine `json:"candidate,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

func (s *Store) SaveExploration(ctx context.Context, e Exploration) error {
	var tr, cand any
	if e.Trace != nil {
		b, _ := json.Marshal(e.Trace)
		tr = string(b)
	}
	if e.Candidate != nil {
		b, _ := json.Marshal(e.Candidate)
		cand = string(b)
	}
	now := ts(time.Now())
	_, err := s.db.ExecContext(ctx, `INSERT INTO explorations (id, request, state, trace, summary, routine, cost_usd, error, candidate, person, created_at, updated_at)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	  ON CONFLICT(id) DO UPDATE SET state = excluded.state, trace = excluded.trace, summary = excluded.summary, routine = excluded.routine, cost_usd = excluded.cost_usd, error = excluded.error, candidate = excluded.candidate, updated_at = excluded.updated_at`,
		e.ID, e.Request, e.State, tr, e.Summary, e.Routine, e.CostUSD, e.Error, cand, e.Person, now, now)
	return err
}

func (s *Store) Exploration(ctx context.Context, id string) (Exploration, error) {
	list, err := s.explorations(ctx, `WHERE id = ?`, id)
	if err != nil {
		return Exploration{}, err
	}
	if len(list) == 0 {
		return Exploration{}, ErrNotFound
	}
	return list[0], nil
}

func (s *Store) Explorations(ctx context.Context, states ...string) ([]Exploration, error) {
	if len(states) == 0 {
		return s.explorations(ctx, `ORDER BY created_at DESC LIMIT 100`)
	}
	q := `WHERE state IN (?` + repeat(",?", len(states)-1) + `) ORDER BY created_at DESC`
	args := make([]any, len(states))
	for i, st := range states {
		args[i] = st
	}
	return s.explorations(ctx, q, args...)
}

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}

func (s *Store) explorations(ctx context.Context, where string, args ...any) ([]Exploration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, request, state, trace, summary, routine, cost_usd, error, candidate, person, created_at, updated_at FROM explorations `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Exploration{}
	for rows.Next() {
		var e Exploration
		var tr, summary, rt, errText, cand sql.NullString
		var created, updated string
		if err := rows.Scan(&e.ID, &e.Request, &e.State, &tr, &summary, &rt, &e.CostUSD, &errText, &cand, &e.Person, &created, &updated); err != nil {
			return nil, err
		}
		e.Summary, e.Routine, e.Error = summary.String, rt.String, errText.String
		e.CreatedAt, e.UpdatedAt = parse(created), parse(updated)
		if cand.Valid && cand.String != "" {
			var r routine.Routine
			if json.Unmarshal([]byte(cand.String), &r) == nil {
				e.Candidate = &r
			}
		}
		if tr.Valid && tr.String != "" {
			var t trace.Trace
			if json.Unmarshal([]byte(tr.String), &t) == nil {
				e.Trace = &t
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type Run struct {
	ID        int64     `json:"id"`
	Routine   string    `json:"routine"`
	Version   int       `json:"version"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
	Outcome   string    `json:"outcome"`
	Error     string    `json:"error,omitempty"`
	CostUSD   float64   `json:"cost_usd"`
	Calls     int       `json:"calls"`
}

const (
	RunRunning = "running"
	RunOK      = "ok"
	RunFailed  = "failed"
	RunSkipped = "skipped"
)

func (s *Store) StartRun(ctx context.Context, routine string, version int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO runs (routine, version, started_at, outcome) VALUES (?, ?, ?, ?)`, routine, version, ts(time.Now()), RunRunning)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RunVersion is the routine and code version a run started with.
func (s *Store) RunVersion(ctx context.Context, id int64) (routine string, version int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT routine, version FROM runs WHERE id = ?`, id).Scan(&routine, &version)
	return routine, version, err
}

func (s *Store) FinishRun(ctx context.Context, id int64, outcome, errText string, cost float64, calls int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET ended_at = ?, outcome = ?, error = ?, cost_usd = ?, calls = ? WHERE id = ?`, ts(time.Now()), outcome, errText, cost, calls, id)
	return err
}

func (s *Store) Runs(ctx context.Context, routine string, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, routine, version, started_at, COALESCE(ended_at, ''), outcome, COALESCE(error, ''), cost_usd, calls FROM runs WHERE routine = ? ORDER BY id DESC LIMIT ?`, routine, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		var started, ended string
		if err := rows.Scan(&r.ID, &r.Routine, &r.Version, &started, &ended, &r.Outcome, &r.Error, &r.CostUSD, &r.Calls); err != nil {
			return nil, err
		}
		r.StartedAt, r.EndedAt = parse(started), parse(ended)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Chat is a conversation in the app: a thread of explorations.
type Chat struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Person    string    `json:"person,omitempty"`
	Assistant string    `json:"assistant,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Turns     int       `json:"turns"`
}

func (s *Store) CreateChat(ctx context.Context, c Chat) error {
	now := ts(time.Now())
	_, err := s.db.ExecContext(ctx, `INSERT INTO chats (id, title, person, assistant, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, c.ID, c.Title, c.Person, c.Assistant, now, now)
	return err
}

// AddTurn appends an exploration to a chat.
func (s *Store) AddTurn(ctx context.Context, chat, exploration string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO chat_turns (chat, seq, exploration) SELECT ?, COALESCE(MAX(seq), 0) + 1, ? FROM chat_turns WHERE chat = ?`, chat, exploration, chat)
	if err == nil {
		_, err = s.db.ExecContext(ctx, `UPDATE chats SET updated_at = ? WHERE id = ?`, ts(time.Now()), chat)
	}
	return err
}

// Chats lists a person's chats, most recent first.
func (s *Store) Chats(ctx context.Context, person string) ([]Chat, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.title, c.person, c.assistant, c.created_at, c.updated_at, (SELECT COUNT(*) FROM chat_turns t WHERE t.chat = c.id)
		FROM chats c WHERE c.person = ? ORDER BY c.updated_at DESC LIMIT 200`, person)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Chat{}
	for rows.Next() {
		var c Chat
		var created, updated string
		if err := rows.Scan(&c.ID, &c.Title, &c.Person, &c.Assistant, &created, &updated, &c.Turns); err != nil {
			return nil, err
		}
		c.CreatedAt, c.UpdatedAt = parse(created), parse(updated)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Chat returns a chat and its explorations in order.
func (s *Store) Chat(ctx context.Context, id string) (Chat, []string, error) {
	var c Chat
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, title, person, assistant, created_at, updated_at FROM chats WHERE id = ?`, id).Scan(&c.ID, &c.Title, &c.Person, &c.Assistant, &created, &updated)
	if err != nil {
		return c, nil, err
	}
	c.CreatedAt, c.UpdatedAt = parse(created), parse(updated)
	rows, err := s.db.QueryContext(ctx, `SELECT exploration FROM chat_turns WHERE chat = ? ORDER BY seq`, id)
	if err != nil {
		return c, nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var e string
		rows.Scan(&e)
		ids = append(ids, e)
	}
	c.Turns = len(ids)
	return c, ids, rows.Err()
}

func (s *Store) DeleteChat(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM chat_turns WHERE chat = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM chats WHERE id = ?`, id)
	return err
}

// RecentRun is a run with its routine's name, for the history of all
// routines.
type RecentRun struct {
	Run
	Name string `json:"name"`
}

// RecentRuns lists the latest runs of every routine, newest first;
// outcome filters when not empty, and before pages back from a run id.
func (s *Store) RecentRuns(ctx context.Context, outcome string, before int64, limit int) ([]RecentRun, error) {
	q := `SELECT r.id, r.routine, r.version, r.started_at, COALESCE(r.ended_at, ''), r.outcome, COALESCE(r.error, ''), r.cost_usd, r.calls, COALESCE(t.name, r.routine)
		FROM runs r LEFT JOIN routines t ON t.id = r.routine WHERE (? = '' OR r.outcome = ?) AND (? = 0 OR r.id < ?) ORDER BY r.id DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, outcome, outcome, before, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecentRun{}
	for rows.Next() {
		var r RecentRun
		var started, ended string
		if err := rows.Scan(&r.ID, &r.Routine, &r.Version, &started, &ended, &r.Outcome, &r.Error, &r.CostUSD, &r.Calls, &r.Name); err != nil {
			return nil, err
		}
		r.StartedAt, r.EndedAt = parse(started), parse(ended)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CostSince sums routine run costs since t.
func (s *Store) RunCostSince(ctx context.Context, routine string, t time.Time) (float64, error) {
	var c sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `SELECT SUM(cost_usd) FROM runs WHERE routine = ? AND started_at >= ?`, routine, ts(t)).Scan(&c)
	return c.Float64, err
}

// DB exposes the connection for tests and migrations.
func (s *Store) DB() *sql.DB { return s.db }

// ForgetPerson deletes a removed person's routines (with their versions
// and runs) and explorations, and returns the routine ids so the
// scheduler can let them go.
func (s *Store) ForgetPerson(ctx context.Context, person string) ([]string, error) {
	if person == "" || person == "owner" {
		return nil, errors.New("the owner is never forgotten")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM routines WHERE person = ?`, person)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		for _, q := range []string{`DELETE FROM runs WHERE routine = ?`, `DELETE FROM routine_versions WHERE routine = ?`, `DELETE FROM routines WHERE id = ?`} {
			if _, err := s.db.ExecContext(ctx, q, id); err != nil {
				return ids, err
			}
		}
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM explorations WHERE person = ?`, person)
	return ids, err
}

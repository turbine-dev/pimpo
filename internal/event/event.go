// Package event is Vigia's append-only log. Every action, decision and
// message is an event, and each event carries the hash of the one before
// it, so any edit to history is detectable.
package event

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Event struct {
	ID    int64           `json:"id"`
	Time  time.Time       `json:"ts"`
	Type  string          `json:"type"`
	Actor string          `json:"actor"`
	Data  json.RawMessage `json:"data"`
	Prev  string          `json:"prev"`
	Hash  string          `json:"hash"`
}

// Decode unmarshals the event's data.
func (e Event) Decode(v any) error { return json.Unmarshal(e.Data, v) }

type Store struct {
	db   *sql.DB
	mu   sync.Mutex
	now  func() time.Time
	subs map[chan Event]struct{}
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return &Store{db: db, now: time.Now, subs: map[chan Event]struct{}{}}, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS events (
  id    INTEGER PRIMARY KEY AUTOINCREMENT,
  ts    TEXT NOT NULL,
  type  TEXT NOT NULL,
  actor TEXT NOT NULL,
  data  TEXT NOT NULL,
  prev  TEXT NOT NULL,
  hash  TEXT NOT NULL UNIQUE
);
CREATE INDEX IF NOT EXISTS events_type ON events (type, id);
CREATE TABLE IF NOT EXISTS kv (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);`

func (s *Store) DB() *sql.DB                 { return s.db }
func (s *Store) Close() error                { return s.db.Close() }
func (s *Store) SetClock(f func() time.Time) { s.now = f }

// Append writes an event after the current head of the chain.
func (s *Store) Append(ctx context.Context, typ, actor string, data any) (Event, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var prev string
	err = s.db.QueryRowContext(ctx, `SELECT hash FROM events ORDER BY id DESC LIMIT 1`).Scan(&prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Event{}, err
	}
	e := Event{Time: s.now().UTC().Truncate(time.Microsecond), Type: typ, Actor: actor, Data: raw, Prev: prev}
	e.Hash = digest(e)
	res, err := s.db.ExecContext(ctx, `INSERT INTO events (ts, type, actor, data, prev, hash) VALUES (?, ?, ?, ?, ?, ?)`,
		e.Time.Format(time.RFC3339Nano), e.Type, e.Actor, string(e.Data), e.Prev, e.Hash)
	if err != nil {
		return Event{}, err
	}
	e.ID, _ = res.LastInsertId()
	for ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
	return e, nil
}

func digest(e Event) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%s\n", e.Prev, e.Time.Format(time.RFC3339Nano), e.Type, e.Actor, e.Data)
	return hex.EncodeToString(h.Sum(nil))
}

type Query struct {
	After  int64
	Types  []string
	Limit  int
	Search string
	// Newest returns the most recent events first.
	Newest bool
}

func (s *Store) List(ctx context.Context, q Query) ([]Event, error) {
	sqlq := `SELECT id, ts, type, actor, data, prev, hash FROM events WHERE id > ?`
	args := []any{q.After}
	if len(q.Types) > 0 {
		sqlq += ` AND type IN (?` + strings.Repeat(",?", len(q.Types)-1) + `)`
		for _, t := range q.Types {
			args = append(args, t)
		}
	}
	if q.Search != "" {
		sqlq += ` AND data LIKE ?`
		args = append(args, "%"+q.Search+"%")
	}
	if q.Newest {
		sqlq += ` ORDER BY id DESC`
	} else {
		sqlq += ` ORDER BY id`
	}
	if q.Limit > 0 {
		sqlq += fmt.Sprintf(` LIMIT %d`, q.Limit)
	}
	rows, err := s.db.QueryContext(ctx, sqlq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ts, data string
		if err := rows.Scan(&e.ID, &ts, &e.Type, &e.Actor, &data, &e.Prev, &e.Hash); err != nil {
			return nil, err
		}
		e.Time, _ = time.Parse(time.RFC3339Nano, ts)
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Verify walks the whole chain and returns the first event whose hash or
// link does not match, or 0 when the log is intact.
func (s *Store) Verify(ctx context.Context) (int64, error) {
	events, err := s.List(ctx, Query{})
	if err != nil {
		return 0, err
	}
	prev := ""
	for _, e := range events {
		if e.Prev != prev || digest(e) != e.Hash {
			return e.ID, nil
		}
		prev = e.Hash
	}
	return 0, nil
}

// Subscribe delivers new events until the context ends. Slow readers miss
// events rather than block writers; they can catch up with List.
func (s *Store) Subscribe(ctx context.Context) <-chan Event {
	ch := make(chan Event, 256)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
		close(ch)
	}()
	return ch
}

func (s *Store) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM kv WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) Put(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// ByID returns one event.
func (s *Store) ByID(ctx context.Context, id int64) (Event, error) {
	evs, err := s.List(ctx, Query{After: id - 1, Limit: 1})
	if err != nil {
		return Event{}, err
	}
	if len(evs) == 0 || evs[0].ID != id {
		return Event{}, fmt.Errorf("event %d not found", id)
	}
	return evs[0], nil
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// A Widget is what a routine last showed with widget.show: its kind, its
// title and the snapshot to draw. It belongs to the routine's person, and
// Shared puts it on the house's shared dashboards.
type Widget struct {
	ID       string          `json:"id"`
	Person   string          `json:"person,omitempty"`
	Routine  string          `json:"routine,omitempty"`
	Key      string          `json:"key,omitempty"`
	Kind     string          `json:"kind"`
	Title    string          `json:"title"`
	Snapshot json.RawMessage `json:"snapshot"`
	Shared   bool            `json:"shared"`
	Updated  time.Time       `json:"updated"`
}

// A Point is one value of a widget over time.
type Point struct {
	Time  time.Time `json:"t"`
	Value float64   `json:"v"`
}

// historyKeep is how many values a widget keeps for its charts.
const historyKeep = 90

// SaveWidget writes a routine's widget, made the first time it shows one
// under that key.
func (s *Store) SaveWidget(ctx context.Context, w Widget) (Widget, error) {
	now := time.Now().UTC()
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM widgets WHERE routine = ? AND key = ?`, w.Routine, w.Key).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if w.ID == "" {
			return Widget{}, errors.New("a new widget needs an id")
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO widgets (id, person, routine, key, kind, title, snapshot, shared, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
			w.ID, w.Person, w.Routine, w.Key, w.Kind, w.Title, string(w.Snapshot), ts(now))
	case err == nil:
		w.ID = id
		_, err = s.db.ExecContext(ctx, `UPDATE widgets SET person = ?, kind = ?, title = ?, snapshot = ?, updated_at = ? WHERE id = ?`,
			w.Person, w.Kind, w.Title, string(w.Snapshot), ts(now), id)
	}
	if err != nil {
		return Widget{}, err
	}
	return s.Widget(ctx, w.ID)
}

func (s *Store) Widget(ctx context.Context, id string) (Widget, error) {
	ws, err := s.widgets(ctx, `WHERE id = ?`, id)
	if err != nil {
		return Widget{}, err
	}
	if len(ws) == 0 {
		return Widget{}, ErrNotFound
	}
	return ws[0], nil
}

// Widgets are every widget, newest first; callers keep the ones a person
// may see.
func (s *Store) Widgets(ctx context.Context) ([]Widget, error) {
	return s.widgets(ctx, `ORDER BY updated_at DESC`)
}

func (s *Store) widgets(ctx context.Context, where string, args ...any) ([]Widget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, person, routine, key, kind, title, snapshot, shared, updated_at FROM widgets `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Widget{}
	for rows.Next() {
		var w Widget
		var snap, updated string
		if err := rows.Scan(&w.ID, &w.Person, &w.Routine, &w.Key, &w.Kind, &w.Title, &snap, &w.Shared, &updated); err != nil {
			return nil, err
		}
		w.Snapshot, w.Updated = json.RawMessage(snap), parse(updated)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) SetWidgetShared(ctx context.Context, id string, shared bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE widgets SET shared = ? WHERE id = ?`, shared, id)
	return err
}

func (s *Store) DeleteWidget(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM widget_history WHERE widget = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM widgets WHERE id = ?`, id)
	return err
}

// AddPoint keeps a value for a widget's chart, dropping the oldest past
// historyKeep.
func (s *Store) AddPoint(ctx context.Context, widget string, v float64) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO widget_history (widget, ts, value) VALUES (?, ?, ?)`, widget, ts(time.Now()), v); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM widget_history WHERE widget = ? AND rowid NOT IN (SELECT rowid FROM widget_history WHERE widget = ? ORDER BY ts DESC LIMIT ?)`, widget, widget, historyKeep)
	return err
}

func (s *Store) History(ctx context.Context, widget string) ([]Point, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ts, value FROM widget_history WHERE widget = ? ORDER BY ts`, widget)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Point{}
	for rows.Next() {
		var t string
		var p Point
		if err := rows.Scan(&t, &p.Value); err != nil {
			return nil, err
		}
		p.Time = parse(t)
		out = append(out, p)
	}
	return out, rows.Err()
}

// A Dashboard is a tab of widgets laid out on a grid. Shared dashboards are
// seen by everyone in the house, and show them only widgets that are
// shared or that everyone has of their own.
type Dashboard struct {
	ID       string          `json:"id"`
	Person   string          `json:"person,omitempty"`
	Name     string          `json:"name"`
	Emoji    string          `json:"emoji"`
	Position int             `json:"position"`
	Layout   json.RawMessage `json:"layout"`
	Shared   bool            `json:"shared"`
	Updated  time.Time       `json:"updated"`
}

func (s *Store) SaveDashboard(ctx context.Context, d Dashboard) error {
	if len(d.Layout) == 0 {
		d.Layout = json.RawMessage("[]")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO dashboards (id, person, name, emoji, position, layout, shared, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, emoji = excluded.emoji, position = excluded.position, layout = excluded.layout, shared = excluded.shared, updated_at = excluded.updated_at`,
		d.ID, d.Person, d.Name, d.Emoji, d.Position, string(d.Layout), d.Shared, ts(time.Now()))
	return err
}

func (s *Store) Dashboards(ctx context.Context) ([]Dashboard, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, person, name, emoji, position, layout, shared, updated_at FROM dashboards ORDER BY position, updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Dashboard{}
	for rows.Next() {
		var d Dashboard
		var layout, updated string
		if err := rows.Scan(&d.ID, &d.Person, &d.Name, &d.Emoji, &d.Position, &layout, &d.Shared, &updated); err != nil {
			return nil, err
		}
		d.Layout, d.Updated = json.RawMessage(layout), parse(updated)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeleteDashboard(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dashboards WHERE id = ?`, id)
	return err
}

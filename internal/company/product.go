package company

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// A product owner gathers signals (what users ask for, what others ship,
// what breaks), proposes briefs that cite a source for every claim and
// predict what will change, and checks those predictions 30 and 90 days
// after a brief ships.

const productSchema = `
CREATE TABLE IF NOT EXISTS company_signals (
  id         TEXT PRIMARY KEY,
  company    TEXT NOT NULL,
  data       TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS company_signals_company ON company_signals (company, created_at);
CREATE TABLE IF NOT EXISTS company_briefs (
  id         TEXT PRIMARY KEY,
  company    TEXT NOT NULL,
  data       TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS company_briefs_company ON company_briefs (company, created_at);
CREATE TABLE IF NOT EXISTS company_proposals (
  id         TEXT PRIMARY KEY,
  company    TEXT NOT NULL,
  data       TEXT NOT NULL,
  created_at TEXT NOT NULL
);`

// A Signal is one thing heard about the product. The same thing heard
// again counts on the first one.
type Signal struct {
	ID      string    `json:"id"`
	Company string    `json:"company"`
	Source  string    `json:"source"`
	Title   string    `json:"title"`
	URL     string    `json:"url,omitempty"`
	Text    string    `json:"text,omitempty"`
	By      string    `json:"by"`
	Count   int       `json:"count"`
	Seen    []string  `json:"seen,omitempty"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// A Claim is something a brief says, with where it comes from and the
// words there that support it.
type Claim struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	Quote  string `json:"quote,omitempty"`
	// Flag says why Jev doubts the source supports the claim.
	Flag string `json:"flag,omitempty"`
}

// Scores go from 1 to 5.
type Scores struct {
	Value           int `json:"value"`
	Differentiation int `json:"differentiation"`
	Adoption        int `json:"adoption"`
	BuildRisk       int `json:"build_risk"`
	SafetyRisk      int `json:"safety_risk"`
}

// Score is the roadmap's formula.
func (s Scores) Score() int {
	return 2*s.Value + s.Differentiation + 2*s.Adoption - s.BuildRisk - s.SafetyRisk
}

func (s Scores) check() error {
	for _, v := range []int{s.Value, s.Differentiation, s.Adoption, s.BuildRisk, s.SafetyRisk} {
		if v < 1 || v > 5 {
			return errors.New("each score goes from 1 to 5")
		}
	}
	return nil
}

// A Prediction is what a brief says will change once it ships.
type Prediction struct {
	Metric   string `json:"metric"`
	Expected string `json:"expected"`
}

// A Review checks a shipped brief's predictions after some days.
type Review struct {
	Day     int       `json:"day"`
	Results []Result  `json:"results"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
}

type Result struct {
	Metric string `json:"metric"`
	Actual string `json:"actual"`
	Met    bool   `json:"met"`
}

// States of a brief.
const (
	BriefProposed = "proposed"
	BriefAccepted = "accepted"
	BriefRejected = "rejected"
	BriefShipped  = "shipped"
)

// ReviewDays are when a shipped brief is checked.
var ReviewDays = []int{30, 90}

type Brief struct {
	ID          string       `json:"id"`
	Company     string       `json:"company"`
	Author      string       `json:"author"`
	Title       string       `json:"title"`
	Problem     string       `json:"problem"`
	Proposal    string       `json:"proposal"`
	Claims      []Claim      `json:"claims"`
	Scores      Scores       `json:"scores"`
	Score       int          `json:"score"`
	Predictions []Prediction `json:"predictions"`
	Signals     []string     `json:"signals,omitempty"`
	State       string       `json:"state"`
	Ref         string       `json:"ref,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	Shipped     time.Time    `json:"shipped,omitzero"`
	Reviews     []Review     `json:"reviews,omitempty"`
	// Checks are the pieces of work asked to review it, by day.
	Checks  map[string]string `json:"checks,omitempty"`
	Created time.Time         `json:"created"`
	Updated time.Time         `json:"updated"`
}

// Check says what a brief misses before it is kept.
func (b Brief) Check() error {
	switch {
	case strings.TrimSpace(b.Title) == "" || strings.TrimSpace(b.Problem) == "" || strings.TrimSpace(b.Proposal) == "":
		return errors.New("a brief has a title, the problem and the proposal")
	case len(b.Claims) == 0:
		return errors.New("a brief makes its case with claims, each with its source")
	case len(b.Predictions) == 0:
		return errors.New("a brief predicts what will change once it ships, so it can be checked")
	case len(b.Claims) > 20 || len(b.Predictions) > 10:
		return errors.New("at most 20 claims and 10 predictions")
	}
	for _, c := range b.Claims {
		if strings.TrimSpace(c.Text) == "" || strings.TrimSpace(c.Source) == "" {
			return fmt.Errorf("every claim cites its source: %q has none", c.Text)
		}
	}
	for _, p := range b.Predictions {
		if strings.TrimSpace(p.Metric) == "" || strings.TrimSpace(p.Expected) == "" {
			return errors.New("each prediction names a metric and what is expected of it")
		}
	}
	return b.Scores.check()
}

// Due is the first review a shipped brief is owed at now, if any.
func (b Brief) Due(now time.Time) (int, bool) {
	if b.State != BriefShipped || b.Shipped.IsZero() {
		return 0, false
	}
	for _, d := range ReviewDays {
		if now.Before(b.Shipped.AddDate(0, 0, d)) {
			return 0, false
		}
		if !slices.ContainsFunc(b.Reviews, func(r Review) bool { return r.Day == d }) {
			return d, true
		}
	}
	return 0, false
}

// Accuracy is how many of an author's checked predictions came true.
func Accuracy(briefs []Brief, author string) (met, checked int) {
	for _, b := range briefs {
		if b.Author != author {
			continue
		}
		for _, r := range b.Reviews {
			for _, x := range r.Results {
				checked++
				if x.Met {
					met++
				}
			}
		}
	}
	return met, checked
}

// sameSignal says whether two signals tell the same thing: the same page,
// or titles with most words in common.
func sameSignal(a, b Signal) bool {
	if a.URL != "" && b.URL != "" {
		return canonical(a.URL) == canonical(b.URL)
	}
	wa, wb := words(a.Title), words(b.Title)
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	common := 0
	for w := range wa {
		if wb[w] {
			common++
		}
	}
	return float64(common)/float64(len(wa)+len(wb)-common) >= 0.7
}

func canonical(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.ToLower(raw)
	}
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(k, "utm_") || k == "ref" || k == "fbclid" || k == "gclid" {
			q.Del(k)
		}
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	path := strings.TrimSuffix(u.Path, "/")
	out := host + path
	if enc := q.Encode(); enc != "" {
		out += "?" + enc
	}
	return out
}

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(fold(s)), func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
		if len(w) > 2 {
			out[w] = true
		}
	}
	return out
}

// AddSignal keeps a signal, or counts it on the one it repeats.
func (s *Store) AddSignal(ctx context.Context, sig Signal) (Signal, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.Signals(ctx, sig.Company, 500)
	if err != nil {
		return sig, false, err
	}
	now := s.now()
	for _, old := range all {
		if sameSignal(old, sig) {
			old.Count++
			if !slices.Contains(old.Seen, sig.Source) && sig.Source != old.Source && len(old.Seen) < 20 {
				old.Seen = append(old.Seen, sig.Source)
			}
			old.Updated = now
			return old, true, s.saveRow(ctx, "company_signals", old.ID, old.Company, old, old.Created)
		}
	}
	sig.Count, sig.Created, sig.Updated = 1, now, now
	return sig, false, s.saveRow(ctx, "company_signals", sig.ID, sig.Company, sig, now)
}

func (s *Store) Signals(ctx context.Context, company string, limit int) ([]Signal, error) {
	return rows[Signal](ctx, s, `SELECT data FROM company_signals WHERE company = ? ORDER BY created_at DESC LIMIT ?`, company, limit)
}

func (s *Store) SaveBrief(ctx context.Context, b Brief) error {
	return s.saveRow(ctx, "company_briefs", b.ID, b.Company, b, b.Created)
}

func (s *Store) Briefs(ctx context.Context, company string) ([]Brief, error) {
	return rows[Brief](ctx, s, `SELECT data FROM company_briefs WHERE company = ? ORDER BY created_at DESC`, company)
}

// AllBriefs are every company's shipped briefs, for the reviews they owe.
func (s *Store) AllBriefs(ctx context.Context) ([]Brief, error) {
	return rows[Brief](ctx, s, `SELECT data FROM company_briefs ORDER BY created_at`)
}

func (s *Store) Brief(ctx context.Context, id string) (Brief, error) {
	out, err := rows[Brief](ctx, s, `SELECT data FROM company_briefs WHERE id = ?`, id)
	if err != nil {
		return Brief{}, err
	}
	if len(out) == 0 {
		return Brief{}, ErrNotFound
	}
	return out[0], nil
}

// UpdateBrief changes a brief under the store's lock.
func (s *Store) UpdateBrief(ctx context.Context, id string, f func(*Brief) error) (Brief, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.Brief(ctx, id)
	if err != nil {
		return b, err
	}
	if err := f(&b); err != nil {
		return b, err
	}
	b.Updated = s.now()
	return b, s.SaveBrief(ctx, b)
}

func (s *Store) saveRow(ctx context.Context, table, id, company string, v any, created time.Time) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO `+table+` (id, company, data, created_at) VALUES (?, ?, ?, ?)
	  ON CONFLICT(id) DO UPDATE SET data = excluded.data`, id, company, string(b), created.UTC().Format(time.RFC3339Nano))
	return err
}

func rows[T any](ctx context.Context, s *Store, q string, args ...any) ([]T, error) {
	r, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out := []T{}
	for r.Next() {
		var data string
		var v T
		if err := r.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, r.Err()
}

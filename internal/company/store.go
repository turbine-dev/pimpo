package company

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Each row keeps its thing as JSON, so a field added later needs no new
// column; the columns are what is looked up.
const schema = `
CREATE TABLE IF NOT EXISTS companies (
  id         TEXT PRIMARY KEY,
  person     TEXT NOT NULL DEFAULT '',
  data       TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS company_parts (
  company TEXT NOT NULL,
  kind    TEXT NOT NULL,
  id      TEXT NOT NULL,
  data    TEXT NOT NULL,
  PRIMARY KEY (company, kind, id)
);`

const (
	partDepartment = "department"
	partRole       = "role"
	partMember     = "member"
	partContext    = "context"
	partRule       = "rule"
	partAgent      = "agent_routine"
)

type Store struct {
	DB  *sql.DB
	Now func() time.Time
	// mu keeps one change at a time: each reads the company, changes it
	// and writes it back whole.
	mu sync.Mutex
}

// Open makes the company tables in Pimpo's database, if they are not there.
func Open(db *sql.DB) (*Store, error) {
	for _, ddl := range []string{schema, workSchema, teamSchema, memorySchema, decisionSchema, productSchema, earnSchema, mediaSchema} {
		if _, err := db.Exec(ddl); err != nil {
			return nil, fmt.Errorf("company tables: %w", err)
		}
	}
	return &Store{DB: db}, nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Create saves a new company with its CEO seat, the person ceoName names.
func (s *Store) Create(ctx context.Context, c Company, ceoName string) (Org, error) {
	now := s.now()
	c.Created, c.Updated = now, now
	o := Org{Company: c, Members: []Member{{ID: CEO, Kind: Person, Person: c.Person, Title: "CEO", Name: ceoName, Created: now, Updated: now}}}
	if o.Members[0].Name == "" {
		o.Members[0].Name = "CEO"
	}
	if err := o.Check(); err != nil {
		return Org{}, err
	}
	return o, s.write(ctx, func(tx *sql.Tx) error {
		if err := insertCompany(ctx, tx, c); err != nil {
			return err
		}
		return putPart(ctx, tx, c.ID, partMember, CEO, o.Members[0])
	})
}

// Org is a company with everything in it.
func (s *Store) Org(ctx context.Context, id string) (Org, error) {
	var data string
	err := s.DB.QueryRowContext(ctx, `SELECT data FROM companies WHERE id = ?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return Org{}, ErrNotFound
	}
	if err != nil {
		return Org{}, err
	}
	o := Org{Departments: []Department{}, Roles: []Role{}, Members: []Member{}, Contexts: []Context{}, Rules: []Rule{}, AgentRoutines: []AgentRoutine{}}
	if err := json.Unmarshal([]byte(data), &o.Company); err != nil {
		return Org{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT kind, data FROM company_parts WHERE company = ? ORDER BY rowid`, id)
	if err != nil {
		return Org{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, part string
		if err := rows.Scan(&kind, &part); err != nil {
			return Org{}, err
		}
		switch kind {
		case partDepartment:
			var d Department
			err = json.Unmarshal([]byte(part), &d)
			o.Departments = append(o.Departments, d)
		case partRole:
			var r Role
			err = json.Unmarshal([]byte(part), &r)
			o.Roles = append(o.Roles, r)
		case partMember:
			var m Member
			err = json.Unmarshal([]byte(part), &m)
			o.Members = append(o.Members, m)
		case partContext:
			var c Context
			err = json.Unmarshal([]byte(part), &c)
			o.Contexts = append(o.Contexts, c)
		case partRule:
			var r Rule
			err = json.Unmarshal([]byte(part), &r)
			o.Rules = append(o.Rules, r)
		case partAgent:
			var r AgentRoutine
			err = json.Unmarshal([]byte(part), &r)
			o.AgentRoutines = append(o.AgentRoutines, r)
		}
		if err != nil {
			return Org{}, err
		}
	}
	return o, rows.Err()
}

// List is every company, newest change first; callers keep the ones a
// person may see.
func (s *Store) List(ctx context.Context) ([]Company, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM companies ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Company{}
	for rows.Next() {
		var data string
		var c Company
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Update changes a company's own fields and partners. Its person never
// changes.
func (s *Store) Update(ctx context.Context, c Company) (Org, error) {
	return s.change(ctx, c.ID, func(o *Org) error {
		c.Person, c.Created = o.Person, o.Created
		o.Company = c
		return nil
	})
}

func (s *Store) SaveDepartment(ctx context.Context, company string, d Department) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		o.Departments = upsert(o.Departments, d, func(x Department) string { return x.ID })
		return nil
	})
}

// DeleteDepartment takes its members out of it; they stay in the company.
func (s *Store) DeleteDepartment(ctx context.Context, company, id string) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		if _, ok := o.Department(id); !ok {
			return ErrNotFound
		}
		o.Departments = remove(o.Departments, id, func(x Department) string { return x.ID })
		o.dropScoped(ScopeDepartment, id)
		for i := range o.Members {
			if o.Members[i].Department == id {
				o.Members[i].Department = ""
			}
		}
		return nil
	})
}

func (s *Store) SaveRole(ctx context.Context, company string, r Role) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		o.Roles = upsert(o.Roles, r, func(x Role) string { return x.ID })
		return nil
	})
}

// DeleteRole refuses while a member holds the role.
func (s *Store) DeleteRole(ctx context.Context, company, id string) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		if _, ok := o.Role(id); !ok {
			return ErrNotFound
		}
		for _, m := range o.Members {
			if m.Role == id {
				return fmt.Errorf("%s holds this role; give them another first", m.Name)
			}
		}
		o.Roles = remove(o.Roles, id, func(x Role) string { return x.ID })
		o.dropScoped(ScopeRole, id)
		return nil
	})
}

// SaveMember hires or changes a member. The CEO seat keeps its person and
// stays at the top.
func (s *Store) SaveMember(ctx context.Context, company string, m Member) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		now := s.now()
		if old, ok := o.Member(m.ID); ok {
			m.Created = old.Created
			if m.ID == CEO {
				m.Kind, m.Person, m.ReportsTo = Person, old.Person, ""
			}
		} else {
			m.Created = now
		}
		m.Updated = now
		o.Members = upsert(o.Members, m, func(x Member) string { return x.ID })
		return nil
	})
}

// DeleteMember lets a member go; whoever reported to them reports to
// their boss. The CEO seat cannot go.
func (s *Store) DeleteMember(ctx context.Context, company, id string) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		m, ok := o.Member(id)
		if !ok {
			return ErrNotFound
		}
		if id == CEO {
			return errors.New("the CEO seat stays")
		}
		for i := range o.Members {
			if o.Members[i].ReportsTo == id {
				o.Members[i].ReportsTo = m.ReportsTo
			}
		}
		o.Members = remove(o.Members, id, func(x Member) string { return x.ID })
		o.dropScoped(ScopeMember, id)
		o.AgentRoutines = slices.DeleteFunc(o.AgentRoutines, func(r AgentRoutine) bool { return r.Member == id })
		return nil
	})
}

func (s *Store) SaveContext(ctx context.Context, company string, c Context) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		o.putContext(c, s.now())
		return nil
	})
}

func (s *Store) DeleteContext(ctx context.Context, company, id string) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		if _, ok := o.Context(id); !ok {
			return ErrNotFound
		}
		o.Contexts = remove(o.Contexts, id, func(x Context) string { return x.ID })
		return nil
	})
}

// SaveRule writes a rule; one that allows what a broader rule forbids
// must say it is an exception (ErrException otherwise).
func (s *Store) SaveRule(ctx context.Context, company string, r Rule) (Org, error) {
	return s.change(ctx, company, func(o *Org) error { return o.putRule(r) })
}

func (s *Store) DeleteRule(ctx context.Context, company, id string) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		if !slices.ContainsFunc(o.Rules, func(r Rule) bool { return r.ID == id }) {
			return ErrNotFound
		}
		o.Rules = remove(o.Rules, id, func(x Rule) string { return x.ID })
		o.markExceptions()
		return nil
	})
}

func (s *Store) SaveAgentRoutine(ctx context.Context, company string, r AgentRoutine) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		o.AgentRoutines = upsert(o.AgentRoutines, r, func(x AgentRoutine) string { return x.ID })
		return nil
	})
}

func (s *Store) DeleteAgentRoutine(ctx context.Context, company, id string) (Org, error) {
	return s.change(ctx, company, func(o *Org) error {
		if !slices.ContainsFunc(o.AgentRoutines, func(r AgentRoutine) bool { return r.ID == id }) {
			return ErrNotFound
		}
		o.AgentRoutines = remove(o.AgentRoutines, id, func(x AgentRoutine) string { return x.ID })
		return nil
	})
}

func (s *Store) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{`DELETE FROM company_parts WHERE company = ?`, `DELETE FROM company_work WHERE company = ?`, `DELETE FROM company_tasks WHERE company = ?`, `DELETE FROM company_questions WHERE company = ?`, `DELETE FROM company_notes WHERE company = ?`, `DELETE FROM company_meetings WHERE company = ?`, `DELETE FROM company_decisions WHERE company = ?`, `DELETE FROM company_signals WHERE company = ?`, `DELETE FROM company_briefs WHERE company = ?`, `DELETE FROM company_streaks WHERE company = ?`, `DELETE FROM company_proposals WHERE company = ?`, `DELETE FROM company_media WHERE company = ?`} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM companies WHERE id = ?`, id)
		return err
	})
}

// change loads a company, applies f, checks the result and saves it whole,
// so a change that would break the tree is never written.
func (s *Store) change(ctx context.Context, id string, f func(*Org) error) (Org, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, err := s.Org(ctx, id)
	if err != nil {
		return Org{}, err
	}
	if err := f(&o); err != nil {
		return Org{}, err
	}
	if err := o.Check(); err != nil {
		return Org{}, err
	}
	o.Updated = s.now()
	return o, s.write(ctx, func(tx *sql.Tx) error { return replace(ctx, tx, o) })
}

// Replace writes a whole company as it is, for imports.
func (s *Store) Replace(ctx context.Context, o Org) error {
	if err := o.Check(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(ctx, func(tx *sql.Tx) error { return replace(ctx, tx, o) })
}

func replace(ctx context.Context, tx *sql.Tx, o Org) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM companies WHERE id = ?`, o.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM company_parts WHERE company = ?`, o.ID); err != nil {
		return err
	}
	if err := insertCompany(ctx, tx, o.Company); err != nil {
		return err
	}
	for _, d := range o.Departments {
		if err := putPart(ctx, tx, o.ID, partDepartment, d.ID, d); err != nil {
			return err
		}
	}
	for _, r := range o.Roles {
		if err := putPart(ctx, tx, o.ID, partRole, r.ID, r); err != nil {
			return err
		}
	}
	for _, m := range o.Members {
		if err := putPart(ctx, tx, o.ID, partMember, m.ID, m); err != nil {
			return err
		}
	}
	for _, c := range o.Contexts {
		if err := putPart(ctx, tx, o.ID, partContext, c.ID, c); err != nil {
			return err
		}
	}
	for _, r := range o.Rules {
		if err := putPart(ctx, tx, o.ID, partRule, r.ID, r); err != nil {
			return err
		}
	}
	for _, r := range o.AgentRoutines {
		if err := putPart(ctx, tx, o.ID, partAgent, r.ID, r); err != nil {
			return err
		}
	}
	return nil
}

func insertCompany(ctx context.Context, tx *sql.Tx, c Company) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO companies (id, person, data, updated_at) VALUES (?, ?, ?, ?)`, c.ID, c.Person, string(b), c.Updated.Format(time.RFC3339Nano))
	return err
}

func putPart(ctx context.Context, tx *sql.Tx, company, kind, id string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO company_parts (company, kind, id, data) VALUES (?, ?, ?, ?)`, company, kind, id, string(b))
	return err
}

func (s *Store) write(ctx context.Context, f func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func upsert[T any](list []T, v T, id func(T) string) []T {
	for i := range list {
		if id(list[i]) == id(v) {
			list[i] = v
			return list
		}
	}
	return append(list, v)
}

func remove[T any](list []T, gone string, id func(T) string) []T {
	out := list[:0]
	for _, x := range list {
		if id(x) != gone {
			out = append(out, x)
		}
	}
	return out
}

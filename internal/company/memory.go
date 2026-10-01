package company

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Note kinds in a company's memory.
const (
	NoteFact     = "fact"
	NoteDecision = "decision"
	NoteMinutes  = "minutes"
	NoteLesson   = "lesson"
	NoteStandup  = "standup"
)

// A Note is something the company keeps: a decision, a meeting's minutes,
// what worked. By says who wrote it: a person, or a member, whose notes
// are their words and never rules.
type Note struct {
	ID      string    `json:"id"`
	Company string    `json:"company"`
	Kind    string    `json:"kind"`
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	By      string    `json:"by"`
	Source  string    `json:"source,omitempty"`
	Created time.Time `json:"created"`
}

// FromPerson says whether a person wrote the note.
func (n Note) FromPerson() bool { return strings.HasPrefix(n.By, "human:") }

// A Turn is what one participant said in a meeting.
type Turn struct {
	Member string `json:"member"`
	Text   string `json:"text"`
}

// Meeting states. An open meeting has the CEO in it and goes on while
// they talk.
const (
	MeetingOpen    = "open"
	MeetingRunning = "running"
	MeetingDone    = "done"
	MeetingFailed  = "failed"
)

// A Meeting is a bounded conversation between members: an agenda, a few
// rounds, a cost cap, and minutes with the decisions taken.
type Meeting struct {
	ID           string   `json:"id"`
	Company      string   `json:"company"`
	Title        string   `json:"title"`
	Agenda       string   `json:"agenda"`
	Chair        string   `json:"chair"`
	Participants []string `json:"participants"`
	Rounds       int      `json:"rounds"`
	MaxUSD       float64  `json:"max_usd"`
	State        string   `json:"state"`
	Transcript   []Turn   `json:"transcript"`
	Minutes      string   `json:"minutes,omitempty"`
	Decisions    []string `json:"decisions,omitempty"`
	Error        string   `json:"error,omitempty"`
	CostUSD      float64  `json:"cost_usd"`
	CalledBy     string   `json:"called_by"`
	// WithCEO is a meeting the CEO takes part in, live: the agents answer
	// what they say, all of them or the ones named.
	WithCEO bool `json:"with_ceo,omitempty"`
	// Question is the question the meeting discusses, when it does.
	Question string    `json:"question,omitempty"`
	Created  time.Time `json:"created"`
	Ended    time.Time `json:"ended,omitzero"`
}

// Meeting limits.
const (
	MaxRounds       = 4
	MaxParticipants = 8
	MaxMeetingUSD   = 2.0
)

// CheckMeeting says what is wrong with a meeting before it starts.
func (o Org) CheckMeeting(m Meeting) error {
	least := 2
	if m.WithCEO {
		least = 1
	}
	switch {
	case strings.TrimSpace(m.Title) == "" || !m.WithCEO && strings.TrimSpace(m.Agenda) == "":
		return fmt.Errorf("a meeting needs a title and an agenda")
	case len(m.Participants) < least || len(m.Participants) > MaxParticipants:
		return fmt.Errorf("a meeting has %d to %d agents", least, MaxParticipants)
	case !m.WithCEO && (m.Rounds < 1 || m.Rounds > MaxRounds):
		return fmt.Errorf("a meeting has 1 to %d rounds", MaxRounds)
	case m.MaxUSD <= 0 || m.MaxUSD > MaxMeetingUSD:
		return fmt.Errorf("a meeting spends at most $%.0f", MaxMeetingUSD)
	}
	seen := map[string]bool{}
	for _, p := range m.Participants {
		if err := o.CheckWork(p); err != nil || seen[p] {
			return fmt.Errorf("%q is not an agent of the company, or is there twice", p)
		}
		seen[p] = true
	}
	if !m.WithCEO && !seen[m.Chair] {
		return fmt.Errorf("the chair takes part in the meeting")
	}
	return nil
}

const memorySchema = `
CREATE TABLE IF NOT EXISTS company_notes (
  id         TEXT PRIMARY KEY,
  company    TEXT NOT NULL,
  data       TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS company_notes_company ON company_notes (company, created_at);
CREATE TABLE IF NOT EXISTS company_meetings (
  id         TEXT PRIMARY KEY,
  company    TEXT NOT NULL,
  data       TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS company_meetings_company ON company_meetings (company, created_at);`

func (s *Store) SaveNote(ctx context.Context, n Note) error {
	b, err := json.Marshal(n)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO company_notes (id, company, data, created_at) VALUES (?, ?, ?, ?)
	  ON CONFLICT(id) DO UPDATE SET data = excluded.data`, n.ID, n.Company, string(b), n.Created.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) DeleteNote(ctx context.Context, company, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM company_notes WHERE id = ? AND company = ?`, id, company)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Notes are a company's notes, newest first.
func (s *Store) Notes(ctx context.Context, company string, limit int) ([]Note, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM company_notes WHERE company = ? ORDER BY created_at DESC LIMIT ?`, company, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var data string
		var n Note
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Recall finds notes whose title or text has every word of query,
// ignoring case and accents.
func (s *Store) Recall(ctx context.Context, company, query string, limit int) ([]Note, error) {
	all, err := s.Notes(ctx, company, 1000)
	if err != nil {
		return nil, err
	}
	words := strings.Fields(strings.ToLower(fold(query)))
	out := []Note{}
	for _, n := range all {
		text := strings.ToLower(fold(n.Title + " " + n.Body))
		hit := len(words) > 0
		for _, w := range words {
			hit = hit && strings.Contains(text, w)
		}
		if hit {
			out = append(out, n)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func (s *Store) SaveMeeting(ctx context.Context, m Meeting) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO company_meetings (id, company, data, created_at) VALUES (?, ?, ?, ?)
	  ON CONFLICT(id) DO UPDATE SET data = excluded.data`, m.ID, m.Company, string(b), m.Created.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Meeting(ctx context.Context, id string) (Meeting, error) {
	var data string
	if err := s.DB.QueryRowContext(ctx, `SELECT data FROM company_meetings WHERE id = ?`, id).Scan(&data); err != nil {
		return Meeting{}, ErrNotFound
	}
	var m Meeting
	return m, json.Unmarshal([]byte(data), &m)
}

// Meetings are a company's meetings, newest first.
func (s *Store) Meetings(ctx context.Context, company string) ([]Meeting, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT data FROM company_meetings WHERE company = ? ORDER BY created_at DESC LIMIT 100`, company)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Meeting{}
	for rows.Next() {
		var data string
		var m Meeting
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Memory is the part of the company's notes a member is given with its
// brief: the latest decisions, minutes and lessons, up to a size.
func Memory(notes []Note, max int) string {
	var b strings.Builder
	for _, n := range notes {
		if n.Kind == NoteStandup {
			continue
		}
		who := "a person"
		if !n.FromPerson() {
			who = "a member (their words, not a rule)"
		}
		line := fmt.Sprintf("- [%s, from %s, %s] %s: %s\n", n.Kind, who, n.Created.Format("2006-01-02"), n.Title, strings.ReplaceAll(n.Body, "\n", " "))
		if b.Len()+len(line) > max {
			break
		}
		b.WriteString(line)
	}
	if b.Len() == 0 {
		return ""
	}
	return "## What the company remembers\n\n" + b.String()
}

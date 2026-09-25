// Package memory is what Zodim knows about its owner: readable Markdown
// files, one per topic, versioned with git so any change can be seen and
// undone. Every fact records where it came from and how far to trust it;
// only facts from the owner ever reach an agent as instructions.
package memory

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Trust string

const (
	// High is what the owner said or confirmed.
	High Trust = "high"
	// Low is what the agent read somewhere: an email, a web page.
	Low Trust = "low"
)

type Fact struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Topic  string `json:"topic"`
	Source string `json:"source"`
	Trust  Trust  `json:"trust"`
	// Person the fact is about and who may read it; empty is the owner,
	// "casa" is everyone in the house.
	Person  string    `json:"person,omitempty"`
	Created time.Time `json:"created"`
}

type Version struct {
	Hash    string    `json:"hash"`
	Message string    `json:"message"`
	When    time.Time `json:"when"`
}

type Memory struct {
	Dir string

	mu   sync.Mutex
	repo *git.Repository
}

const factsFile = "facts.json"

func Open(dir string) (*Memory, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	repo, err := git.PlainOpen(dir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		repo, err = git.PlainInit(dir, false)
	}
	if err != nil {
		return nil, fmt.Errorf("memory repository: %w", err)
	}
	return &Memory{Dir: dir, repo: repo}, nil
}

func (m *Memory) load() ([]Fact, error) {
	b, err := os.ReadFile(filepath.Join(m.Dir, factsFile))
	if errors.Is(err, os.ErrNotExist) {
		return []Fact{}, nil
	}
	if err != nil {
		return nil, err
	}
	var facts []Fact
	if err := json.Unmarshal(b, &facts); err != nil {
		return nil, err
	}
	return facts, nil
}

// save writes the facts and one readable Markdown file per topic, then
// commits the change.
func (m *Memory) save(facts []Fact, message string) error {
	b, _ := json.MarshalIndent(facts, "", "  ")
	if err := os.WriteFile(filepath.Join(m.Dir, factsFile), b, 0o600); err != nil {
		return err
	}
	byTopic := map[string][]Fact{}
	for _, f := range facts {
		byTopic[f.Topic] = append(byTopic[f.Topic], f)
	}
	entries, _ := os.ReadDir(m.Dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			os.Remove(filepath.Join(m.Dir, e.Name()))
		}
	}
	for topic, fs := range byTopic {
		var md strings.Builder
		r := []rune(topic)
		fmt.Fprintf(&md, "# %s\n\n", strings.ToUpper(string(r[:1]))+string(r[1:]))
		for _, f := range fs {
			mark := ""
			if f.Trust == Low {
				mark = " _(não confirmado)_"
			}
			fmt.Fprintf(&md, "- %s%s  \n  <sub>%s · %s</sub>\n", f.Text, mark, f.Source, f.Created.Format("02/01/2006"))
		}
		if err := os.WriteFile(filepath.Join(m.Dir, slug(topic)+".md"), []byte(md.String()), 0o600); err != nil {
			return err
		}
	}
	wt, err := m.repo.Worktree()
	if err != nil {
		return err
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return err
	}
	_, err = wt.Commit(message, &git.CommitOptions{AllowEmptyCommits: false, Author: &object.Signature{Name: "Zodim", Email: "zodim@localhost", When: time.Now()}})
	if errors.Is(err, git.ErrEmptyCommit) {
		return nil
	}
	return err
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		default:
			if r > 127 {
				b.WriteRune(r)
			}
		}
	}
	if b.Len() == 0 {
		return "geral"
	}
	return b.String()
}

func newID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *Memory) List() ([]Fact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	facts, err := m.load()
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].Created.After(facts[j].Created) })
	return facts, err
}

// Add records a fact. Repeating an existing fact only refreshes it, and a
// low-trust copy never downgrades what the owner confirmed.
func (m *Memory) Add(text, topic, source string, trust Trust) (Fact, error) {
	return m.AddFor(text, topic, source, trust, "")
}

// AddFor records a fact kept for one person.
func (m *Memory) AddFor(text, topic, source string, trust Trust, person string) (Fact, error) {
	person = owned(person)
	// A fact is one line: a newline in it could forge entries in the
	// Markdown files people read.
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return Fact{}, errors.New("empty fact")
	}
	if topic = strings.Join(strings.Fields(topic), " "); topic == "" {
		topic = "geral"
	}
	if trust != High {
		trust = Low
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	facts, err := m.load()
	if err != nil {
		return Fact{}, err
	}
	for i, f := range facts {
		if f.Person == person && strings.EqualFold(f.Text, text) {
			if trust == High && f.Trust == Low {
				facts[i].Trust, facts[i].Source = High, source
				return facts[i], m.save(facts, "confirm: "+text)
			}
			return f, nil
		}
	}
	f := Fact{ID: newID(), Text: text, Topic: topic, Source: source, Trust: trust, Person: person, Created: time.Now()}
	facts = append(facts, f)
	return f, m.save(facts, "add: "+text)
}

func (m *Memory) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	facts, err := m.load()
	if err != nil {
		return err
	}
	for i, f := range facts {
		if f.ID == id {
			return m.save(append(facts[:i], facts[i+1:]...), "remove: "+f.Text)
		}
	}
	return fmt.Errorf("fact %s not found", id)
}

// Confirm marks a fact as the owner's own.
func (m *Memory) Confirm(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	facts, err := m.load()
	if err != nil {
		return err
	}
	for i := range facts {
		if facts[i].ID == id {
			facts[i].Trust = High
			facts[i].Source = "owner"
			return m.save(facts, "confirm: "+facts[i].Text)
		}
	}
	return fmt.Errorf("fact %s not found", id)
}

// owned stores the owner's facts with an empty person, as before people
// existed.
func owned(person string) string {
	if person == "owner" {
		return ""
	}
	return person
}

func visible(f Fact, reader string) bool {
	return f.Person == owned(reader) || f.Person == "casa"
}

// Instructions are the facts an agent may treat as the owner's word.
func (m *Memory) Instructions() ([]Fact, error) { return m.InstructionsFor("") }

// InstructionsFor are the confirmed facts a run for this person may follow.
func (m *Memory) InstructionsFor(person string) ([]Fact, error) {
	facts, err := m.List()
	var out []Fact
	for _, f := range facts {
		if f.Trust == High && visible(f, person) {
			out = append(out, f)
		}
	}
	return out, err
}

// Search returns the owner's facts containing every word of the query.
func (m *Memory) Search(query string) ([]Fact, error) { return m.SearchFor(query, "") }

// SearchFor searches only what this person may read.
func (m *Memory) SearchFor(query, person string) ([]Fact, error) {
	facts, err := m.List()
	words := strings.Fields(strings.ToLower(query))
	var out []Fact
	for _, f := range facts {
		if !visible(f, person) {
			continue
		}
		text := strings.ToLower(f.Text + " " + f.Topic)
		all := true
		for _, w := range words {
			all = all && strings.Contains(text, w)
		}
		if all {
			out = append(out, f)
		}
	}
	return out, err
}

func (m *Memory) History(limit int) ([]Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	head, err := m.repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return []Version{}, nil
	}
	if err != nil {
		return nil, err
	}
	iter, err := m.repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return nil, err
	}
	out := []Version{}
	iter.ForEach(func(c *object.Commit) error {
		if len(out) >= limit {
			return errors.New("stop")
		}
		out = append(out, Version{Hash: c.Hash.String(), Message: strings.TrimSpace(c.Message), When: c.Author.When})
		return nil
	})
	return out, nil
}

// Restore brings memory back to how it was at a version, as a new commit,
// so the restore itself can be undone.
func (m *Memory) Restore(hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, err := m.repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return fmt.Errorf("version %s not found", hash)
	}
	f, err := c.File(factsFile)
	var facts []Fact
	if err == nil {
		content, _ := f.Contents()
		json.Unmarshal([]byte(content), &facts)
	}
	if facts == nil {
		facts = []Fact{}
	}
	return m.save(facts, "restore to "+hash[:8]+" ("+c.Author.When.Format("02/01 15:04")+")")
}

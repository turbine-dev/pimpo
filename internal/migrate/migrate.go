// Package migrate reads an OpenClaw or Hermes home and turns what it finds
// into things Vigia understands: memories, proposed tasks, standing rules
// and a report of which skills can become routines.
//
// Nothing here writes to Vigia. Read returns a Plan; the caller shows it
// and applies it.
package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Plan struct {
	From     string   `json:"from"`
	Home     string   `json:"home"`
	Timezone string   `json:"timezone,omitempty"`
	Memories []Memory `json:"memories"`
	Tasks    []Task   `json:"tasks"`
	Rules    []Rule   `json:"rules"`
	Skills   []Skill  `json:"skills"`
	Telegram Telegram `json:"telegram"`
	Mail     Mail     `json:"mail"`
	Warnings []string `json:"warnings"`
}

type Memory struct {
	Text   string `json:"text"`
	Topic  string `json:"topic"`
	Source string `json:"source"`
}

// Task is a scheduled job from the other agent. It becomes a proposal the
// owner explores once; the exploration then compiles into a routine.
type Task struct {
	Name     string `json:"name"`
	Prompt   string `json:"prompt"`
	Schedule string `json:"schedule"`
	Timezone string `json:"timezone,omitempty"`
	Deliver  string `json:"deliver,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// Request is what Vigia's explorer is asked to do for this task.
func (t Task) Request() string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(t.Prompt))
	if t.Schedule != "" {
		b.WriteString("\n\nQuando: " + t.Schedule)
		if t.Timezone != "" {
			b.WriteString(" (" + t.Timezone + ")")
		}
	}
	if t.Deliver != "" {
		b.WriteString("\nEntregar em: " + t.Deliver)
	}
	return b.String()
}

type Rule struct {
	File string `json:"file"`
	Text string `json:"text"`
}

type Telegram struct {
	Token   string   `json:"-"`
	HasBot  bool     `json:"has_bot"`
	Allowed []string `json:"allowed,omitempty"`
}

type Mail struct {
	Address  string `json:"address,omitempty"`
	IMAP     string `json:"imap,omitempty"`
	SMTP     string `json:"smtp,omitempty"`
	Password string `json:"-"`
}

// Read loads the plan from an OpenClaw or Hermes home. from is "openclaw"
// or "hermes"; home defaults to that agent's usual directory.
func Read(from, home string) (Plan, error) {
	if home == "" {
		h, _ := os.UserHomeDir()
		switch from {
		case "openclaw":
			home = envOr("OPENCLAW_STATE_DIR", filepath.Join(h, ".openclaw"))
		case "hermes":
			home = envOr("HERMES_HOME", filepath.Join(h, ".hermes"))
		}
	}
	if _, err := os.Stat(home); err != nil {
		return Plan{}, fmt.Errorf("no %s data at %s", from, home)
	}
	p := Plan{From: from, Home: home, Memories: []Memory{}, Tasks: []Task{}, Rules: []Rule{}, Skills: []Skill{}, Warnings: []string{}}
	var err error
	switch from {
	case "openclaw":
		err = readOpenClaw(&p)
	case "hermes":
		err = readHermes(&p)
	default:
		return Plan{}, fmt.Errorf("unknown source %q (openclaw or hermes)", from)
	}
	sort.SliceStable(p.Skills, func(i, j int) bool { return p.Skills[i].Name < p.Skills[j].Name })
	return p, err
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// rules keeps a standing-instructions file whole: splitting it would lose
// the context each line relies on.
func (p *Plan) rule(dir, name string) {
	if text := strings.TrimSpace(readFile(filepath.Join(dir, name))); text != "" {
		p.Rules = append(p.Rules, Rule{File: name, Text: text})
	}
}

// markdownEntries splits a free-form Markdown memory into entries: list
// items and paragraphs. Headings become the topic of what follows.
func markdownEntries(text, topic string) []Memory {
	var out []Memory
	var para []string
	current := topic
	flush := func() {
		if s := strings.TrimSpace(strings.Join(para, " ")); s != "" {
			out = append(out, Memory{Text: s, Topic: current})
		}
		para = nil
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "" || strings.HasPrefix(t, "<!--"):
			flush()
		case strings.HasPrefix(t, "#"):
			flush()
			if h := strings.TrimSpace(strings.TrimLeft(t, "#")); h != "" {
				current = h
			}
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			flush()
			para = []string{t[2:]}
		default:
			para = append(para, t)
		}
	}
	flush()
	return out
}

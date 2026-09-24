package migrate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill is a skill from the other agent and what it would take to run it
// as a Vigia routine.
type Skill struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Path         string   `json:"path"`
	Capabilities []string `json:"capabilities"`
	Missing      []string `json:"missing"`
	Secrets      []string `json:"secrets,omitempty"`
	// Verdict is "works" when every need maps to a capability, "partial"
	// when some do, and "no" when the skill is really a program.
	Verdict string `json:"verdict"`
}

var (
	needs = []struct {
		pattern *regexp.Regexp
		caps    []string
	}{
		{regexp.MustCompile(`(?i)\b(gmail|e-?mails?|inbox|imap|mailbox|newsletters?)\b`), []string{"gmail.search", "gmail.label", "gmail.archive"}},
		{regexp.MustCompile(`(?i)\b(send (an? )?(e-?mail|reply)|smtp|draft)\b`), []string{"gmail.draft", "gmail.send"}},
		{regexp.MustCompile(`(?i)\b(calendar|agenda|events?|meetings?|ical)\b`), []string{"calendar.list"}},
		{regexp.MustCompile(`(?i)\b(telegram|notify|notification|remind(er)?s?)\b`), []string{"telegram.send"}},
		{regexp.MustCompile(`(?i)\b(https?://|rest api|json api|webhook|fetch|weather|rss)\b`), []string{"http.getJSON"}},
	}
	programs = []struct {
		pattern *regexp.Regexp
		label   string
	}{
		{regexp.MustCompile("(?i)(```(bash|sh|shell|zsh)|\\bterminal\\b|\\bshell command|\\bbash\\b)"), "roda comandos no terminal"},
		{regexp.MustCompile(`(?i)\b(browser|playwright|puppeteer|selenium|chrom(e|ium)|click|screenshot)\b`), "controla um navegador"},
		{regexp.MustCompile(`(?i)\b(python3?|node|npm|pip|uv run|docker|ssh|git clone)\b`), "executa programas"},
		{regexp.MustCompile(`(?i)\b(write (to )?(a )?files?|edit files?|filesystem|file system)\b`), "escreve arquivos"},
	}
)

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		OpenClaw struct {
			Requires struct {
				Bins    []string `yaml:"bins"`
				AnyBins []string `yaml:"anyBins"`
				Env     []string `yaml:"env"`
			} `yaml:"requires"`
		} `yaml:"openclaw"`
	} `yaml:"metadata"`
	RequiredEnv []struct {
		Name string `yaml:"name"`
	} `yaml:"required_environment_variables"`
}

func findSkills(root string) ([]Skill, error) {
	var out []Skill
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
			return filepath.SkipDir
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, analyze(filepath.Dir(path), string(b)))
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return out, err
}

func analyze(dir, text string) Skill {
	var fm frontmatter
	body := text
	if rest, ok := strings.CutPrefix(text, "---\n"); ok {
		if head, tail, ok := strings.Cut(rest, "\n---"); ok {
			yaml.Unmarshal([]byte(head), &fm)
			body = tail
		}
	}
	s := Skill{Name: fm.Name, Description: fm.Description, Path: dir, Capabilities: []string{}, Missing: []string{}}
	if s.Name == "" {
		s.Name = filepath.Base(dir)
	}
	about := s.Description + "\n" + body

	caps := map[string]bool{}
	for _, n := range needs {
		if n.pattern.MatchString(about) {
			for _, c := range n.caps {
				caps[c] = true
			}
		}
	}
	for c := range caps {
		s.Capabilities = append(s.Capabilities, c)
	}
	sort.Strings(s.Capabilities)

	missing := map[string]bool{}
	for _, p := range programs {
		if p.pattern.MatchString(about) {
			missing[p.label] = true
		}
	}
	req := fm.Metadata.OpenClaw.Requires
	if len(req.Bins)+len(req.AnyBins) > 0 {
		missing["precisa de "+strings.Join(append(req.Bins, req.AnyBins...), ", ")] = true
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "scripts")); err == nil && len(entries) > 0 {
		missing["traz scripts próprios"] = true
	}
	for m := range missing {
		s.Missing = append(s.Missing, m)
	}
	sort.Strings(s.Missing)

	s.Secrets = append(s.Secrets, req.Env...)
	for _, e := range fm.RequiredEnv {
		s.Secrets = append(s.Secrets, e.Name)
	}

	switch {
	case len(s.Missing) == 0 && len(s.Capabilities) > 0:
		s.Verdict = "works"
	case len(s.Capabilities) > 0:
		s.Verdict = "partial"
	default:
		s.Verdict = "no"
	}
	return s
}

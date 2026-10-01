package company

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/turbine-dev/pimpo/internal/policy"
	"gopkg.in/yaml.v3"
)

// FileFormat is the version of company.yaml this Pimpo writes. A newer one
// is refused.
const FileFormat = 1

// A File is a company as it travels: what it is, without whose it is.
// People's seats are written without their person; importing gives every
// seat to whoever imports.
type File struct {
	Format        int               `yaml:"format"`
	Name          string            `yaml:"name"`
	Industry      string            `yaml:"industry,omitempty"`
	Mission       string            `yaml:"mission,omitempty"`
	Zone          string            `yaml:"zone,omitempty"`
	Hours         Hours             `yaml:"hours,omitempty"`
	Decider       Decider           `yaml:"decider,omitempty"`
	Levels        Levels            `yaml:"levels,omitempty"`
	Budget        Budget            `yaml:"budget,omitempty"`
	Memory        MemoryPolicy      `yaml:"memory,omitempty"`
	CodeEnv       map[string]string `yaml:"code_env,omitempty"`
	Departments   []Department      `yaml:"departments,omitempty"`
	Roles         []Role            `yaml:"roles,omitempty"`
	Members       []FileMember      `yaml:"members"`
	Contexts      []FileContext     `yaml:"contexts,omitempty"`
	Rules         []FileRule        `yaml:"rules,omitempty"`
	AgentRoutines []AgentRoutine    `yaml:"agent_routines,omitempty"`
}

type FileContext struct {
	ID    string `yaml:"id"`
	Scope string `yaml:"scope"`
	Of    string `yaml:"of,omitempty"`
	Title string `yaml:"title"`
	Body  string `yaml:"body"`
}

// A FileRule keeps its conditions with the same names as the house's rules
// in JSON.
type FileRule struct {
	ID    string         `yaml:"id"`
	Scope string         `yaml:"scope"`
	Of    string         `yaml:"of,omitempty"`
	Text  string         `yaml:"text"`
	When  map[string]any `yaml:"when,omitempty"`
	Then  policy.Verdict `yaml:"then"`
	Off   bool           `yaml:"off,omitempty"`
}

type FileMember struct {
	ID           string     `yaml:"id"`
	Kind         string     `yaml:"kind,omitempty"`
	Title        string     `yaml:"title,omitempty"`
	Role         string     `yaml:"role,omitempty"`
	Department   string     `yaml:"department,omitempty"`
	ReportsTo    string     `yaml:"reports_to,omitempty"`
	Name         string     `yaml:"name"`
	Avatar       string     `yaml:"avatar,omitempty"`
	Persona      string     `yaml:"persona,omitempty"`
	Capabilities []string   `yaml:"capabilities,omitempty"`
	Models       []string   `yaml:"models,omitempty"`
	Autonomy     []Autonomy `yaml:"autonomy,omitempty"`
	Budget       Budget     `yaml:"budget,omitempty"`
	Coder        string     `yaml:"coder,omitempty"`
	CodeSandbox  bool       `yaml:"code_sandbox,omitempty"`
}

// Export writes the company as a file.
func (o Org) Export() ([]byte, error) {
	f := File{Format: FileFormat, Name: o.Name, Industry: o.Industry, Mission: o.Mission, Zone: o.Zone, Hours: o.Hours, Decider: o.Decider, Levels: o.Levels, Budget: o.Budget, Memory: o.Memory, CodeEnv: o.CodeEnv, Departments: o.Departments, Roles: o.Roles, AgentRoutines: o.AgentRoutines}
	for _, m := range o.Members {
		f.Members = append(f.Members, FileMember{ID: m.ID, Kind: m.Kind, Title: m.Title, Role: m.Role, Department: m.Department, ReportsTo: m.ReportsTo,
			Name: m.Name, Avatar: m.Avatar, Persona: m.Persona, Capabilities: m.Capabilities, Models: m.Models, Autonomy: m.Autonomy, Budget: m.Budget, Coder: m.Coder, CodeSandbox: m.CodeSandbox})
	}
	for _, c := range o.Contexts {
		f.Contexts = append(f.Contexts, FileContext{ID: c.ID, Scope: c.Scope, Of: c.Of, Title: c.Title, Body: c.Body})
	}
	for _, r := range o.Rules {
		var when map[string]any
		b, _ := json.Marshal(r.When)
		json.Unmarshal(b, &when)
		f.Rules = append(f.Rules, FileRule{ID: r.ID, Scope: r.Scope, Of: r.Of, Text: r.Text, When: when, Then: r.Then, Off: r.Off})
	}
	return yaml.Marshal(f)
}

// Import reads a company file into a new company of person, with id. The
// result is checked like any change.
func Import(b []byte, id, person, personName string, now time.Time) (Org, error) {
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return Org{}, fmt.Errorf("read the company file: %w", err)
	}
	switch {
	case f.Format > FileFormat:
		return Org{}, fmt.Errorf("the company file is format %d, newer than this Pimpo reads (%d)", f.Format, FileFormat)
	case f.Format < 1:
		return Org{}, errors.New("this is not a company file")
	}
	o := Org{Company: Company{ID: id, Person: person, Name: f.Name, Industry: f.Industry, Mission: f.Mission, Zone: f.Zone, Hours: f.Hours, Decider: f.Decider, Levels: f.Levels, Budget: f.Budget, Memory: f.Memory, CodeEnv: f.CodeEnv, Created: now, Updated: now},
		Departments: f.Departments, Roles: f.Roles, AgentRoutines: f.AgentRoutines}
	if o.Departments == nil {
		o.Departments = []Department{}
	}
	if o.Roles == nil {
		o.Roles = []Role{}
	}
	for _, fm := range f.Members {
		m := Member{ID: fm.ID, Kind: fm.Kind, Title: fm.Title, Role: fm.Role, Department: fm.Department, ReportsTo: fm.ReportsTo,
			Name: fm.Name, Avatar: fm.Avatar, Persona: fm.Persona, Capabilities: fm.Capabilities, Models: fm.Models, Autonomy: fm.Autonomy, Budget: fm.Budget, Coder: fm.Coder, CodeSandbox: fm.CodeSandbox, State: Active, Created: now, Updated: now}
		if m.Kind == "" {
			m.Kind = Agent
		}
		if m.Kind == Person {
			m.Person = person
			if m.ID == CEO && personName != "" {
				m.Name = personName
			}
		}
		o.Members = append(o.Members, m)
	}
	if _, ok := o.Member(CEO); !ok {
		o.Members = append([]Member{{ID: CEO, Kind: Person, Person: person, Title: "CEO", Name: personName, Created: now, Updated: now}}, o.Members...)
	}
	if i := slices.IndexFunc(o.Members, func(m Member) bool { return m.ID == CEO }); o.Members[i].Name == "" {
		o.Members[i].Name = "CEO"
	}
	o.Contexts, o.Rules = []Context{}, []Rule{}
	if o.AgentRoutines == nil {
		o.AgentRoutines = []AgentRoutine{}
	}
	for _, c := range f.Contexts {
		o.putContext(Context{ID: c.ID, Scope: c.Scope, Of: c.Of, Title: c.Title, Body: c.Body}, now)
	}
	for _, fr := range f.Rules {
		r := Rule{ID: fr.ID, Scope: fr.Scope, Of: fr.Of, Text: fr.Text, Then: fr.Then, Off: fr.Off}
		b, _ := json.Marshal(fr.When)
		if err := json.Unmarshal(b, &r.When); err != nil {
			return Org{}, fmt.Errorf("rule %q: %w", fr.ID, err)
		}
		o.Rules = append(o.Rules, r)
	}
	o.markExceptions()
	return o, o.Check()
}

package company

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"gopkg.in/yaml.v3"
)

// FileFormat is the version of company.yaml this Pimpo writes. A newer one
// is refused.
const FileFormat = 1

// A File is a company as it travels: what it is, without whose it is.
// People's seats are written without their person; importing gives every
// seat to whoever imports.
type File struct {
	Format      int          `yaml:"format"`
	Name        string       `yaml:"name"`
	Industry    string       `yaml:"industry,omitempty"`
	Mission     string       `yaml:"mission,omitempty"`
	Zone        string       `yaml:"zone,omitempty"`
	Departments []Department `yaml:"departments,omitempty"`
	Roles       []Role       `yaml:"roles,omitempty"`
	Members     []FileMember `yaml:"members"`
}

type FileMember struct {
	ID           string   `yaml:"id"`
	Kind         string   `yaml:"kind,omitempty"`
	Title        string   `yaml:"title,omitempty"`
	Role         string   `yaml:"role,omitempty"`
	Department   string   `yaml:"department,omitempty"`
	ReportsTo    string   `yaml:"reports_to,omitempty"`
	Name         string   `yaml:"name"`
	Avatar       string   `yaml:"avatar,omitempty"`
	Persona      string   `yaml:"persona,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty"`
	Models       []string `yaml:"models,omitempty"`
}

// Export writes the company as a file.
func (o Org) Export() ([]byte, error) {
	f := File{Format: FileFormat, Name: o.Name, Industry: o.Industry, Mission: o.Mission, Zone: o.Zone, Departments: o.Departments, Roles: o.Roles}
	for _, m := range o.Members {
		f.Members = append(f.Members, FileMember{ID: m.ID, Kind: m.Kind, Title: m.Title, Role: m.Role, Department: m.Department, ReportsTo: m.ReportsTo,
			Name: m.Name, Avatar: m.Avatar, Persona: m.Persona, Capabilities: m.Capabilities, Models: m.Models})
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
	o := Org{Company: Company{ID: id, Person: person, Name: f.Name, Industry: f.Industry, Mission: f.Mission, Zone: f.Zone, Created: now, Updated: now},
		Departments: f.Departments, Roles: f.Roles}
	if o.Departments == nil {
		o.Departments = []Department{}
	}
	if o.Roles == nil {
		o.Roles = []Role{}
	}
	for _, fm := range f.Members {
		m := Member{ID: fm.ID, Kind: fm.Kind, Title: fm.Title, Role: fm.Role, Department: fm.Department, ReportsTo: fm.ReportsTo,
			Name: fm.Name, Avatar: fm.Avatar, Persona: fm.Persona, Capabilities: fm.Capabilities, Models: fm.Models, State: Active, Created: now, Updated: now}
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
	return o, o.Check()
}

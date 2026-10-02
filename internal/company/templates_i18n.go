package company

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// The templates are written in English. Each language has one file,
// templates/i18n/<lang>.yaml, with the words a person reads (names,
// functions, responsibilities, context, routine names...) for every
// template, keyed by the template's ids. Ids, capabilities and schedules
// never change, so a company made in one language works like one made in
// another.

// A templateText is one template's words in one language.
type templateText struct {
	Name        string                 `yaml:"name"`
	Industry    string                 `yaml:"industry"`
	Mission     string                 `yaml:"mission"`
	Departments map[string]string      `yaml:"departments"`
	Levels      map[int]string         `yaml:"levels"`
	Roles       map[string]roleText    `yaml:"roles"`
	Contexts    map[string]contextText `yaml:"contexts"`
	Rules       map[string]string      `yaml:"rules"`
	Routines    map[string]routineText `yaml:"routines"`
}

type roleText struct {
	Title            string   `yaml:"title"`
	Function         string   `yaml:"function"`
	Responsibilities []string `yaml:"responsibilities"`
	Deliverables     []string `yaml:"deliverables"`
}

type contextText struct {
	Title string `yaml:"title"`
	Body  string `yaml:"body"`
}

type routineText struct {
	Name         string `yaml:"name"`
	Instructions string `yaml:"instructions"`
}

// templateTexts reads a language's file: template id to its words. A
// language without a file has none, and its templates stay in English.
func templateTexts(lang string) map[string]templateText {
	b, err := templateFiles.ReadFile("templates/i18n/" + lang + ".yaml")
	if err != nil {
		return nil
	}
	var out map[string]templateText
	if err := yaml.Unmarshal(b, &out); err != nil {
		panic(fmt.Sprintf("company: templates/i18n/%s.yaml: %v", lang, err))
	}
	return out
}

// localize puts a template's words in lang into f. What the language
// does not have stays in English.
func localize(f *File, id, lang string) {
	tx, ok := templateTexts(lang)[id]
	if !ok {
		return
	}
	set := func(dst *string, s string) {
		if s != "" {
			*dst = s
		}
	}
	set(&f.Name, tx.Name)
	set(&f.Industry, tx.Industry)
	set(&f.Mission, tx.Mission)
	for i := range f.Departments {
		set(&f.Departments[i].Name, tx.Departments[f.Departments[i].ID])
	}
	for i := range f.Levels.List {
		set(&f.Levels.List[i].Name, tx.Levels[f.Levels.List[i].Level])
	}
	for i := range f.Roles {
		r, rt := &f.Roles[i], tx.Roles[f.Roles[i].ID]
		set(&r.Title, rt.Title)
		set(&r.Function, rt.Function)
		if len(rt.Responsibilities) == len(r.Responsibilities) {
			r.Responsibilities = rt.Responsibilities
		}
		if len(rt.Deliverables) == len(r.Deliverables) {
			r.Deliverables = rt.Deliverables
		}
	}
	for i := range f.Contexts {
		c := tx.Contexts[f.Contexts[i].ID]
		set(&f.Contexts[i].Title, c.Title)
		set(&f.Contexts[i].Body, c.Body)
	}
	for i := range f.Rules {
		set(&f.Rules[i].Text, tx.Rules[f.Rules[i].ID])
	}
	for i := range f.AgentRoutines {
		r := tx.Routines[f.AgentRoutines[i].ID]
		set(&f.AgentRoutines[i].Name, r.Name)
		set(&f.AgentRoutines[i].Instructions, r.Instructions)
	}
}

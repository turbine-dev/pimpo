package company

import (
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

var templateLangs = []string{"pt", "es", "fr", "de", "it", "ja", "ko", "ru", "zh"}

// TestEveryTemplateIsInEveryLanguage keeps the language files whole: every
// word a person reads has a translation, every list keeps its length, and
// nothing names an id the template does not have.
func TestEveryTemplateIsInEveryLanguage(t *testing.T) {
	for _, lang := range templateLangs {
		texts := templateTexts(lang)
		if texts == nil {
			t.Errorf("%s: no templates/i18n/%s.yaml", lang, lang)
			continue
		}
		for id := range texts {
			if !slices.Contains(templateOrder, id) {
				t.Errorf("%s: no template %q", lang, id)
			}
		}
		for _, id := range templateOrder {
			b, _ := TemplateFile(id, "")
			var f File
			if err := yaml.Unmarshal(b, &f); err != nil {
				t.Fatal(err)
			}
			checkText(t, lang+"/"+id, f, texts[id])
		}
	}
}

func checkText(t *testing.T, where string, f File, tx templateText) {
	t.Helper()
	miss := func(what string) { t.Errorf("%s: no translation for %s", where, what) }
	if tx.Name == "" {
		miss("name")
	}
	if f.Industry != "" && tx.Industry == "" {
		miss("industry")
	}
	if f.Mission != "" && tx.Mission == "" {
		miss("mission")
	}
	for _, d := range f.Departments {
		if tx.Departments[d.ID] == "" {
			miss("department " + d.ID)
		}
	}
	for _, l := range f.Levels.List {
		if tx.Levels[l.Level] == "" {
			miss("level " + l.Name)
		}
	}
	for _, r := range f.Roles {
		rt, ok := tx.Roles[r.ID]
		if !ok || rt.Title == "" || (r.Function != "" && rt.Function == "") {
			miss("role " + r.ID)
		}
		if len(rt.Responsibilities) != len(r.Responsibilities) || len(rt.Deliverables) != len(r.Deliverables) {
			t.Errorf("%s: role %s has %d/%d responsibilities and %d/%d deliverables", where, r.ID, len(rt.Responsibilities), len(r.Responsibilities), len(rt.Deliverables), len(r.Deliverables))
		}
	}
	for _, c := range f.Contexts {
		if tx.Contexts[c.ID].Title == "" || tx.Contexts[c.ID].Body == "" {
			miss("context " + c.ID)
		}
	}
	for _, r := range f.Rules {
		if tx.Rules[r.ID] == "" {
			miss("rule " + r.ID)
		}
	}
	for _, r := range f.AgentRoutines {
		if tx.Routines[r.ID].Name == "" || tx.Routines[r.ID].Instructions == "" {
			miss("routine " + r.ID)
		}
	}
	extra := func(kind string, n int, want int) {
		if n > want {
			t.Errorf("%s: %d %s translated, the template has %d", where, n, kind, want)
		}
	}
	extra("departments", len(tx.Departments), len(f.Departments))
	extra("levels", len(tx.Levels), len(f.Levels.List))
	extra("roles", len(tx.Roles), len(f.Roles))
	extra("contexts", len(tx.Contexts), len(f.Contexts))
	extra("rules", len(tx.Rules), len(f.Rules))
	extra("routines", len(tx.Routines), len(f.AgentRoutines))
}

func TestATemplateStartsInThePersonsLanguage(t *testing.T) {
	ts := Templates("pt")
	if ts[0].Name != "Empresa de software" || !slices.Contains(ts[0].Roles, "Gerente de projeto") {
		t.Fatalf("templates in pt = %+v", ts[0])
	}
	b, _ := TemplateFile("software", "pt")
	o, err := Import(b, "c_1", "ana", "Ana", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if o.Departments[1].ID != "engineering" || o.Departments[1].Name != "Engenharia" {
		t.Fatalf("departments = %+v", o.Departments)
	}
	po := o.Roles[0]
	if po.ID != "po" || !strings.HasPrefix(po.Function, "Sabe do que") || len(po.Responsibilities) != 3 || !slices.Contains(po.Capabilities, "company.brief") {
		t.Fatalf("po = %+v", po)
	}
	if o.AgentRoutines[0].Schedule != "0 9 * * 1" || o.AgentRoutines[0].Name != "Radar semanal" {
		t.Fatalf("routines = %+v", o.AgentRoutines)
	}
	// A language without words of its own keeps the English.
	if en := Templates("xx"); en[0].Name != "Software company" {
		t.Fatalf("templates in xx = %+v", en[0])
	}
}

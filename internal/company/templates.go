package company

import (
	"embed"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*.company.yaml templates/i18n/*.yaml
var templateFiles embed.FS

// A Template is a company file to start from.
type Template struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Industry string   `json:"industry,omitempty"`
	Mission  string   `json:"mission,omitempty"`
	Roles    []string `json:"roles"`
	Members  int      `json:"members"`
	File     string   `json:"file"`
}

// templateOrder puts the blank company last.
var templateOrder = []string{"software", "agency", "shop", "channel", "consultancy", "blank"}

// Templates are the company files Pimpo ships with, in lang ("pt", "en"...).
func Templates(lang string) []Template {
	out := []Template{}
	for _, id := range templateOrder {
		b, ok := TemplateFile(id, lang)
		if !ok {
			continue
		}
		var f File
		if yaml.Unmarshal(b, &f) != nil {
			continue
		}
		t := Template{ID: id, Name: f.Name, Industry: f.Industry, Mission: f.Mission, Roles: []string{}, File: string(b)}
		for _, r := range f.Roles {
			t.Roles = append(t.Roles, r.Title)
		}
		t.Members = len(slices.DeleteFunc(slices.Clone(f.Members), func(m FileMember) bool { return m.Kind == Person }))
		out = append(out, t)
	}
	return out
}

// TemplateFile is a template's company file by id, with its words in lang;
// English (or "") is the file as written.
func TemplateFile(id, lang string) ([]byte, bool) {
	if !slices.Contains(templateOrder, id) || strings.ContainsAny(id, "./") {
		return nil, false
	}
	b, err := templateFiles.ReadFile("templates/" + id + ".company.yaml")
	if err != nil {
		return nil, false
	}
	if lang == "" || lang == "en" || strings.ContainsAny(lang, "./") {
		return b, true
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, false
	}
	localize(&f, id, lang)
	out, err := yaml.Marshal(f)
	return out, err == nil
}

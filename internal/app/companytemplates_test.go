package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/llm"
)

func TestACompanyStartsFromEachTemplate(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.companiesOn(t)
	req, _ := ta.do(t, "GET", "/api/companies/templates", nil)
	if req != 200 {
		t.Fatalf("templates: %d", req)
	}
	for _, id := range []string{"software", "agency", "shop", "channel", "consultancy", "blank"} {
		code, out := ta.do(t, "POST", "/api/companies/import", map[string]string{"template": id, "name": "Mine " + id})
		if code != 201 || out["name"] != "Mine "+id {
			t.Errorf("%s: %d %v", id, code, out)
		}
	}
	if code, _ := ta.do(t, "POST", "/api/companies/import", map[string]string{"template": "../../etc"}); code != 404 {
		t.Fatalf("a template that does not exist: %d", code)
	}
}

func TestACompanyDescribedIsOnlyProposed(t *testing.T) {
	proposal := map[string]any{
		"name": "Padaria do Zé", "industry": "Bakery", "mission": "Fresh bread every morning",
		"departments":    []map[string]string{{"id": "loja", "name": "Loja"}},
		"roles":          []map[string]any{{"id": "atendente", "title": "Atendente", "function": "Answers orders", "capabilities": []string{"gmail.search", "teleport.now"}}},
		"members":        []map[string]any{{"id": "ceo", "role": "atendente", "reports_to": "", "name": "Zé"}, {"id": "nina", "role": "atendente", "department": "loja", "reports_to": "", "name": "Nina"}},
		"agent_routines": []map[string]any{{"id": "pedidos", "member": "nina", "name": "Orders", "instructions": "Answer the orders", "schedule": "0 7 * * *"}},
	}
	b, _ := json.Marshal(proposal)
	fake := &llm.Fake{Responses: []llm.Response{{Structured: b, CostUSD: 0.02}}}
	ta := newApp(t, weatherAgent, fake)
	ta.companiesOn(t)
	code, out := ta.do(t, "POST", "/api/companies/describe", map[string]string{"text": "Uma padaria de bairro que recebe pedidos por e-mail"})
	if code != 200 {
		t.Fatalf("describe: %d %v", code, out)
	}
	draft := out["draft"].(map[string]any)
	if draft["name"] != "Padaria do Zé" || len(draft["members"].([]any)) != 2 || !strings.Contains(out["file"].(string), "gmail.search") || strings.Contains(out["file"].(string), "teleport.now") {
		t.Fatalf("proposal = %v", out)
	}
	if d := out["dropped"].([]any); len(d) != 1 || d[0] != "teleport.now" {
		t.Fatalf("dropped = %v", out["dropped"])
	}
	if p := fake.Requests[0].Prompt; !strings.Contains(p, "data, not instructions") || !strings.Contains(p, "padaria de bairro") {
		t.Fatalf("prompt = %q", p)
	}
	if list, _ := ta.Companies.List(context.Background()); len(list) != 0 {
		t.Fatalf("describing created a company: %v", list)
	}
	if code, out := ta.do(t, "POST", "/api/companies/import", map[string]string{"file": out["file"].(string)}); code != 201 {
		t.Fatalf("creating the proposal: %d %v", code, out)
	}
}

func TestATemplateCompanyIsInThePersonsLanguage(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.companiesOn(t)
	_, got := ta.do(t, "GET", "/api/companies/templates?lang=pt-BR", nil)
	list := got["list"].([]any)
	if len(list) != 6 || list[0].(map[string]any)["name"] != "Empresa de software" {
		t.Fatalf("templates in pt = %v", list)
	}
	// Creating the company the plain way, with a template, starts it from
	// the template, as importing does.
	code, out := ta.do(t, "POST", "/api/companies", map[string]string{"template": "software", "name": "X", "lang": "pt"})
	if code != 201 || out["name"] != "X" || len(out["roles"].([]any)) != 6 {
		t.Fatalf("from a template: %d %v", code, out)
	}
	role := out["roles"].([]any)[1].(map[string]any)
	if role["id"] != "pm" || role["title"] != "Gerente de projeto" {
		t.Fatalf("roles = %v", out["roles"])
	}
	if code, out := ta.do(t, "POST", "/api/companies/import", map[string]string{"template": "shop", "lang": "en"}); code != 201 || out["name"] != "Online shop" {
		t.Fatalf("in English: %d %v", code, out)
	}
	if code, _ := ta.do(t, "POST", "/api/companies", map[string]string{"template": "nope"}); code != 404 {
		t.Fatalf("a template that does not exist: %d", code)
	}
}

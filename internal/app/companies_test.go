package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/llm"
)

func (ta *testApp) companiesOn(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	s := ta.Settings(ctx)
	s.LabsOn = append(s.LabsOn, companiesLab)
	if err := ta.SaveSettings(ctx, s, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestCompaniesStartOff(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	if code, out := ta.do(t, "POST", "/api/companies", map[string]any{"name": "Lume"}); code != 403 || !strings.Contains(out["error"].(string), "Labs") {
		t.Fatalf("made a company with the lab off: %d %v", code, out)
	}
	ta.companiesOn(t)
	if code, _ := ta.do(t, "GET", "/api/companies", nil); code != 200 {
		t.Fatalf("list with the lab on: %d", code)
	}
}

func TestMakingACompany(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.companiesOn(t)
	code, out := ta.do(t, "POST", "/api/companies", map[string]any{"name": " Lume Moda ", "industry": "online shop"})
	if code != 201 || out["name"] != "Lume Moda" || out["grant"] != company.Configure {
		t.Fatalf("create: %d %v", code, out)
	}
	id := out["id"].(string)
	base := "/api/companies/" + id
	steps := []struct {
		method, path string
		body         any
		want         int
	}{
		{"PUT", base + "/departments/vendas", map[string]any{"name": "Vendas"}, 200},
		{"PUT", base + "/roles/atendente", map[string]any{"title": "Atendente", "capabilities": []string{"gmail.search", "gmail.search"}}, 200},
		{"PUT", base + "/roles/espiao", map[string]any{"title": "Espião", "capabilities": []string{"spy.read"}}, 400},
		{"PUT", base + "/members/clara", map[string]any{"name": "Clara", "role": "atendente", "department": "vendas", "reports_to": "ceo"}, 200},
		{"PUT", base + "/members/eve", map[string]any{"name": "Eve", "role": "atendente", "reports_to": "nobody"}, 400},
		{"PUT", base + "/members/rui", map[string]any{"name": "Rui", "kind": "person", "person": "rui", "reports_to": "ceo"}, 400},
		{"DELETE", base + "/roles/atendente", nil, 400},
	}
	for _, s := range steps {
		if code, out := ta.do(t, s.method, s.path, s.body); code != s.want {
			t.Fatalf("%s %s = %d %v, want %d", s.method, s.path, code, out, s.want)
		}
	}
	o, err := ta.Companies.Org(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	clara, _ := o.Member("clara")
	role, _ := o.Role("atendente")
	if clara.Kind != company.Agent || clara.State != company.Active || len(role.Capabilities) != 1 || len(o.Members) != 2 {
		t.Fatalf("org = %+v", o)
	}
	if code, _ := ta.do(t, "DELETE", base, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := ta.do(t, "GET", base, nil); code != 404 {
		t.Fatalf("a deleted company: %d", code)
	}
}

func TestACompanyFileRoundTrip(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.companiesOn(t)
	_, out := ta.do(t, "POST", "/api/companies", map[string]any{"name": "Pimpo Dev"})
	base := "/api/companies/" + out["id"].(string)
	ta.do(t, "PUT", base+"/roles/dev", map[string]any{"title": "Fullstack", "function": "Ship issues"})
	ta.do(t, "PUT", base+"/members/bia", map[string]any{"name": "Bia", "role": "dev", "reports_to": "ceo"})
	code, file := ta.raw(t, "tok", "GET", base+"/export", nil)
	if code != 200 || !strings.Contains(file, "Ship issues") || strings.Contains(file, "person:") {
		t.Fatalf("export: %d\n%s", code, file)
	}
	code, out = ta.do(t, "POST", "/api/companies/import", map[string]any{"file": file})
	if code != 201 || out["name"] != "Pimpo Dev" || len(out["members"].([]any)) != 2 {
		t.Fatalf("import: %d %v", code, out)
	}
	if code, out = ta.do(t, "POST", "/api/companies/import", map[string]any{"file": "format: 9\nname: x\nmembers: []"}); code != 400 {
		t.Fatalf("a newer file: %d %v", code, out)
	}
}

// A company is its person's, and its partners' by grant; nobody else finds
// it, the administrator included.
func TestACompanyIsOnlyItsPeoples(t *testing.T) {
	h := newHouse(t)
	ana := h.anas["company"]
	if code, _ := h.do(t, "GET", "/api/companies/"+ana, nil); code != 404 {
		t.Fatalf("the administrator opened Ana's company: %d", code)
	}
	if code, _ := h.raw(t, h.ana, "PUT", "/api/companies/"+ana, js(map[string]any{"name": "Ana Co", "partners": []map[string]string{{"person": "owner", "grant": "view"}}})); code != 200 {
		t.Fatalf("Ana shares her company: %d", code)
	}
	if code, body := h.raw(t, "tok", "GET", "/api/companies/"+ana, nil); code != 200 || !strings.Contains(body, `"grant":"view"`) {
		t.Fatalf("a partner with view: %d %.200s", code, body)
	}
	if code, _ := h.do(t, "PUT", "/api/companies/"+ana+"/departments/x", map[string]any{"name": "X"}); code != 403 {
		t.Fatalf("a partner with view changed the company: %d", code)
	}
	if code, _ := h.do(t, "DELETE", "/api/companies/"+ana, nil); code != 403 {
		t.Fatalf("a partner deleted the company: %d", code)
	}
	if _, out := h.do(t, "GET", "/api/companies", nil); len(out["list"].([]any)) != 2 {
		t.Fatalf("the administrator's list: %v", out)
	}
	h.do(t, "DELETE", "/api/people/"+h.anaID, nil)
	if _, err := h.Companies.Org(context.Background(), ana); err != company.ErrNotFound {
		t.Fatalf("a removed person's company stayed: %v", err)
	}
}

// A company's rules sit on the house's: they may restrict and allow among
// themselves, never past the house.
func TestCompanyRulesSitOnTheHouses(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.companiesOn(t)
	ctx := context.Background()
	_, out := ta.do(t, "POST", "/api/companies", map[string]any{"name": "Lume"})
	id := out["id"].(string)
	base := "/api/companies/" + id
	ta.do(t, "PUT", base+"/roles/atendente", map[string]any{"title": "Atendente", "capabilities": []string{"gmail.send", "telegram.send", "gmail.search"}})
	ta.do(t, "PUT", base+"/members/clara", map[string]any{"name": "Clara", "role": "atendente", "reports_to": "ceo"})
	steps := []struct {
		path string
		body any
		want int
	}{
		{"/rules/quiet", map[string]any{"scope": "company", "text": "No Telegram", "when": map[string]any{"capabilities": []string{"telegram.send"}}, "then": "block"}, 200},
		{"/rules/clerks-telegram", map[string]any{"scope": "role", "of": "atendente", "text": "Clerks may", "when": map[string]any{"capabilities": []string{"telegram.send"}}, "then": "allow"}, 409},
		{"/rules/email", map[string]any{"scope": "company", "text": "Email freely", "when": map[string]any{"capabilities": []string{"gmail.send"}}, "then": "allow"}, 200},
		{"/contexts/voz", map[string]any{"scope": "company", "title": "Voice", "body": "Kind and brief."}, 200},
	}
	for _, s := range steps {
		if code, out := ta.do(t, "PUT", base+s.path, s.body); code != s.want {
			t.Fatalf("PUT %s = %d %v", s.path, code, out)
		}
	}
	code, out := ta.do(t, "GET", base+"/members/clara/preview", nil)
	if code != 200 || !strings.Contains(out["brief"].(string), "Kind and brief.") {
		t.Fatalf("preview: %d %v", code, out)
	}
	verdicts := map[string]string{}
	for _, v := range out["rules"].([]any) {
		v := v.(map[string]any)
		verdicts[v["capability"].(string)] = v["verdict"].(string)
	}
	// The house asks before any email is sent; the company cannot let it through.
	if verdicts["telegram.send"] != "block" || verdicts["gmail.send"] != "ask" || verdicts["gmail.search"] != "allow" {
		t.Fatalf("verdicts = %v", verdicts)
	}
	h := &host.Host{Env: ta.Explore.Env, Source: "routine:t#1", Member: id + "/clara"}
	if _, err := h.Call(ctx, "telegram.send", "", map[string]any{"text": "oi"}); !errors.Is(err, host.ErrBlocked) {
		t.Fatalf("a member got past its company's rule: %v", err)
	}
	gone := &host.Host{Env: ta.Explore.Env, Source: "routine:t#1", Member: id + "/eve"}
	if _, err := gone.Call(ctx, "gmail.search", "", map[string]any{}); !errors.Is(err, host.ErrBlocked) {
		t.Fatalf("someone not in the company acted as a member: %v", err)
	}
}

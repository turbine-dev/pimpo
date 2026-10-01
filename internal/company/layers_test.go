package company

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/policy"
)

func send(member string) policy.Action {
	return policy.Action{Capability: "gmail.send", Risk: capability.Irreversible, Member: "c1/" + member}
}

func TestTheMostSpecificRuleWins(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	never := Rule{ID: "no-email", Scope: ScopeCompany, Text: "Never email customers", When: policy.When{Capabilities: []string{"gmail.send"}}, Then: policy.Block}
	if _, err := s.SaveRule(ctx, "c1", never); err != nil {
		t.Fatal(err)
	}
	allow := Rule{ID: "clerks-email", Scope: ScopeRole, Of: "atendente", Text: "Clerks may email customers", When: policy.When{Capabilities: []string{"gmail.send"}}, Then: policy.Allow}
	if _, err := s.SaveRule(ctx, "c1", allow); !errors.Is(err, ErrException) {
		t.Fatalf("an unmarked exception: %v", err)
	}
	allow.Exception = true
	o, err := s.SaveRule(ctx, "c1", allow)
	if err != nil {
		t.Fatal(err)
	}
	if r := o.Rules[1]; !r.Exception || len(r.Overrides) != 1 || r.Overrides[0] != "no-email" {
		t.Fatalf("exception = %+v", r)
	}
	for _, c := range []struct {
		member string
		want   policy.Verdict
		rule   string
	}{{"clara", policy.Allow, "company:clerks-email"}, {"bia", policy.Block, "company:no-email"}} {
		d, ok := o.Decide(c.member, send(c.member))
		if !ok || d.Verdict != c.want || d.Rule != c.rule {
			t.Errorf("%s: %+v %v", c.member, d, ok)
		}
	}
	// Asking is looser than the company's block, so it is an exception too.
	ask := Rule{ID: "clara-asks", Scope: ScopeMember, Of: "clara", Text: "Clara asks first", When: policy.When{MinRisk: "irreversible"}, Then: policy.Ask, Exception: true}
	if o, err = s.SaveRule(ctx, "c1", ask); err != nil {
		t.Fatal(err)
	}
	if d, _ := o.Decide("clara", send("clara")); d.Verdict != policy.Ask {
		t.Fatalf("Clara's own rule did not win: %+v", d)
	}
	if _, ok := o.Decide("clara", policy.Action{Capability: "gmail.search", Risk: capability.Read}); ok {
		t.Fatal("a read no rule covers was decided")
	}
}

func TestABroaderRuleMakesExceptionsVisible(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	s.SaveRule(ctx, "c1", Rule{ID: "clerks-email", Scope: ScopeRole, Of: "atendente", Text: "Clerks email", When: policy.When{Capabilities: []string{"gmail.send"}}, Then: policy.Allow})
	o, err := s.SaveRule(ctx, "c1", Rule{ID: "careful", Scope: ScopeCompany, Text: "Ask before anything irreversible", When: policy.When{MinRisk: "irreversible"}, Then: policy.Ask})
	if err != nil {
		t.Fatal(err)
	}
	if r := o.Rules[0]; !r.Exception || r.Overrides[0] != "careful" {
		t.Fatalf("the older rule is not shown as an exception: %+v", r)
	}
	if o, _ = s.DeleteRule(ctx, "c1", "careful"); o.Rules[0].Exception {
		t.Fatalf("still an exception after the broader rule went: %+v", o.Rules[0])
	}
}

func TestRulesAreAboutMembers(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	for _, r := range []Rule{
		{ID: "x", Scope: ScopeCompany, Text: "Ana", When: policy.When{People: []string{"ana"}}, Then: policy.Ask},
		{ID: "x", Scope: ScopeRole, Of: "chef", Text: "nobody", Then: policy.Ask},
		{ID: "x", Scope: ScopeCompany, Text: "bad", Then: "maybe"},
		{ID: "x", Scope: ScopeCompany, Text: "bad", When: policy.When{Capabilities: []string{"spy.read"}}, Then: policy.Ask},
	} {
		if _, err := s.SaveRule(ctx, "c1", r); err == nil {
			t.Errorf("saved %+v", r)
		}
	}
}

func TestContextsKeepTheirEarlierTexts(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	c := Context{ID: "trocas", Scope: ScopeCompany, Title: "Returns", Body: "30 days"}
	s.SaveContext(ctx, "c1", c)
	c.Body = "15 days"
	s.SaveContext(ctx, "c1", c)
	o, _ := s.SaveContext(ctx, "c1", c)
	got, _ := o.Context("trocas")
	if got.Version != 2 || len(got.Previous) != 1 || got.Previous[0] != "30 days" {
		t.Fatalf("context = %+v", got)
	}
	if _, err := s.SaveContext(ctx, "c1", Context{ID: "x", Scope: ScopeMember, Of: "nobody", Title: "x"}); err == nil {
		t.Fatal("saved a context for nobody")
	}
}

func TestABriefGoesFromTheCompanyToTheMember(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	s.Update(ctx, Company{ID: "c1", Name: "Lume Moda", Industry: "online shop", Mission: "Sell kindly."})
	s.SaveRole(ctx, "c1", Role{ID: "atendente", Title: "Atendente", Function: "Answer customers", Responsibilities: []string{"WhatsApp", "email"}})
	s.SaveMember(ctx, "c1", Member{ID: "clara", Kind: Agent, Role: "atendente", Department: "vendas", ReportsTo: "bia", Name: "Clara", Persona: "Warm and brief."})
	s.SaveContext(ctx, "c1", Context{ID: "voz", Scope: ScopeCompany, Title: "Voice", Body: "COMPANYVOICE"})
	s.SaveContext(ctx, "c1", Context{ID: "vendas", Scope: ScopeDepartment, Of: "vendas", Title: "Sales", Body: "DEPTNOTE"})
	s.SaveContext(ctx, "c1", Context{ID: "script", Scope: ScopeRole, Of: "atendente", Title: "Script", Body: "ROLESCRIPT"})
	o, _ := s.SaveContext(ctx, "c1", Context{ID: "clara", Scope: ScopeMember, Of: "clara", Title: "Notes", Body: "MEMBERNOTE"})
	brief := o.Brief("clara")
	last := -1
	for _, want := range []string{"Sell kindly.", "COMPANYVOICE", "DEPTNOTE", "Answer customers", "ROLESCRIPT", "Warm and brief.", "MEMBERNOTE"} {
		i := strings.Index(brief, want)
		if i < last {
			t.Fatalf("%q is out of order or missing in:\n%s", want, brief)
		}
		last = i
	}
	if strings.Contains(o.Brief("bia"), "ROLESCRIPT") {
		t.Fatal("Bia got the clerks' script")
	}
	if o, _ = s.DeleteMember(ctx, "c1", "clara"); len(o.Contexts) != 3 {
		t.Fatalf("Clara's notes stayed: %+v", o.Contexts)
	}
}

func TestRulesAndContextsTravelInTheFile(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	s.SaveRule(ctx, "c1", Rule{ID: "careful", Scope: ScopeCompany, Text: "Ask first", When: policy.When{MinRisk: "irreversible", Except: []string{"gmail.draft"}}, Then: policy.Ask})
	o, _ := s.SaveContext(ctx, "c1", Context{ID: "voz", Scope: ScopeCompany, Title: "Voice", Body: "Kind."})
	b, err := o.Export()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Import(b, "c2", "rui", "Rui", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 1 || got.Rules[0].When.MinRisk != "irreversible" || got.Rules[0].When.Except[0] != "gmail.draft" || len(got.Contexts) != 1 || got.Contexts[0].Body != "Kind." {
		t.Fatalf("imported = %+v %+v\n%s", got.Rules, got.Contexts, b)
	}
}

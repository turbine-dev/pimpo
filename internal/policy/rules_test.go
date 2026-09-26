package policy

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/denerFernandes/pimpo/internal/capability"
	"github.com/denerFernandes/pimpo/internal/event"
)

func engine(t *testing.T) *Engine {
	ev, _ := event.Open(filepath.Join(t.TempDir(), "v.db"))
	t.Cleanup(func() { ev.Close() })
	return &Engine{Events: ev}
}

func TestBalancedPreset(t *testing.T) {
	e := engine(t)
	ctx := context.Background()
	for name, tc := range map[string]struct {
		a    Action
		want Verdict
	}{
		"read":                {Action{Capability: "gmail.search", Risk: capability.Read, Source: "routine:x#1"}, Allow},
		"message to owner":    {Action{Capability: "telegram.send", Risk: capability.Notify, Source: "routine:x#1"}, Allow},
		"reversible change":   {Action{Capability: "gmail.archive", Risk: capability.Reversible, Source: "routine:x#1"}, Reversible},
		"irreversible change": {Action{Capability: "gmail.delete", Risk: capability.Irreversible, Source: "routine:x#1"}, Ask},
		"new host exploring":  {Action{Capability: "http.getJSON", Scope: "evil.example", Risk: capability.Read, Source: "exploration:e1"}, Ask},
		"known host routine":  {Action{Capability: "http.getJSON", Scope: "api.open-meteo.com", Risk: capability.Read, Source: "routine:x#1"}, Allow},
	} {
		if got := e.Decide(ctx, tc.a); got.Verdict != tc.want {
			t.Errorf("%s: got %s (%s), want %s", name, got.Verdict, got.Reason, tc.want)
		}
	}
	e.AllowHost(ctx, "evil.example", false, "human:owner")
	if got := e.Decide(ctx, Action{Capability: "http.getJSON", Scope: "evil.example", Source: "routine:x#1"}); got.Verdict != Block {
		t.Fatalf("denied host: %s", got.Verdict)
	}
}

func TestOwnerRulesAndStrictestWins(t *testing.T) {
	e := engine(t)
	ctx := context.Background()
	rules := append(Preset(),
		Rule{ID: "r1", Text: "Nunca apague e-mail sem me perguntar", When: When{Capabilities: []string{"gmail.delete", "gmail.trash"}}, Then: Ask},
		Rule{ID: "r2", Text: "A triagem pode arquivar sem perguntar", When: When{Capabilities: []string{"gmail.archive"}, Source: "routine:triage"}, Then: Allow},
		Rule{ID: "r3", Text: "Nada de e-mails do chefe", When: When{Capabilities: []string{"gmail.archive"}, ArgsContain: []string{"boss@"}}, Then: Block},
	)
	if err := e.SaveRules(ctx, rules, "human:owner"); err != nil {
		t.Fatal(err)
	}
	if d := e.Decide(ctx, Action{Capability: "gmail.trash", Risk: capability.Reversible, Source: "routine:cleanup#3"}); d.Verdict != Ask || d.Rule != "r1" {
		t.Fatalf("trash: %+v", d)
	}
	// A rule cannot loosen the default for reversible changes below "reversible".
	if d := e.Decide(ctx, Action{Capability: "gmail.archive", Risk: capability.Reversible, Source: "routine:triage#9"}); d.Verdict != Reversible {
		t.Fatalf("archive in triage: %+v", d)
	}
	if d := e.Decide(ctx, Action{Capability: "gmail.archive", Risk: capability.Reversible, Args: map[string]any{"from": "boss@acme.com"}, Source: "routine:triage#9"}); d.Verdict != Block || d.Rule != "r3" {
		t.Fatalf("boss email: %+v", d)
	}
	if err := e.SaveRules(ctx, []Rule{{Text: "x", Then: "maybe"}}, "human:owner"); err == nil {
		t.Fatal("invalid verdict accepted")
	}
	if err := e.SaveRules(ctx, []Rule{{Text: "x", Then: Ask, When: When{Capabilities: []string{"bank.pay"}}}}, "human:owner"); err == nil {
		t.Fatal("unknown capability accepted")
	}
}

func TestAlwaysOverridesTheGeneralAsk(t *testing.T) {
	e := engine(t)
	ctx := context.Background()
	send := Action{Capability: "gmail.send", Risk: capability.Irreversible, Source: "routine:followup#4"}
	if d := e.Decide(ctx, send); d.Verdict != Ask {
		t.Fatalf("before: %+v", d)
	}
	rules := append(e.Rules(ctx), AllowAlways(send))
	e.SaveRules(ctx, rules, "human:owner")
	if d := e.Decide(ctx, send); d.Verdict != Allow || d.Rule != "always-routine-followup-gmail-send" {
		t.Fatalf("after always: %+v", d)
	}
	other := Action{Capability: "gmail.send", Risk: capability.Irreversible, Source: "routine:other#1"}
	if d := e.Decide(ctx, other); d.Verdict != Ask {
		t.Fatalf("other routine should still ask: %+v", d)
	}
	e.SaveRules(ctx, append(rules, Rule{ID: "b", Text: "never send", When: When{Capabilities: []string{"gmail.send"}}, Then: Block}), "human:owner")
	if d := e.Decide(ctx, send); d.Verdict != Block {
		t.Fatalf("block must win: %+v", d)
	}
}

func TestRulesPerPersonAndGuests(t *testing.T) {
	e := engine(t)
	ctx := context.Background()
	e.SaveRules(ctx, []Rule{{ID: "kids", Text: "As crianças não podem comprar nada", When: When{People: []string{"leo"}, Capabilities: []string{"gmail.send"}}, Then: Block}}, "human:owner")
	send := Action{Capability: "gmail.send", Risk: capability.Irreversible, Source: "routine:r"}
	if d := e.Decide(ctx, send); d.Verdict != Allow {
		t.Fatalf("owner blocked by a rule for leo: %+v", d)
	}
	send.Person = "leo"
	if d := e.Decide(ctx, send); d.Verdict != Block {
		t.Fatalf("leo not blocked: %+v", d)
	}
	e.SaveRules(ctx, []Rule{AllowAlways(Action{Capability: "gmail.label", Source: "routine:r"})}, "human:owner")
	label := Action{Capability: "gmail.label", Risk: capability.Reversible, Source: "routine:r", Person: "visita", Role: "guest"}
	if d := e.Decide(ctx, label); d.Verdict != Ask {
		t.Fatalf("a guest's change went through: %+v", d)
	}
	label.Role = "member"
	if d := e.Decide(ctx, label); d.Verdict != Reversible {
		t.Fatalf("member: %+v", d)
	}
	if err := (Rule{Then: Ask, When: When{Roles: []string{"kid"}}}).Validate(); err == nil {
		t.Fatal("accepted an unknown role")
	}
}

func TestWhatsAppToOthersAlwaysAsks(t *testing.T) {
	e := engine(t)
	ctx := context.Background()
	e.SaveRules(ctx, []Rule{{ID: "x", Text: "allow all", Then: Allow}, AllowAlways(Action{Capability: "whatsapp.send_to", Source: "routine:r"})}, "human:owner")
	if d := e.Decide(ctx, Action{Capability: "whatsapp.send_to", Risk: capability.Irreversible, Source: "routine:r"}); d.Verdict != Ask {
		t.Fatalf("%+v", d)
	}
	e.SaveRules(ctx, []Rule{{ID: "b", Text: "never", When: When{Capabilities: []string{"whatsapp.send_to"}}, Then: Block}}, "human:owner")
	if d := e.Decide(ctx, Action{Capability: "whatsapp.send_to", Risk: capability.Irreversible}); d.Verdict != Block {
		t.Fatalf("%+v", d)
	}
}

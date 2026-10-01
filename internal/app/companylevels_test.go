package app

import (
	"context"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/judge"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/policy"
)

// wordJudge thinks a decision is the CEO's when it names one of a few
// strategic words, and is unsure about money.
type wordJudge struct{}

func (wordJudge) Ask(_ context.Context, _ string, item any) (judge.Answer, error) {
	text := strings.ToLower(item.(map[string]string)["decision"])
	for _, w := range []string{"sócio", "partner", "lawsuit", "processo", "investor", "fire", "demitir"} {
		if strings.Contains(text, w) {
			return judge.Answer{P: 0.9}, nil
		}
	}
	if strings.Contains(text, "money") || strings.Contains(text, "dinheiro") {
		return judge.Answer{P: 0.4}, nil
	}
	return judge.Answer{P: 0.05}, nil
}

// The labeled bank: every decision a person marked as the CEO's must reach
// the CEO. Sending more there than needed is acceptable; fewer is not.
func TestNoStrategicDecisionMissesTheCEO(t *testing.T) {
	ta, co := team(t, &script{})
	ta.DemoJudge = wordJudge{}
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	o.Levels = company.SuggestedLevels()
	o.Levels.List[3].When.Words = []string{"preço", "price", "desconto"}
	ta.Companies.Update(ctx, o.Company)
	o, _ = ta.Companies.Org(ctx, co)
	bank := []struct {
		m   company.Matter
		ceo bool
	}{
		{company.Matter{Text: "Raise the price of the Pro plan?"}, true},
		{company.Matter{Text: "Dar 20% de desconto para a Ana?"}, true},
		{company.Matter{Text: "Sign the contract with the agency?", Kind: "contract"}, true},
		{company.Matter{Text: "Spend $300 on ads this week?", AmountUSD: 300}, true},
		{company.Matter{Text: "Post the launch on LinkedIn?", Public: true}, true},
		{company.Matter{Text: "Bring in a new partner for distribution?"}, true},
		{company.Matter{Text: "Answer the lawsuit letter?"}, true},
		{company.Matter{Text: "Should we fire the contractor?"}, true},
		{company.Matter{Text: "Hire a second designer?", Kind: "hiring"}, true},
		{company.Matter{Text: "Move money from savings to ads?"}, true},
		{company.Matter{Text: "Accept the investor's offer?"}, true},
		{company.Matter{Text: "Mudar o orçamento do mês?", Kind: "budget"}, true},
		{company.Matter{Text: "Which button color?"}, false},
		{company.Matter{Text: "Rename a variable in the login code?"}, false},
		{company.Matter{Text: "Answer a customer about delivery?"}, false},
		{company.Matter{Text: "Order coffee for the office, $15?", AmountUSD: 15}, false},
		{company.Matter{Text: "Use Postgres or SQLite for the cache?", Kind: "architecture"}, false},
	}
	top, _ := o.Levels.Top()
	missed := 0
	for _, c := range bank {
		level, why := ta.levelOf(ctx, o, c.m)
		if c.ceo && level != top.Level {
			missed++
			t.Errorf("%q stayed at level %d (%s)", c.m.Text, level, why)
		}
	}
	if missed > 0 {
		t.Fatalf("%d strategic decisions missed the CEO", missed)
	}
}

func TestAStrategicQuestionGoesToTheCEOWithOpinions(t *testing.T) {
	s := &script{}
	s.rules = []scriptRule{
		{"Give your recommendation", func(r llm.AgentRequest) string {
			q := r.Prompt[strings.Index(r.Prompt, "(question ")+10:]
			q = q[:strings.Index(q, ")")]
			errorf(t, rpc(r.MCPURL, 1, "company_answer", map[string]any{"question": q, "choice": "No", "reason": "margins are thin"}))
			return "Gave my view."
		}},
		{"Ask about the price", func(r llm.AgentRequest) string {
			errorf(t, rpc(r.MCPURL, 1, "company_ask", map[string]any{"question": "Lower the price by 30%?", "options": []string{"Yes", "No"}, "kind": "price"}))
			return "Asked."
		}},
	}
	ta, co := team(t, s)
	ctx := context.Background()
	o, _ := ta.Companies.Org(ctx, co)
	o.Levels = company.SuggestedLevels()
	ta.Companies.Update(ctx, o.Company)
	o, _ = ta.Companies.Org(ctx, co)
	w, _ := ta.enqueue(ctx, o, "bia", "Ask about the price", nil, "test", 0)
	ta.waitWorkState(t, w.ID, company.WorkWaiting)
	var q company.Question
	ta.waitFor(t, "Rui's opinion", func() bool {
		qs, _ := ta.Companies.Questions(ctx, co, true)
		if len(qs) == 1 {
			q = qs[0]
		}
		return len(q.Opinions) == 1
	})
	if q.To != company.CEO || q.Level != 4 || q.Why != "kind price" || q.Opinions[0].Member != "rui" || q.Opinions[0].Choice != "No" {
		t.Fatalf("question = %+v", q)
	}
}

func TestAnActionAtTheCEOsLevelWaitsForThemWhateverTheAutonomy(t *testing.T) {
	ta, co := clerk(t)
	ctx := context.Background()
	ta.autonomy(t, co, map[string]any{"kind": "self"})
	o, _ := ta.Companies.Org(ctx, co)
	o.Levels = company.SuggestedLevels()
	ta.Companies.Update(ctx, o.Company)
	big := sendAs(co)
	big.Args = map[string]any{"to": "supplier@example.com", "amount": 500.0}
	if d := ta.decide(ctx, big); d.Verdict != policy.Ask {
		t.Fatalf("a $500 decision did not reach the CEO: %+v", d)
	}
	// A change the rules allow, at a boss's level, goes to the boss even
	// when Jev would have let it through.
	ta.Rules.SaveRules(ctx, nil, "test")
	ta.DemoJudge = fixedJudge(0.99)
	ta.LLM = &llm.Fake{Responses: []llm.Response{{Structured: []byte(`{"approve":false,"reason":"not now"}`)}}}
	ta.autonomy(t, co, map[string]any{"kind": "jev", "threshold": 0.9})
	if d := ta.decide(ctx, sendAs(co)); d.Verdict != policy.Block || !strings.Contains(d.Reason, "Bia said no") {
		t.Fatalf("the boss was not asked: %+v", d)
	}
}

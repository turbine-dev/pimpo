package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/approval"
	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
)

// outgoing records what would have been sent.
type outgoing struct {
	mu   sync.Mutex
	sent []any
}

func (o *outgoing) Capabilities() []string { return []string{"gmail.send", "whatsapp.send_to"} }

func (o *outgoing) Call(_ context.Context, _, _ string, args any) (any, error) {
	o.mu.Lock()
	o.sent = append(o.sent, args)
	o.mu.Unlock()
	return map[string]any{"ok": true}, nil
}

// grantHouse is an app where every email asks first, a routine that
// sends them, and a way to run one of its calls as a new run.
type grantHouse struct {
	*testApp
	out *outgoing
}

func newGrantHouse(t *testing.T) *grantHouse {
	t.Helper()
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.Approvals.Timeout = 3 * time.Second
	rules := append(policy.Preset(), policy.Rule{ID: "r-mail", Text: "ask before email", When: policy.When{Capabilities: []string{"gmail.send"}}, Then: policy.Ask})
	if err := ta.Rules.SaveRules(ctx, rules, "human:owner"); err != nil {
		t.Fatal(err)
	}
	g := &grantHouse{testApp: ta, out: &outgoing{}}
	g.saveRoutine(t)
	return g
}

func (g *grantHouse) saveRoutine(t *testing.T) {
	t.Helper()
	body := routine.Routine{Name: "Resumo", Code: "async function run() {}", Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"gmail.send", "whatsapp.send_to"}}}
	if _, err := g.Store.SaveRoutine(context.Background(), "brief", body, "test", "owner"); err != nil {
		t.Fatal(err)
	}
}

// call makes one call as a new run of the routine for person. It returns
// the id of the approval it waits for ("" when it did not ask) and the
// call's outcome once answer (if any) answers it.
func (g *grantHouse) call(t *testing.T, person, capability string, args map[string]any, answer func(id string)) (asked bool, err error) {
	t.Helper()
	ctx := context.Background()
	rt, _ := g.Store.Routine(ctx, "brief")
	run, _ := g.Store.StartRun(ctx, "brief", rt.Version)
	env := g.Explore.Env
	env.Router = connector.NewRouter(g.out)
	h := &host.Host{Env: env, Source: fmt.Sprintf("routine:brief#%d", run), Person: person}
	done := make(chan error, 1)
	go func() {
		_, err := h.Call(ctx, capability, "", args)
		done <- err
	}()
	for {
		select {
		case err := <-done:
			return asked, err
		case <-time.After(10 * time.Millisecond):
			if open := g.Approvals.Open(); len(open) == 1 && !asked {
				asked = true
				if answer == nil {
					g.Approvals.Resolve(ctx, open[0].ID, approval.Deny, "human:owner")
				} else {
					answer(open[0].ID)
				}
			}
		}
	}
}

func mailTo(to, day string) map[string]any {
	return map[string]any{"to": to, "subject": "Resumo de " + day, "body": "Hoje, " + day + ", nada novo."}
}

func (g *grantHouse) lastAction(t *testing.T) host.ActionRecord {
	t.Helper()
	evs, _ := g.Events.List(context.Background(), event.Query{Types: []string{host.ActionEvent}, Newest: true, Limit: 1})
	var rec host.ActionRecord
	evs[0].Decode(&rec)
	return rec
}

func TestApproveForThisRoutine(t *testing.T) {
	g := newGrantHouse(t)
	approve := func(id string) {
		if code, out := g.do(t, "POST", "/api/approvals/"+id+"/routine", nil); code != 200 {
			t.Errorf("answer %d %v", code, out)
		}
	}
	if asked, err := g.call(t, people.OwnerID, "gmail.send", mailTo("ana@x.com", "segunda"), approve); !asked || err != nil {
		t.Fatalf("first send: asked %v, %v", asked, err)
	}

	// The next run sends the same kind of email to the same person with
	// new words: allowed by the grant, and the log says so.
	if asked, err := g.call(t, people.OwnerID, "gmail.send", mailTo("ANA@x.com", "terça"), nil); asked || err != nil {
		t.Fatalf("the exact repeat asked again (%v) or failed: %v", asked, err)
	}
	if rec := g.lastAction(t); rec.Verdict != policy.Allow || !strings.HasPrefix(rec.Rule, "grant:") {
		t.Fatalf("not recorded as allowed by the grant: %+v", rec)
	}

	// Anyone else asks as usual.
	if asked, _ := g.call(t, people.OwnerID, "gmail.send", mailTo("eve@evil.com", "terça"), nil); !asked {
		t.Fatal("a new recipient did not ask")
	}
	_, list := g.do(t, "GET", "/api/grants", nil)
	items := list["list"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["routine_name"] != "Resumo" {
		t.Fatalf("grants %v", list)
	}

	// A new version of the routine asks again, and its old grant is gone.
	g.saveRoutine(t)
	if asked, _ := g.call(t, people.OwnerID, "gmail.send", mailTo("ana@x.com", "quarta"), nil); !asked {
		t.Fatal("a new version of the routine used the old grant")
	}
	if _, list := g.do(t, "GET", "/api/grants", nil); len(list["list"].([]any)) != 0 {
		t.Fatalf("an expired grant is still listed: %v", list)
	}
}

func TestRevokingAGrantAsksAgain(t *testing.T) {
	g := newGrantHouse(t)
	g.call(t, people.OwnerID, "gmail.send", mailTo("ana@x.com", "segunda"), func(id string) { g.do(t, "POST", "/api/approvals/"+id+"/routine", nil) })
	_, list := g.do(t, "GET", "/api/grants", nil)
	id := list["list"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := g.do(t, "DELETE", "/api/grants/"+id, nil); code != 200 {
		t.Fatalf("revoke %d", code)
	}
	if asked, _ := g.call(t, people.OwnerID, "gmail.send", mailTo("ana@x.com", "terça"), nil); !asked {
		t.Fatal("a revoked grant still applied")
	}
}

// The owner's grant does not carry over to someone else's run, a member
// cannot see or revoke it, and a member's answer on channels works for
// their own routine.
func TestGrantsArePerPerson(t *testing.T) {
	g := newGrantHouse(t)
	ctx := context.Background()
	g.call(t, people.OwnerID, "gmail.send", mailTo("ana@x.com", "segunda"), func(id string) { g.do(t, "POST", "/api/approvals/"+id+"/routine", nil) })
	_, list := g.do(t, "GET", "/api/grants", nil)
	id := list["list"].([]any)[0].(map[string]any)["id"].(string)

	_, out := g.do(t, "POST", "/api/people", map[string]string{"name": "Ana", "role": "member"})
	ana := out["id"].(string)
	link, _ := g.invite(t, ana, "Celular da Ana")
	_, token := g.open(t, link)
	if code, body := g.raw(t, token, "GET", "/api/grants", nil); code != 200 || strings.Contains(body, id) {
		t.Fatalf("Ana sees the owner's grants: %d %s", code, body)
	}
	if code, _ := g.raw(t, token, "DELETE", "/api/grants/"+id, nil); code != 404 {
		t.Fatalf("Ana revoked the owner's grant: %d", code)
	}

	// The routine now works for Ana: the owner's grant is not hers.
	g.Store.SetRoutinePerson(ctx, "brief", ana)
	answerAsAna := func(id string) {
		if !g.Approvals.MayAnswer(id, ana) {
			t.Error("Ana does not answer for her routine")
		}
		out, err := handler{g.App}.Button(people.With(ctx, ana), "grant", id)
		if err != nil || out == "" {
			t.Errorf("grant button: %q %v", out, err)
		}
	}
	if asked, err := g.call(t, ana, "gmail.send", mailTo("ana@x.com", "terça"), answerAsAna); !asked || err != nil {
		t.Fatalf("Ana's run used the owner's grant (asked %v) or failed: %v", asked, err)
	}
	if asked, _ := g.call(t, ana, "gmail.send", mailTo("ana@x.com", "quarta"), nil); asked {
		t.Fatal("Ana's own grant did not apply")
	}
	if n := len(g.Grants.Mine(ctx, ana)); n != 1 {
		t.Fatalf("Ana has %d grants", n)
	}
}

// Messages to other people on WhatsApp always ask: the choice is not
// offered, answering with it is refused, and a stored grant is ignored.
func TestAlwaysAskIgnoresGrants(t *testing.T) {
	g := newGrantHouse(t)
	ctx := context.Background()
	args := map[string]any{"to": "+5511988887777", "text": "Chego às 19h"}
	var grantable bool
	refuse := func(id string) {
		req, _ := g.Approvals.Get(id)
		grantable = req.Grantable
		if code, _ := g.do(t, "POST", "/api/approvals/"+id+"/routine", nil); code != 403 {
			t.Errorf("the grant answer was accepted: %d", code)
		}
		g.Approvals.Resolve(ctx, id, approval.Once, "human:owner")
	}
	if asked, err := g.call(t, people.OwnerID, "whatsapp.send_to", args, refuse); !asked || err != nil || grantable {
		t.Fatalf("asked %v grantable %v: %v", asked, grantable, err)
	}
	rt, _ := g.Store.Routine(ctx, "brief")
	forged := approval.Grant{ID: "forged", Person: people.OwnerID, Routine: "brief", Version: rt.Version, Capability: "whatsapp.send_to",
		Match: approval.OperationOf("whatsapp.send_to", args).Fields}
	g.Grants.Add(ctx, forged, "human:owner")
	if asked, _ := g.call(t, people.OwnerID, "whatsapp.send_to", args, nil); !asked {
		t.Fatal("whatsapp.send_to went through on a grant")
	}
	b, _ := json.Marshal(g.lastAction(t))
	if strings.Contains(string(b), "grant:") {
		t.Fatalf("recorded as granted: %s", b)
	}
}

// Numbered channels offer "for this routine" but never "always".
func TestNumbersOfferTheRoutineChoice(t *testing.T) {
	acts := []explore.Action{{Label: "Permitir", Data: "approve:x"}, {Label: "Para esta rotina", Data: "grant:x"}, {Label: "Sempre", Data: "always:x"}, {Label: "Negar", Data: "deny:x"}}
	choices, line := numbered(context.Background(), acts)
	if len(choices) != 3 || !strings.Contains(line, "2 = Para esta rotina") || strings.Contains(line, "Sempre") {
		t.Fatalf("%q", line)
	}
}

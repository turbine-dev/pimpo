package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/policy"
)

// history lists the history as ta's owner sees it, optionally of one area.
func (ta *testApp) settingsHistory(t *testing.T, area string) []historyView {
	t.Helper()
	code, body := ta.raw(t, "tok", "GET", "/api/history?area="+area, nil)
	if code != 200 {
		t.Fatalf("history %d %s", code, body)
	}
	var out []historyView
	json.Unmarshal([]byte(body), &out)
	return out
}

func fieldOf(v historyView, name string) (histField, bool) {
	for _, f := range v.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return histField{}, false
}

func TestHistoryRecordsEveryArea(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ta.do(t, "PUT", "/api/settings", Settings{Zone: "Europe/Lisbon", JudgeBackend: "local"})
	ta.do(t, "PUT", "/api/settings", Settings{Zone: "Europe/Lisbon", JudgeBackend: "jev"})
	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 3})
	ta.do(t, "PUT", "/api/rules/preset", map[string]string{"preset": "conservative"})
	ta.do(t, "PUT", "/api/connections/mail", map[string]string{"user": "me@example.com", "password": "app pass word"})
	ta.do(t, "PUT", "/api/models/keys/openai", map[string]string{"key": "sk-live-123"})
	_, p := ta.do(t, "POST", "/api/people", map[string]string{"name": "Bia", "role": "member"})
	ta.do(t, "PUT", "/api/people/"+p["id"].(string), map[string]string{"role": "guest"})

	all := ta.settingsHistory(t, "")
	areas := []string{}
	for _, h := range all {
		areas = append(areas, h.Area)
		if h.Actor != "human:owner" || h.Time.IsZero() {
			t.Fatalf("who and when missing: %+v", h)
		}
	}
	// Newest first.
	// The first save also sets the models from their defaults.
	want := "people people models connections rules budget models models settings"
	if got := strings.Join(areas, " "); !strings.HasPrefix(got, want) {
		t.Fatalf("areas %q, want %q first", got, want)
	}
	if f, ok := fieldOf(ta.settingsHistory(t, "models")[1], "judge_backend"); !ok || string(f.Before) != `"local"` || string(f.After) != `"jev"` {
		t.Fatalf("judge backend change %+v", f)
	}
	if f, _ := fieldOf(ta.settingsHistory(t, "budget")[0], "budget.daily_usd"); string(f.After) != `"3.00"` {
		t.Fatalf("budget change %+v", f)
	}
	rules := ta.settingsHistory(t, "rules")[0]
	if f, ok := fieldOf(rules, "preset"); !ok || string(f.After) != `"conservative"` {
		t.Fatalf("preset change %+v", rules)
	}
	mail := ta.settingsHistory(t, "connections")[0]
	if f, _ := fieldOf(mail, "mail.user"); string(f.After) != `"me@example.com"` {
		t.Fatalf("mail user %+v", mail)
	}
	if f, _ := fieldOf(mail, "vault:mail.password"); f.Secret != "added" || f.Before != nil || f.After != nil {
		t.Fatalf("mail password %+v", f)
	}
	if len(mail.Reenter) != 1 || mail.Reenter[0] != "mail.password" {
		t.Fatalf("reenter %v", mail.Reenter)
	}
	role := ta.settingsHistory(t, "people")[0]
	if f, _ := fieldOf(role, "role"); string(f.Before) != `"member"` || string(f.After) != `"guest"` || !role.Undoable {
		t.Fatalf("role change %+v", role)
	}
	if added := ta.settingsHistory(t, "people")[1]; added.Undoable {
		t.Fatalf("adding a person offered undo: %+v", added)
	}
	// A change that changes nothing is not recorded.
	n := len(all)
	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 3})
	if len(ta.settingsHistory(t, "")) != n {
		t.Fatal("recorded a change that changed nothing")
	}
}

func TestHistoryNeverKeepsSecrets(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	secrets := []string{"JEVSECRET1", "MAILSECRET2", "sk-OPENAI3", "LINKSECRET4", "ELEVEN5"}
	ta.do(t, "PUT", "/api/connections/jev", map[string]string{"key": secrets[0]})
	ta.do(t, "PUT", "/api/connections/mail", map[string]string{"user": "me@example.com", "password": secrets[1]})
	ta.do(t, "PUT", "/api/models/keys/openai", map[string]string{"key": secrets[2]})
	ta.do(t, "PUT", "/api/models/keys/openai", map[string]string{"key": secrets[2] + "b"})
	ta.do(t, "PUT", "/api/models/keys/openai", map[string]string{"key": ""})
	// A link with a password in it is a secret too.
	ta.do(t, "PUT", "/api/settings", Settings{Zone: "UTC", JudgeBackend: "local", CustomURL: "https://me:" + secrets[3] + "@llm.example.com"})
	ta.Vault.Set(ctx, "voice.elevenlabs.key", secrets[4])
	ta.do(t, "PUT", "/api/connections/mail", map[string]string{"user": "me2@example.com", "password": secrets[1] + "x"})

	evs, _ := ta.Events.List(ctx, event.Query{Types: []string{historyEvent}})
	for _, e := range evs {
		for _, s := range secrets {
			if strings.Contains(string(e.Data), s) {
				t.Fatalf("history event %d keeps a secret: %s", e.ID, e.Data)
			}
		}
	}
	models := ta.settingsHistory(t, "models")
	states := []string{}
	for _, h := range models {
		if f, ok := fieldOf(h, "vault:model.openai.key"); ok {
			states = append(states, f.Secret)
			if h.Undoable {
				t.Fatalf("a secret-only change offered undo: %+v", h)
			}
		}
	}
	if strings.Join(states, " ") != "removed replaced added" {
		t.Fatalf("key states %v", states)
	}
	if f, ok := fieldOf(models[0], "custom_url"); !ok || f.Secret == "" || f.After != nil {
		t.Fatalf("a link with a password was kept: %+v", models[0])
	}
	// Undo of a secret-only change is refused and says to type it again.
	code, body := ta.raw(t, "tok", "POST", fmt.Sprintf("/api/history/%d/undo", models[1].ID), nil)
	if code != 409 || !strings.Contains(body, "not_undoable") {
		t.Fatalf("undo of a secret %d %s", code, body)
	}
}

func TestHistoryUndo(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	undo := func(id int64) (int, string) {
		return ta.raw(t, "tok", "POST", fmt.Sprintf("/api/history/%d/undo", id), nil)
	}

	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 2})
	ta.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 5})
	b := ta.settingsHistory(t, "budget")
	// The older change was changed again since: the newer goes first.
	if code, body := undo(b[1].ID); code != 409 || !strings.Contains(body, "changed_since") {
		t.Fatalf("undo under a newer change %d %s", code, body)
	}
	if code, body := undo(b[0].ID); code != 200 {
		t.Fatalf("undo %d %s", code, body)
	}
	if ta.Budget.Limit(ctx) != 2 {
		t.Fatalf("budget after undo %v", ta.Budget.Limit(ctx))
	}
	if code, body := undo(b[0].ID); code != 409 || !strings.Contains(body, "undone") {
		t.Fatalf("undo twice %d %s", code, body)
	}
	b = ta.settingsHistory(t, "budget")
	if !b[1].Undone || b[1].Undoable || b[0].UndoOf != b[1].ID || !b[0].Undoable {
		t.Fatalf("after undo %+v", b)
	}

	// Settings: only the fields changed go back.
	ta.do(t, "PUT", "/api/settings", Settings{Zone: "Europe/Lisbon", JudgeBackend: "local", Locale: "pt-BR"})
	ta.do(t, "PUT", "/api/settings", Settings{Zone: "Asia/Tokyo", JudgeBackend: "local", Locale: "pt-BR", SuggestOff: true})
	if code, body := undo(ta.settingsHistory(t, "settings")[0].ID); code != 200 {
		t.Fatalf("undo settings %d %s", code, body)
	}
	if s := ta.Settings(ctx); s.Zone != "Europe/Lisbon" || s.SuggestOff || s.Locale != "pt-BR" {
		t.Fatalf("settings after undo %+v", s)
	}

	// Rules: a rule added goes away, one changed comes back.
	first := ta.Rules.Rules(ctx)
	rule := policy.Rule{ID: "r-mail", Text: "ask before email", Then: policy.Ask, When: policy.When{Capabilities: []string{"gmail.send"}}}
	ta.do(t, "PUT", "/api/rules", []policy.Rule{rule})
	rule.Then = policy.Block
	ta.do(t, "PUT", "/api/rules", []policy.Rule{rule})
	undo(ta.settingsHistory(t, "rules")[0].ID)
	if r := ta.Rules.Rules(ctx); len(r) != 1 || r[0].Then != policy.Ask {
		t.Fatalf("rules after undo %+v", r)
	}
	undo(ta.settingsHistory(t, "rules")[2].ID)
	if r := ta.Rules.Rules(ctx); len(r) != len(first) || slices.ContainsFunc(r, func(x policy.Rule) bool { return x.ID == "r-mail" }) {
		t.Fatalf("rules not back as they were after undo %+v", r)
	}

	// People: a role goes back, and the person learns of it.
	_, p := ta.do(t, "POST", "/api/people", map[string]string{"name": "Bia", "role": "member"})
	id := p["id"].(string)
	ta.do(t, "PUT", "/api/people/"+id, map[string]string{"role": "guest"})
	if code, body := undo(ta.settingsHistory(t, "people")[0].ID); code != 200 {
		t.Fatalf("undo role %d %s", code, body)
	}
	if q, _ := ta.People.Get(ctx, id); q.Role != "member" {
		t.Fatalf("role after undo %v", q.Role)
	}

	// A connection: the address goes back, the password is typed again.
	ta.do(t, "PUT", "/api/connections/mail", map[string]string{"user": "old@example.com", "password": "p1"})
	ta.do(t, "PUT", "/api/connections/mail", map[string]string{"user": "new@example.com", "password": "p2"})
	code, body := undo(ta.settingsHistory(t, "connections")[0].ID)
	if code != 200 || !strings.Contains(body, "mail.password") {
		t.Fatalf("undo mail %d %s", code, body)
	}
	if u, _ := ta.Events.Get(ctx, "mail.user"); u != "old@example.com" {
		t.Fatalf("mail user after undo %q", u)
	}
	if pw, _ := ta.Vault.Get(ctx, "mail.password"); pw != "p2" {
		t.Fatalf("undo touched the password")
	}
}

func TestHistoryIsEachPersonsOwn(t *testing.T) {
	h := newHouse(t)
	ana := h.ana
	h.do(t, "PUT", "/api/budget", map[string]float64{"daily_usd": 4})
	code, body := h.raw(t, ana, "PUT", "/api/people/"+h.anaID+"/connections/mail", js(map[string]string{"user": "ana@example.com", "password": "ANAPASS"}))
	if code != 200 {
		t.Fatalf("Ana's mail %d %s", code, body)
	}

	// Ana sees her own change, and nothing of the house settings.
	code, body = h.raw(t, ana, "GET", "/api/history", nil)
	var mine []historyView
	json.Unmarshal([]byte(body), &mine)
	if code != 200 || len(mine) != 1 || mine[0].Area != "connections" || mine[0].Actor != "human:"+h.anaID {
		t.Fatalf("Ana's history %d %s", code, body)
	}
	if strings.Contains(body, "budget") || strings.Contains(body, "ANAPASS") {
		t.Fatalf("Ana's history shows too much: %s", body)
	}
	// The owner does not see Ana's accounts, not even that they changed.
	for _, v := range h.settingsHistory(t, "") {
		if v.Area == "connections" || strings.Contains(fmt.Sprint(v.Fields), "ana@example.com") {
			t.Fatalf("the owner sees Ana's change: %+v", v)
		}
	}
	if code, _ := h.raw(t, "tok", "POST", fmt.Sprintf("/api/history/%d/undo", mine[0].ID), nil); code != 404 {
		t.Fatalf("the owner undid Ana's change: %d", code)
	}
	// Nor may Ana undo the owner's.
	owners := h.settingsHistory(t, "budget")
	if code, _ := h.raw(t, ana, "POST", fmt.Sprintf("/api/history/%d/undo", owners[0].ID), nil); code != 404 {
		t.Fatalf("Ana undid the owner's change: %d", code)
	}
}

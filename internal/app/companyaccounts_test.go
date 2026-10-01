package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/policy"
)

// asMember is ctx as a call of a member for a capability, as the host
// makes it.
func asMember(member, capability string) context.Context {
	ctx := host.WithMember(context.Background(), member)
	return host.WithCapability(ctx, capability)
}

func TestAMemberUsesOnlyItsOwnAccounts(t *testing.T) {
	ta, co := clerk(t)
	ctx := context.Background()
	if code, out := ta.do(t, "PUT", "/api/catalog/github", map[string]string{"token": "owner-token"}); code != 200 {
		t.Fatalf("owner's GitHub: %d %v", code, out)
	}
	github := ta.catalogConfig("github")
	var missing *connector.MissingCredential
	if v, err := github(asMember(co+"/clara", "github.issues"), "token"); !errors.As(err, &missing) || v == "owner-token" {
		t.Fatalf("Clara reached the owner's GitHub: %q %v", v, err)
	}
	if code, out := ta.do(t, "PUT", "/api/companies/"+co+"/members/clara/accounts/github", map[string]string{"token": "clara-token", "account_kind": "machine"}); code != 200 || out["warning"] != "" {
		t.Fatalf("Clara's GitHub: %d %v", code, out)
	}
	if v, _ := github(asMember(co+"/clara", "github.issues"), "token"); v != "clara-token" {
		t.Fatalf("Clara's own token = %q", v)
	}
	if v, err := github(asMember(co+"/bia", "github.issues"), "token"); err == nil {
		t.Fatalf("Bia reached Clara's GitHub: %q", v)
	}
	if v, _ := github(ctx, "token"); v != "owner-token" {
		t.Fatalf("the owner's own token = %q", v)
	}
	_, out := ta.do(t, "GET", "/api/companies/"+co+"/members/clara/accounts", nil)
	if !strings.Contains(stringify(out), `"configured":true`) || strings.Contains(stringify(out), "clara-token") {
		t.Fatalf("accounts view = %v", out)
	}
}

func TestASharedAccountGoesAsFarAsItsGrant(t *testing.T) {
	ta, co := clerk(t)
	ctx := context.Background()
	ta.do(t, "PUT", "/api/companies/"+co+"/accounts/github", map[string]string{"token": "company-token"})
	o, _ := ta.Companies.Org(ctx, co)
	o.Accounts = []company.SharedAccount{{Kind: "github", Grants: map[string]string{"bia": company.GrantRead, "clara": company.GrantAct}}}
	if _, err := ta.Companies.Update(ctx, o.Company); err != nil {
		t.Fatal(err)
	}
	github := ta.catalogConfig("github")
	for _, c := range []struct {
		member, capability, want string
	}{
		{"bia", "github.issues", "company-token"},
		{"bia", "github.comment", ""},
		{"clara", "github.comment", "company-token"},
	} {
		v, _ := github(asMember(co+"/"+c.member, c.capability), "token")
		if v != c.want {
			t.Errorf("%s %s = %q", c.member, c.capability, v)
		}
	}
}

func TestMembersStayOutOfTheirPersonsLife(t *testing.T) {
	ta, co := clerk(t)
	ctx := context.Background()
	for _, c := range []string{"phone.arrivals", "apple.notes.search", "spotify.play", "ha.call", "sheets.read", "reminder.list"} {
		act := policy.Action{Capability: c, Risk: capability.Read, Source: "exploration:x", Member: co + "/clara"}
		if d := ta.decide(ctx, act); d.Verdict != policy.Block {
			t.Errorf("%s: %+v", c, d)
		}
	}
	if personal(host.WithMember(ctx, co+"/clara"), "mail.addr") == "mail.addr" {
		t.Fatal("a member read the owner's mailbox settings")
	}
	if ta.memberBrowser(co+"/clara") == ta.theBrowser() || ta.memberBrowser(co+"/clara").Profile == ta.memberBrowser(co+"/bia").Profile {
		t.Fatal("members share a browser profile")
	}
}

func TestAMemberWaitsForTheAccountsItsRoleNeeds(t *testing.T) {
	b := newBlock()
	ta, co := shopApp(t, b)
	ctx := context.Background()
	ta.do(t, "PUT", "/api/companies/"+co+"/roles/clerk", map[string]any{"title": "Clerk", "capabilities": []string{"github.issues"}, "account_kinds": []string{"github"}})
	o, _ := ta.Companies.Org(ctx, co)
	w, _ := ta.enqueue(ctx, o, "clara", "Triage the issues", nil, "test", 0)
	ta.pumpWork(ctx)
	if got, _ := ta.Companies.Work(ctx, w.ID); got.State != company.WorkQueued {
		t.Fatalf("work started without its account: %+v", got)
	}
	_, view := ta.do(t, "GET", "/api/companies/"+co, nil)
	if !strings.Contains(stringify(view["activity"]), "account_missing") {
		t.Fatalf("activity = %v", view["activity"])
	}
	_, out := ta.do(t, "PUT", "/api/companies/"+co+"/members/clara/accounts/github", map[string]string{"token": "t", "account_kind": "brand"})
	if !strings.Contains(out["warning"].(string), "terms") {
		t.Fatalf("no warning for a brand account on GitHub: %v", out)
	}
	ta.pumpWork(ctx)
	<-b.started
	close(b.release)
	ta.waitWorkState(t, w.ID, company.WorkDone)
}

func stringify(v any) string { return string(js(v)) }

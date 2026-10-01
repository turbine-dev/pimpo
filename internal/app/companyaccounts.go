package app

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/policy"
	"github.com/turbine-dev/pimpo/internal/server"
)

// A company member has accounts of its own, kept under its own names in
// the vault and settings, and reaches the company's shared accounts it
// was granted. It never reaches its person's accounts, nor the parts of
// its person's life Pimpo connects to: the phone, the Mac's apps, the
// house, the person's Google, music and notes.

// memberKey names a member's own setting or secret.
func memberKey(member, name string) string {
	co, id, _ := strings.Cut(member, "/")
	return "member." + co + "." + id + "." + name
}

// companyKey names a company's shared setting or secret.
func companyKey(co, name string) string { return "company." + co + "." + name }

// personalOnly are capabilities that reach the person's own life, which a
// company member never uses.
var personalOnly = []string{"apple.", "phone.", "spotify.", "ha.", "obsidian.", "sheets.", "guard.", "reminder.list", "reminder.cancel"}

func isPersonalOnly(capability string) bool {
	return slices.ContainsFunc(personalOnly, func(p string) bool { return strings.HasPrefix(capability, p) })
}

// Kinds of account.
const (
	AccountBrand   = "brand"
	AccountMachine = "machine"
	AccountService = "service"
	AccountPerson  = "person"
)

// accountTerms are the kinds of account each service's terms accept for
// automation. A kind outside them is allowed with a warning.
var accountTerms = map[string][]string{
	"github":  {AccountMachine, AccountService, AccountPerson},
	"mail":    {AccountService, AccountBrand, AccountPerson},
	"slack":   {AccountService, AccountBrand},
	"discord": {AccountService, AccountBrand},
	"notion":  {AccountService, AccountPerson},
	"todoist": {AccountService, AccountPerson},
}

// socialNetworks only take a brand's account for an agent: a personal
// profile of someone who does not exist breaks their terms.
var socialNetworks = []string{"instagram", "tiktok", "linkedin", "x", "facebook", "youtube"}

func accountWarning(service, kind string) string {
	if slices.Contains(socialNetworks, service) && kind != AccountBrand {
		return "This network's terms allow automation only on a brand's account; a personal profile for an agent can be banned."
	}
	if accepted, ok := accountTerms[service]; ok && !slices.Contains(accepted, kind) {
		return "This service's terms may not allow automation on this kind of account."
	}
	return ""
}

// mailFields are what a member's own mailbox needs, as the mail connector
// reads it.
var mailFields = []services.Field{{Name: "addr", Label: "IMAP server", Placeholder: "imap.example.com:993"}, {Name: "user", Label: "User"},
	{Name: "smtp", Label: "SMTP server", Placeholder: "smtp.example.com:587", Optional: true}, {Name: "password", Label: "Password", Secret: true}}

func accountFields(kind string) ([]services.Field, string, bool) {
	if kind == "mail" {
		return mailFields, "mail.", true
	}
	k, ok := services.Get(kind)
	if !ok {
		return nil, "", false
	}
	return k.Fields, "conn." + kind + ".", true
}

type accountView struct {
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Fields      []services.Field  `json:"fields"`
	Configured  bool              `json:"configured"`
	Values      map[string]string `json:"values"`
	AccountKind string            `json:"account_kind,omitempty"`
	Shared      string            `json:"shared,omitempty"`
}

// accounts are the kinds a member can connect, with what is set.
func (a *App) accounts(ctx context.Context, o company.Org, member string) []accountView {
	kinds := []string{"mail"}
	for _, k := range services.All() {
		kinds = append(kinds, k.ID)
	}
	out := []accountView{}
	for _, kind := range kinds {
		fields, prefix, _ := accountFields(kind)
		v := accountView{Kind: kind, Name: kind, Fields: fields, Values: map[string]string{}}
		if k, ok := services.Get(kind); ok {
			v.Name = k.Title
		}
		any := false
		for _, f := range fields {
			key := memberKey(o.ID+"/"+member, prefix+f.Name)
			if f.Secret {
				if s, err := a.Vault.Get(ctx, key); err == nil && s != "" {
					any = true
				}
				continue
			}
			if s, _ := a.Events.Get(ctx, key); s != "" {
				v.Values[f.Name], any = s, true
			}
		}
		v.Configured = any
		v.AccountKind, _ = a.Events.Get(ctx, memberKey(o.ID+"/"+member, "account."+kind))
		if g := o.AccountGrant(kind, member); g != "" {
			v.Shared = g
		}
		out = append(out, v)
	}
	return out
}

// missingAccounts are the account kinds a member's role needs and it has
// neither of its own nor shared with it.
func (a *App) missingAccounts(ctx context.Context, o company.Org, member string) []string {
	m, _ := o.Member(member)
	role, _ := o.Role(m.Role)
	var out []string
	for _, kind := range role.AccountKinds {
		if _, _, known := accountFields(kind); !known {
			continue
		}
		if o.AccountGrant(kind, member) != "" {
			continue
		}
		if !a.hasAccount(ctx, o.ID+"/"+member, kind) {
			out = append(out, kind)
		}
	}
	return out
}

func (a *App) hasAccount(ctx context.Context, member, kind string) bool {
	fields, prefix, _ := accountFields(kind)
	for _, f := range fields {
		key := memberKey(member, prefix+f.Name)
		if f.Secret {
			if s, err := a.Vault.Get(ctx, key); err == nil && s != "" {
				return true
			}
		} else if s, _ := a.Events.Get(ctx, key); s != "" {
			return true
		}
	}
	return false
}

// putAccount writes a member's own account, or the company's shared one
// when member is "".
func (a *App) putAccount(ctx context.Context, o company.Org, member, kind string, in map[string]string) (map[string]string, error) {
	fields, prefix, ok := accountFields(kind)
	if !ok {
		return nil, server.StatusError{Status: 404, Msg: "unknown kind of account"}
	}
	key := func(name string) string {
		if member == "" {
			return companyKey(o.ID, name)
		}
		return memberKey(o.ID+"/"+member, name)
	}
	for _, f := range fields {
		v := strings.TrimSpace(in[f.Name])
		if v == "" {
			continue
		}
		var err error
		if f.Secret {
			err = a.Vault.Set(ctx, key(prefix+f.Name), v)
		} else {
			err = a.Events.Put(ctx, key(prefix+f.Name), v)
		}
		if err != nil {
			return nil, err
		}
	}
	kindOf := in["account_kind"]
	if kindOf == "" {
		kindOf = AccountService
	}
	if !slices.Contains([]string{AccountBrand, AccountMachine, AccountService, AccountPerson}, kindOf) {
		return nil, server.StatusError{Status: 400, Msg: "an account is a brand's, a machine's, a service's or a person's"}
	}
	a.Events.Put(ctx, key("account."+kind), kindOf)
	a.Events.Append(ctx, "company.account.set", actor(ctx), map[string]string{"company": o.ID, "member": member, "kind": kind, "person": o.Person})
	return map[string]string{"kind": kind, "warning": accountWarning(kind, kindOf)}, nil
}

func (a *App) deleteAccount(ctx context.Context, o company.Org, member, kind string) error {
	fields, prefix, ok := accountFields(kind)
	if !ok {
		return server.StatusError{Status: 404, Msg: "unknown kind of account"}
	}
	for _, f := range append(fields, services.Field{Name: "account"}) {
		name := prefix + f.Name
		if f.Name == "account" {
			name = "account." + kind
		}
		k := memberKey(o.ID+"/"+member, name)
		if member == "" {
			k = companyKey(o.ID, name)
		}
		a.Vault.Delete(ctx, k)
		a.Events.DB().ExecContext(ctx, `DELETE FROM kv WHERE key = ?`, k)
	}
	a.Events.Append(ctx, "company.account.removed", actor(ctx), map[string]string{"company": o.ID, "member": member, "kind": kind, "person": o.Person})
	return nil
}

// sharedConfig reads a catalog field from a company's shared account, for
// a member it was granted to, at the grant's reach.
func (a *App) sharedConfig(ctx context.Context, kind, field string, secret bool) (string, bool) {
	member := host.MemberOf(ctx)
	co, id, ok := strings.Cut(member, "/")
	if !ok {
		return "", false
	}
	o, err := a.Companies.Org(ctx, co)
	if err != nil {
		return "", false
	}
	grant := o.AccountGrant(kind, id)
	spec := capability.Catalog[host.CapabilityOf(ctx)]
	if grant == "" || grant == company.GrantRead && spec.Risk != capability.Read {
		return "", false
	}
	name := companyKey(co, catalogKey(kind, field))
	if secret {
		v, err := a.Vault.Get(ctx, name)
		return v, err == nil && v != ""
	}
	v, _ := a.Events.Get(ctx, name)
	return v, v != ""
}

// personalLife blocks what reaches the person's own life for a member.
func personalLife(act policy.Action) (policy.Decision, bool) {
	if act.Member != "" && isPersonalOnly(act.Capability) {
		return policy.Decision{Verdict: policy.Block, Reason: "this reaches the person's own life; a company member uses accounts of its own"}, true
	}
	return policy.Decision{}, false
}

func (a *App) companyAccountRoutes() {
	a.Server.Handle("GET /api/companies/{id}/members/{part}/accounts", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		if err := o.CheckWork(r.PathValue("part")); err != nil {
			return nil, err
		}
		return map[string]any{"accounts": a.accounts(r.Context(), o, r.PathValue("part")), "missing": a.missingAccounts(r.Context(), o, r.PathValue("part"))}, nil
	}))
	a.Server.Handle("PUT /api/companies/{id}/members/{part}/accounts/{kind}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		if err := o.CheckWork(r.PathValue("part")); err != nil {
			return nil, err
		}
		var in map[string]string
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		return a.putAccount(r.Context(), o, r.PathValue("part"), r.PathValue("kind"), in)
	}))
	a.Server.Handle("DELETE /api/companies/{id}/members/{part}/accounts/{kind}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return map[string]bool{"ok": true}, a.deleteAccount(r.Context(), o, r.PathValue("part"), r.PathValue("kind"))
	}))
	a.Server.Handle("PUT /api/companies/{id}/accounts/{kind}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var in map[string]string
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		return a.putAccount(r.Context(), o, "", r.PathValue("kind"), in)
	}))
	a.Server.Handle("DELETE /api/companies/{id}/accounts/{kind}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return map[string]bool{"ok": true}, a.deleteAccount(r.Context(), o, "", r.PathValue("kind"))
	}))
}

package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// Companies of agents (RFC 0004) belong to the person who made them, and
// to the partners they share them with, by grant. Nobody else, the
// administrator included, finds them. They are a lab that starts off.

const companiesLab = "companies"

// orgView is a company as the person asking sees it: what they may do,
// what each member is doing now and the routines members have.
type orgView struct {
	company.Org
	Grant    string                    `json:"grant"`
	Activity map[string]memberActivity `json:"activity"`
	Routines []map[string]string       `json:"routines"`
}

func (a *App) view(ctx context.Context, o company.Org) orgView {
	return orgView{o, o.Grant(people.From(ctx)), a.activity(ctx, o), a.memberRoutines(ctx, o)}
}

func (a *App) companyRoutes() {
	a.Server.Handle("GET /api/companies", a.listCompanies)
	a.Server.Handle("POST /api/companies", a.createCompany)
	a.Server.Handle("POST /api/companies/import", a.importCompany)
	a.Server.Handle("GET /api/companies/{id}", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return o, nil
	}))
	a.Server.Handle("PUT /api/companies/{id}", a.companyRoute(company.Configure, a.putCompany))
	a.Server.Handle("DELETE /api/companies/{id}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		if o.Person != people.From(r.Context()) {
			return nil, server.StatusError{Status: 403, Msg: "only whoever made the company may delete it"}
		}
		if err := a.Companies.Delete(r.Context(), o.ID); err != nil {
			return nil, err
		}
		a.dropSpaces(o.ID)
		return map[string]bool{"ok": true}, nil
	}))
	a.Server.Handle("GET /api/companies/{id}/export", a.exportCompany)
	a.Server.Handle("PUT /api/companies/{id}/departments/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		var d company.Department
		if err := server.Decode(r, &d); err != nil {
			return nil, err
		}
		d.ID, d.Name = r.PathValue("part"), strings.TrimSpace(d.Name)
		return a.Companies.SaveDepartment(r.Context(), o.ID, d)
	}))
	a.Server.Handle("DELETE /api/companies/{id}/departments/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.DeleteDepartment(r.Context(), o.ID, r.PathValue("part"))
	}))
	a.Server.Handle("PUT /api/companies/{id}/roles/{part}", a.companyRoute(company.Configure, a.putRole))
	a.Server.Handle("DELETE /api/companies/{id}/roles/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.DeleteRole(r.Context(), o.ID, r.PathValue("part"))
	}))
	a.Server.Handle("PUT /api/companies/{id}/members/{part}", a.companyRoute(company.Configure, a.putMember))
	a.Server.Handle("DELETE /api/companies/{id}/members/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.DeleteMember(r.Context(), o.ID, r.PathValue("part"))
	}))
	a.Server.Handle("GET /api/companies/{id}/members/{part}/preview", a.companyRoute(company.View, a.previewMember))
	a.Server.Handle("PUT /api/companies/{id}/contexts/{part}", a.companyRoute(company.Configure, a.putContext))
	a.Server.Handle("DELETE /api/companies/{id}/contexts/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.DeleteContext(r.Context(), o.ID, r.PathValue("part"))
	}))
	a.Server.Handle("PUT /api/companies/{id}/rules/{part}", a.companyRoute(company.Configure, a.putCompanyRule))
	a.Server.Handle("DELETE /api/companies/{id}/rules/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.DeleteRule(r.Context(), o.ID, r.PathValue("part"))
	}))
}

// companiesOn refuses when the lab is off.
func (a *App) companiesOn(w http.ResponseWriter, r *http.Request) bool {
	if !a.chose(r.Context(), companiesLab) {
		server.WriteError(w, server.StatusError{Status: 403, Msg: "companies are turned off in Settings › Labs"})
		return false
	}
	return true
}

// myCompany is a company the person asking may reach with need; anyone
// else's does not exist for them, and a partner without the grant is told so.
func (a *App) myCompany(ctx context.Context, id, need string) (company.Org, error) {
	o, err := a.Companies.Org(ctx, id)
	if err != nil {
		return company.Org{}, err
	}
	grant := o.Grant(people.From(ctx))
	switch {
	case grant == "":
		return company.Org{}, company.ErrNotFound
	case !company.Allows(grant, need):
		return company.Org{}, server.StatusError{Status: 403, Msg: "you may " + grant + " this company, not " + need + " it"}
	}
	return o, nil
}

// companyRoute loads the company for a route, refuses what the person may
// not do, runs f and answers with what it returns, or with the company as
// it is now after a change.
func (a *App) companyRoute(need string, f func(http.ResponseWriter, *http.Request, company.Org) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.companiesOn(w, r) {
			return
		}
		ctx := r.Context()
		o, err := a.myCompany(ctx, r.PathValue("id"), need)
		if err == nil {
			var out any
			if out, err = f(w, r, o); err == nil {
				if changed, ok := out.(company.Org); ok {
					if r.Method != "GET" {
						a.companyChanged(ctx, changed)
					}
					out = a.view(ctx, changed)
				}
				server.WriteJSON(w, 200, out)
				return
			}
		}
		server.WriteError(w, companyError(err))
	}
}

func companyError(err error) error {
	var se server.StatusError
	switch {
	case errors.As(err, &se):
		return err
	case errors.Is(err, company.ErrNotFound):
		return server.StatusError{Status: 404, Msg: "no such company"}
	case errors.Is(err, company.ErrException):
		return server.StatusError{Status: 409, Msg: err.Error()}
	}
	return server.StatusError{Status: 400, Msg: err.Error()}
}

func (a *App) companyChanged(ctx context.Context, o company.Org) {
	a.Events.Append(ctx, "company.changed", actor(ctx), map[string]string{"id": o.ID, "person": o.Person})
	a.holdWork(ctx, o)
	a.scheduleAgents(ctx)
	go a.pumpWork(context.WithoutCancel(ctx))
}

func (a *App) listCompanies(w http.ResponseWriter, r *http.Request) {
	if !a.companiesOn(w, r) {
		return
	}
	all, err := a.Companies.List(r.Context())
	if err != nil {
		server.WriteError(w, err)
		return
	}
	me := people.From(r.Context())
	type item struct {
		company.Company
		Grant string `json:"grant"`
	}
	out := []item{}
	for _, c := range all {
		if g := c.Grant(me); g != "" {
			if me != c.Person {
				c.Partners = nil
			}
			out = append(out, item{c, g})
		}
	}
	server.WriteJSON(w, 200, out)
}

func (a *App) createCompany(w http.ResponseWriter, r *http.Request) {
	if !a.companiesOn(w, r) {
		return
	}
	ctx := r.Context()
	var c company.Company
	if err := server.Decode(r, &c); err != nil {
		server.WriteError(w, err)
		return
	}
	if err := checkCompanyText(&c); err != nil {
		server.WriteError(w, err)
		return
	}
	c.ID, c.Person, c.Partners = newCompanyID(), people.From(ctx), nil
	if c.Zone == "" {
		c.Zone = a.Settings(ctx).Zone
	}
	o, err := a.Companies.Create(ctx, c, a.nameOf(ctx))
	if err != nil {
		server.WriteError(w, companyError(err))
		return
	}
	a.companyChanged(ctx, o)
	server.WriteJSON(w, 201, a.view(ctx, o))
}

func (a *App) putCompany(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
	ctx := r.Context()
	var c company.Company
	if err := server.Decode(r, &c); err != nil {
		return nil, err
	}
	if err := checkCompanyText(&c); err != nil {
		return nil, err
	}
	c.ID = o.ID
	if people.From(ctx) != o.Person {
		// Partners never change who else the company is shared with.
		c.Partners = o.Partners
	}
	for _, p := range c.Partners {
		if _, err := a.People.Get(ctx, p.Person); err != nil && p.Person != people.OwnerID {
			return nil, server.StatusError{Status: 400, Msg: "partners are people of the house"}
		}
	}
	// A company never gets more than its person may spend; what it spends
	// is also checked against that limit and the house's at every call.
	if limit := a.Budget.LimitFor(ctx, o.Person); limit > 0 && (c.Budget.DayUSD > limit || c.Budget.MonthUSD > 31*limit) {
		return nil, server.StatusError{Status: 400, Msg: fmt.Sprintf("the company may spend at most its person's own limit, $%.2f a day", limit)}
	}
	return a.Companies.Update(ctx, c)
}

func checkCompanyText(c *company.Company) error {
	c.Name, c.Industry, c.Mission = strings.TrimSpace(c.Name), strings.TrimSpace(c.Industry), strings.TrimSpace(c.Mission)
	switch {
	case c.Name == "" || len([]rune(c.Name)) > 60:
		return server.StatusError{Status: 400, Msg: "give the company a name of up to 60 characters"}
	case len([]rune(c.Industry)) > 120:
		return server.StatusError{Status: 400, Msg: "keep the industry under 120 characters"}
	case len([]rune(c.Mission)) > 2000:
		return server.StatusError{Status: 400, Msg: "keep the mission under 2000 characters"}
	}
	return nil
}

func (a *App) putRole(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
	var role company.Role
	if err := server.Decode(r, &role); err != nil {
		return nil, err
	}
	role.ID, role.Title, role.Function = r.PathValue("part"), strings.TrimSpace(role.Title), strings.TrimSpace(role.Function)
	if len([]rune(role.Title)) > 60 || len([]rune(role.Function)) > 4000 {
		return nil, server.StatusError{Status: 400, Msg: "keep the title under 60 characters and the function under 4000"}
	}
	var err error
	if role.Capabilities, err = knownCapabilities(role.Capabilities); err != nil {
		return nil, err
	}
	if role.Models, err = a.houseChoice(r.Context(), role.Models); err != nil {
		return nil, err
	}
	return a.Companies.SaveRole(r.Context(), o.ID, role)
}

func (a *App) putMember(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
	ctx := r.Context()
	var m company.Member
	if err := server.Decode(r, &m); err != nil {
		return nil, err
	}
	m.ID, m.Name, m.Persona = r.PathValue("part"), strings.TrimSpace(m.Name), strings.TrimSpace(m.Persona)
	if len([]rune(m.Name)) > 60 || len([]rune(m.Persona)) > 2000 || len([]rune(m.Avatar)) > 4 {
		return nil, server.StatusError{Status: 400, Msg: "keep the name under 60 characters, the persona under 2000 and the avatar to one emoji"}
	}
	if m.Kind == "" {
		m.Kind = company.Agent
	}
	if m.State == "" {
		m.State = company.Active
	}
	if m.Kind == company.Person && m.ID != company.CEO {
		// A seat for someone of the house is theirs only once they are a
		// partner, so a company never names a stranger.
		if !slices.ContainsFunc(o.Partners, func(p company.Partner) bool { return p.Person == m.Person }) {
			return nil, server.StatusError{Status: 400, Msg: "a seat for a person goes to a partner of the company"}
		}
	}
	var err error
	if m.Capabilities, err = knownCapabilities(m.Capabilities); err != nil {
		return nil, err
	}
	if m.Models, err = a.houseChoice(ctx, m.Models); err != nil {
		return nil, err
	}
	return a.Companies.SaveMember(ctx, o.ID, m)
}

func knownCapabilities(list []string) ([]string, error) {
	out := []string{}
	for _, c := range list {
		if _, ok := capability.Catalog[c]; !ok {
			return nil, server.StatusError{Status: 400, Msg: "unknown capability " + c}
		}
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out, nil
}

func (a *App) exportCompany(w http.ResponseWriter, r *http.Request) {
	if !a.companiesOn(w, r) {
		return
	}
	o, err := a.myCompany(r.Context(), r.PathValue("id"), company.View)
	if err != nil {
		server.WriteError(w, companyError(err))
		return
	}
	b, err := o.Export()
	if err != nil {
		server.WriteError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", `attachment; filename="`+company.Slug(o.Name)+`.company.yaml"`)
	w.Write(b)
}

func (a *App) importCompany(w http.ResponseWriter, r *http.Request) {
	if !a.companiesOn(w, r) {
		return
	}
	ctx := r.Context()
	var in struct {
		File     string `json:"file"`
		Template string `json:"template"`
		Name     string `json:"name"`
	}
	if err := server.Decode(r, &in); err != nil {
		server.WriteError(w, err)
		return
	}
	file := []byte(in.File)
	if in.Template != "" {
		var ok bool
		if file, ok = company.TemplateFile(in.Template); !ok {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such template"})
			return
		}
	}
	o, err := company.Import(file, newCompanyID(), people.From(ctx), a.nameOf(ctx), time.Now())
	if err == nil && strings.TrimSpace(in.Name) != "" {
		o.Name = strings.TrimSpace(in.Name)
		err = checkCompanyText(&o.Company)
	}
	if err == nil && o.Zone == "" {
		o.Zone = a.Settings(ctx).Zone
	}
	if err == nil {
		for _, role := range o.Roles {
			if _, err = knownCapabilities(role.Capabilities); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = a.Companies.Replace(ctx, o)
	}
	if err != nil {
		server.WriteError(w, companyError(err))
		return
	}
	a.companyChanged(ctx, o)
	server.WriteJSON(w, 201, a.view(ctx, o))
}

func newCompanyID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return "co_" + hex.EncodeToString(b)
}

// forgetCompaniesOf deletes a removed person's companies and takes them
// out of the ones they were a partner in.
func (a *App) forgetCompaniesOf(ctx context.Context, person string) {
	all, _ := a.Companies.List(ctx)
	for _, c := range all {
		switch {
		case c.Person == person:
			a.Companies.Delete(ctx, c.ID)
			a.dropSpaces(c.ID)
		case c.Grant(person) != "":
			o, err := a.Companies.Org(ctx, c.ID)
			if err != nil {
				continue
			}
			for _, m := range o.Members {
				if m.Kind == company.Person && m.Person == person {
					a.Companies.DeleteMember(ctx, c.ID, m.ID)
				}
			}
			c.Partners = slices.DeleteFunc(c.Partners, func(p company.Partner) bool { return p.Person == person })
			a.Companies.Update(ctx, c)
		}
	}
}

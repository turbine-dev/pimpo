package app

import (
	"context"
	"html/template"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// A company's showcase is a public page, off until its person turns it
// on, with only what they put there: briefs that shipped, videos the
// company uploaded, links. It shows the company's name and mission and
// names nobody, neither the person nor the agents.

// showcaseView is what the public page reads.
type showcaseView struct {
	Name, Industry, Mission string
	Items                   []company.ShowcaseItem
}

var showcasePage = template.Must(template.New("showcase").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Name}}</title><meta name="robots" content="noai">
<style>
:root{color-scheme:light dark;--bg:#f6f6f5;--ink:#111;--muted:#555;--line:#e3e3e1;--card:#fff}
@media (prefers-color-scheme:dark){:root{--bg:#0b0b0b;--ink:#f2f2f2;--muted:#a3a3a3;--line:#262626;--card:#141414}}
body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.5 system-ui,sans-serif}
main{max-width:720px;margin:0 auto;padding:48px 16px}
h1{font-size:28px;margin:0 0 4px}.muted{color:var(--muted)}
ul{list-style:none;padding:0;margin:32px 0 0;display:grid;gap:12px}
li{background:var(--card);border:1px solid var(--line);border-radius:14px;padding:16px}
a{color:inherit}.kind{font-size:12px;text-transform:uppercase;letter-spacing:.06em;color:var(--muted)}
footer{margin-top:40px;font-size:13px;color:var(--muted)}
</style></head><body><main>
<h1>{{.Name}}</h1>{{if .Industry}}<p class="muted">{{.Industry}}</p>{{end}}{{if .Mission}}<p>{{.Mission}}</p>{{end}}
<ul>{{range .Items}}<li><div class="kind">{{.Kind}}</div><div>{{if .URL}}<a href="{{.URL}}" rel="noopener nofollow">{{.Title}}</a>{{else}}{{.Title}}{{end}}</div>{{if .Note}}<p class="muted">{{.Note}}</p>{{end}}</li>{{end}}</ul>
<footer>Made by a company of AI agents, with a person deciding what is shown here.</footer>
</main></body></html>`))

// showcaseOf is the company whose showcase is at slug, if it is on.
func (a *App) showcaseOf(ctx context.Context, slug string) (company.Org, bool) {
	all, _ := a.Companies.List(ctx)
	for _, c := range all {
		if c.Showcase.On && c.Showcase.Slug == slug {
			o, err := a.Companies.Org(ctx, c.ID)
			return o, err == nil
		}
	}
	return company.Org{}, false
}

func (a *App) servePublicShowcase(w http.ResponseWriter, r *http.Request) {
	ctx := people.With(r.Context(), people.OwnerID)
	o, ok := a.showcaseOf(ctx, r.PathValue("slug"))
	if !ok || !a.chose(ctx, companiesLab) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	showcasePage.Execute(w, showcaseView{Name: o.Name, Industry: o.Industry, Mission: o.Mission, Items: o.Showcase.Items})
}

// showcaseItem turns what the person picked into what the page shows:
// only a brief that shipped, a video the company uploaded, or a link.
func (a *App) showcaseItem(ctx context.Context, o company.Org, in company.ShowcaseItem) (company.ShowcaseItem, error) {
	it := company.ShowcaseItem{ID: newTeamID("x_"), Kind: in.Kind, Note: clip(strings.TrimSpace(in.Note), 1000), Added: time.Now().UTC()}
	switch in.Kind {
	case company.ShowBrief:
		b, err := a.Companies.Brief(ctx, in.Ref)
		if err != nil || b.Company != o.ID {
			return it, company.ErrNotFound
		}
		if b.State != company.BriefShipped {
			return it, server.StatusError{Status: 400, Msg: "only a brief that shipped goes on the showcase"}
		}
		it.Title, it.Ref = b.Title, b.ID
		if strings.HasPrefix(b.Ref, "https://") {
			it.URL = b.Ref
		}
	case company.ShowVideo:
		evs, _ := a.Events.List(ctx, event.Query{Types: []string{"company.media.uploaded"}})
		for _, e := range evs {
			var u struct{ Company, Media, Video string }
			if e.Decode(&u) == nil && u.Company == o.ID && u.Media == in.Ref {
				it.URL, it.Ref = "https://youtu.be/"+u.Video, u.Media
			}
		}
		if it.URL == "" {
			return it, server.StatusError{Status: 400, Msg: "only a video the company uploaded goes on the showcase"}
		}
		m, err := a.Companies.MediaItem(ctx, o.ID, in.Ref)
		if err != nil {
			return it, err
		}
		it.Title = firstNonEmptyText(in.Title, m.Title, "Video")
	case company.ShowLink:
		it.Title, it.URL = clip(strings.TrimSpace(in.Title), 200), strings.TrimSpace(in.URL)
	default:
		return it, server.StatusError{Status: 400, Msg: "the showcase shows briefs, videos and links"}
	}
	return it, nil
}

func firstNonEmptyText(v ...string) string {
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			return clip(s, 200)
		}
	}
	return ""
}

// companyPersonOnly lets only a company's person change its showcase: making it
// public is sharing it with everyone.
func companyPersonOnly(r *http.Request, o company.Org) error {
	if people.From(r.Context()) != o.Person {
		return server.StatusError{Status: 403, Msg: "only whoever made the company decides what it shows publicly"}
	}
	return nil
}

func (a *App) companyShowcaseRoutes() {
	a.Server.HandlePublic("GET /showcase/{slug}", a.servePublicShowcase)
	a.Server.Handle("PUT /api/companies/{id}/showcase", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		if err := companyPersonOnly(r, o); err != nil {
			return nil, err
		}
		var in struct {
			On   bool   `json:"on"`
			Slug string `json:"slug"`
		}
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
		if in.On {
			if other, ok := a.showcaseOf(r.Context(), in.Slug); ok && other.ID != o.ID {
				return nil, server.StatusError{Status: 409, Msg: "another company has that address"}
			}
		}
		c := o.Company
		c.Showcase.On, c.Showcase.Slug = in.On, in.Slug
		saved, err := a.Companies.Update(r.Context(), c)
		if err == nil {
			a.Events.Append(r.Context(), "company.showcase", actor(r.Context()), map[string]any{"company": o.ID, "on": in.On, "person": o.Person})
		}
		return saved, err
	}))
	a.Server.Handle("POST /api/companies/{id}/showcase/items", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		if err := companyPersonOnly(r, o); err != nil {
			return nil, err
		}
		var in company.ShowcaseItem
		if err := server.Decode(r, &in); err != nil {
			return nil, err
		}
		it, err := a.showcaseItem(r.Context(), o, in)
		if err != nil {
			return nil, err
		}
		c := o.Company
		c.Showcase.Items = append(slices.Clone(c.Showcase.Items), it)
		return a.Companies.Update(r.Context(), c)
	}))
	a.Server.Handle("DELETE /api/companies/{id}/showcase/items/{part}", a.companyRoute(company.Configure, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		if err := companyPersonOnly(r, o); err != nil {
			return nil, err
		}
		c := o.Company
		c.Showcase.Items = slices.DeleteFunc(slices.Clone(c.Showcase.Items), func(it company.ShowcaseItem) bool { return it.ID == r.PathValue("part") })
		return a.Companies.Update(r.Context(), c)
	}))
}

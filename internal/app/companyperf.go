package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"github.com/turbine-dev/pimpo/internal/media"
)

// How each member performs, from what it did: work finished and failed,
// how long it took, what it cost, tasks closed and stuck, questions it
// asked, kinds of delivery it earned, and, where the role has them, the
// accuracy of its briefs and how many of its videos passed their checks.

type memberPerf struct {
	Member     string  `json:"member"`
	Name       string  `json:"name"`
	Done       int     `json:"done"`
	Failed     int     `json:"failed"`
	Minutes    float64 `json:"minutes"`
	CostUSD    float64 `json:"cost_usd"`
	CostEach   float64 `json:"cost_each"`
	Tasks      int     `json:"tasks_done"`
	Blocked    int     `json:"tasks_blocked"`
	Questions  int     `json:"questions"`
	Earned     int     `json:"earned"`
	BriefsMet  int     `json:"briefs_met,omitempty"`
	BriefsOf   int     `json:"briefs_checked,omitempty"`
	VideosOK   int     `json:"videos_ok,omitempty"`
	VideosMade int     `json:"videos,omitempty"`
}

func (a *App) performance(ctx context.Context, o company.Org) []memberPerf {
	by := map[string]*memberPerf{}
	var order []string
	for _, m := range o.Members {
		if m.Kind == company.Agent {
			by[m.ID] = &memberPerf{Member: m.ID, Name: m.Name}
			order = append(order, m.ID)
		}
	}
	works, _ := a.Companies.Works(ctx, o.ID, 2000)
	minutes := map[string]float64{}
	for _, w := range works {
		p := by[w.Member]
		if p == nil {
			continue
		}
		p.CostUSD += w.CostUSD
		switch w.State {
		case company.WorkDone:
			p.Done++
			if !w.Started.IsZero() && w.Ended.After(w.Started) {
				minutes[w.Member] += w.Ended.Sub(w.Started).Minutes()
			}
		case company.WorkFailed:
			p.Failed++
		}
	}
	tasks, _ := a.Companies.Tasks(ctx, o.ID)
	for _, t := range tasks {
		if p := by[t.Assignee]; p != nil {
			switch t.State {
			case company.TaskDone:
				p.Tasks++
			case company.TaskBlocked:
				p.Blocked++
			}
		}
	}
	qs, _ := a.Companies.Questions(ctx, o.ID, false)
	for _, q := range qs {
		if p := by[q.From]; p != nil && !q.Drift {
			p.Questions++
		}
	}
	streaks, _ := a.Companies.Streaks(ctx, o.ID)
	for _, s := range streaks {
		if p := by[s.Member]; p != nil && s.Earned {
			p.Earned++
		}
	}
	briefs, _ := a.Companies.Briefs(ctx, o.ID)
	for id, p := range by {
		p.BriefsMet, p.BriefsOf = company.Accuracy(briefs, id)
	}
	items, _ := a.Companies.MediaList(ctx, o.ID)
	for _, m := range items {
		p := by[m.Member]
		if p == nil || m.Kind != company.MediaVideo {
			continue
		}
		p.VideosMade++
		if r, ok := m.Check.(map[string]any); ok {
			if problems, _ := r["problems"].([]any); len(problems) == 0 {
				p.VideosOK++
			}
		} else if r, ok := m.Check.(media.Report); ok && r.OK() {
			p.VideosOK++
		}
	}
	out := []memberPerf{}
	for _, id := range order {
		p := by[id]
		if p.Done > 0 {
			p.Minutes = minutes[id] / float64(p.Done)
			p.CostEach = p.CostUSD / float64(p.Done)
		}
		out = append(out, *p)
	}
	return out
}

// companyWidgets are the widgets each company the person may view offers
// to dashboards.
var companyWidgetKinds = []string{"summary", "spent"}

func (a *App) companyWidgetIDs(ctx context.Context) []string {
	if !a.chose(ctx, companiesLab) {
		return nil
	}
	var out []string
	all, _ := a.Companies.List(ctx)
	for _, c := range all {
		if _, err := a.myCompany(ctx, c.ID, company.View); err == nil {
			for _, k := range companyWidgetKinds {
				out = append(out, "company:"+c.ID+":"+k)
			}
		}
	}
	return out
}

// companyWidget draws a company's widget for someone who may view it.
func (a *App) companyWidget(ctx context.Context, id string) (widgetView, bool) {
	rest := strings.TrimPrefix(id, "company:")
	co, kind, ok := strings.Cut(rest, ":")
	if !ok || !a.chose(ctx, companiesLab) {
		return widgetView{}, false
	}
	o, err := a.myCompany(ctx, co, company.View)
	if err != nil {
		return widgetView{}, false
	}
	v := widgetView{ID: id, Source: "company", Updated: time.Now(), Mine: true}
	spend := a.spendOf(ctx, o).counted(o)
	link := "/companies/" + o.ID
	var s widgetSnap
	switch kind {
	case "summary":
		working, waiting := 0, 0
		works, _ := a.Companies.Waiting(ctx)
		for _, w := range works {
			if w.Company == o.ID && w.State == company.WorkRunning {
				working++
			}
		}
		qs, _ := a.Companies.Questions(ctx, o.ID, true)
		for _, q := range qs {
			if to, _ := o.Member(q.To); to.Kind == company.Person {
				waiting++
			}
		}
		items := []widgetItem{
			{Title: i18n.T(ctx, "widget.company.working"), Value: fmt.Sprint(working)},
			{Title: i18n.T(ctx, "widget.company.questions"), Value: fmt.Sprint(waiting), Status: map[bool]string{true: "warn", false: "ok"}[waiting > 0]},
			{Title: i18n.T(ctx, "widget.company.today"), Value: fmt.Sprintf("$%.2f", spend.Company.Day)},
			{Title: i18n.T(ctx, "widget.company.month"), Value: fmt.Sprintf("$%.2f", spend.Company.Month)},
		}
		if o.Budget.MonthUSD > 0 {
			items[3].Detail = i18n.T(ctx, "widget.company.of", "limit", fmt.Sprintf("$%.2f", o.Budget.MonthUSD))
		}
		s = widgetSnap{Kind: "list", Title: o.Name, Items: items, Meta: map[string]string{"company": o.ID, "link": link}}
	case "spent":
		zone := companyLevels{o}.zone()
		now := time.Now().In(zone)
		var pts []widgetPoint
		for d := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, zone); !d.After(now); d = d.AddDate(0, 0, 1) {
			pts = append(pts, widgetPoint{Label: d.Format("02"), Y: spend.ByDay[d.Format(time.DateOnly)]})
		}
		month := spend.Company.Month
		s = widgetSnap{Kind: "chart", Title: i18n.T(ctx, "widget.company.spent", "name", o.Name), Chart: "bar", Unit: "USD", Value: &month, Series: []widgetSeries{{Name: "USD", Points: pts}}, Meta: map[string]string{"company": o.ID, "link": link}}
	default:
		return widgetView{}, false
	}
	v.Kind, v.Title, v.Snapshot = s.Kind, s.Title, s
	return v, true
}

func (a *App) companyPerfRoutes() {
	a.Server.Handle("GET /api/companies/{id}/performance", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.performance(r.Context(), o), nil
	}))
}

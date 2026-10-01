package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
	"github.com/turbine-dev/pimpo/internal/store"
)

// Widgets are what routines show at a glance (docs/rfcs/0003-dashboards.md):
// a routine calls widget.show with one of a few fixed kinds, Pimpo keeps the
// latest snapshot and a short history, and dashboards lay widgets out in
// tabs. Built-in widgets need no routine. Everything is its person's, as
// since M6; a widget marked shared also shows on the house's shared
// dashboards. A snapshot is data to draw, never markup: every field is
// checked and capped here.

var widgetKinds = []string{"metric", "progress", "list", "status", "text", "table", "chart"}

const (
	maxWidgetItems  = 20
	maxWidgetCols   = 8
	maxWidgetSeries = 5
	maxWidgetPoints = 60
	maxLayoutItems  = 60
)

type widgetItem struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Badge  string `json:"badge,omitempty"`
	Value  string `json:"value,omitempty"`
	Status string `json:"status,omitempty"`
	Link   string `json:"link,omitempty"`
}

type widgetPoint struct {
	Label string  `json:"label,omitempty"`
	Y     float64 `json:"y"`
}

type widgetSeries struct {
	Name   string        `json:"name,omitempty"`
	Points []widgetPoint `json:"points"`
}

// widgetSnap is a widget's drawing: only the fields of its kind are kept.
type widgetSnap struct {
	Kind     string         `json:"kind"`
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle,omitempty"`
	Value    *float64       `json:"value,omitempty"`
	Goal     *float64       `json:"goal,omitempty"`
	Unit     string         `json:"unit,omitempty"`
	Trend    *float64       `json:"trend,omitempty"`
	Status   string         `json:"status,omitempty"`
	Text     string         `json:"text,omitempty"`
	Items    []widgetItem   `json:"items,omitempty"`
	Columns  []string       `json:"columns,omitempty"`
	Rows     [][]string     `json:"rows,omitempty"`
	Chart    string         `json:"chart,omitempty"`
	Series   []widgetSeries `json:"series,omitempty"`
	Link     string         `json:"link,omitempty"`
	// Meta is what the app words in the person's language for built-in
	// and status widgets: times, outcomes, which empty state.
	Meta map[string]string `json:"meta,omitempty"`
}

func finite(p *float64) *float64 {
	if p == nil || math.IsNaN(*p) || math.IsInf(*p, 0) {
		return nil
	}
	return p
}

func safeLink(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return u.String()
}

func oneOf(s string, allowed ...string) string {
	if slices.Contains(allowed, s) {
		return s
	}
	return ""
}

// cell is a table cell as text, whatever the routine passed.
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return clip(x, 120)
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.4f", x), "0"), ".")
	case bool:
		if x {
			return "✓"
		}
		return "—"
	default:
		b, _ := json.Marshal(x)
		return clip(string(b), 120)
	}
}

// cleanWidget checks a routine's widget.show arguments into a snapshot.
func cleanWidget(args map[string]any) (widgetSnap, string, error) {
	raw, _ := json.Marshal(args)
	var in struct {
		widgetSnap
		Key    string  `json:"key"`
		Rows   [][]any `json:"rows"`
		Values []any   `json:"values"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return widgetSnap{}, "", fmt.Errorf("widget: %w", err)
	}
	s := in.widgetSnap
	s.Kind = strings.ToLower(strings.TrimSpace(s.Kind))
	if !slices.Contains(widgetKinds, s.Kind) {
		return widgetSnap{}, "", fmt.Errorf("kind must be one of %s", strings.Join(widgetKinds, ", "))
	}
	s.Title = clip(strings.TrimSpace(s.Title), 80)
	if s.Title == "" {
		return widgetSnap{}, "", errors.New("a widget needs a title")
	}
	s.Subtitle, s.Unit = clip(s.Subtitle, 120), clip(s.Unit, 12)
	s.Value, s.Goal, s.Trend = finite(s.Value), finite(s.Goal), finite(s.Trend)
	s.Status = oneOf(strings.ToLower(s.Status), "ok", "warn", "alert")
	s.Link = safeLink(s.Link)
	s.Text = clip(s.Text, 600)
	key := clip(strings.TrimSpace(in.Key), 40)
	if key == "" {
		key = "main"
	}
	switch s.Kind {
	case "metric":
		if s.Value == nil {
			return widgetSnap{}, "", errors.New("a metric widget needs a value")
		}
	case "progress":
		if s.Value == nil || s.Goal == nil || *s.Goal == 0 {
			return widgetSnap{}, "", errors.New("a progress widget needs a value and a goal")
		}
	case "status":
		if s.Status == "" {
			s.Status = "ok"
		}
	case "text":
		if s.Text == "" {
			return widgetSnap{}, "", errors.New("a text widget needs text")
		}
	case "list":
		if len(s.Items) == 0 {
			return widgetSnap{}, "", errors.New("a list widget needs items")
		}
	case "table":
		s.Columns = s.Columns[:min(len(s.Columns), maxWidgetCols)]
		for i := range s.Columns {
			s.Columns[i] = clip(s.Columns[i], 40)
		}
		for i, r := range in.Rows {
			if i == maxWidgetItems {
				break
			}
			row := []string{}
			for j, v := range r {
				if j == maxWidgetCols {
					break
				}
				row = append(row, cell(v))
			}
			s.Rows = append(s.Rows, row)
		}
		if len(s.Columns) == 0 || len(s.Rows) == 0 {
			return widgetSnap{}, "", errors.New("a table widget needs columns and rows")
		}
	case "chart":
		s.Chart = oneOf(strings.ToLower(s.Chart), "line", "area", "bar", "donut")
		if s.Chart == "" {
			s.Chart = "line"
		}
		// A single list of numbers is a series too.
		if len(s.Series) == 0 && len(in.Values) > 0 {
			var pts []widgetPoint
			for _, v := range in.Values {
				if f, ok := v.(float64); ok {
					pts = append(pts, widgetPoint{Y: f})
				}
			}
			s.Series = []widgetSeries{{Points: pts}}
		}
		s.Series = s.Series[:min(len(s.Series), maxWidgetSeries)]
		for i := range s.Series {
			s.Series[i].Name = clip(s.Series[i].Name, 40)
			pts := s.Series[i].Points
			if len(pts) > maxWidgetPoints {
				pts = pts[len(pts)-maxWidgetPoints:]
			}
			kept := pts[:0]
			for _, p := range pts {
				if !math.IsNaN(p.Y) && !math.IsInf(p.Y, 0) {
					p.Label = clip(p.Label, 24)
					kept = append(kept, p)
				}
			}
			s.Series[i].Points = kept
		}
		if len(s.Series) == 0 || len(s.Series[0].Points) == 0 {
			return widgetSnap{}, "", errors.New("a chart widget needs values or series")
		}
	}
	if s.Kind != "table" {
		s.Columns, s.Rows = nil, nil
	}
	if s.Kind != "chart" {
		s.Series, s.Chart = nil, ""
	}
	items := s.Items[:min(len(s.Items), maxWidgetItems)]
	for i := range items {
		items[i].Title = clip(items[i].Title, 100)
		items[i].Detail = clip(items[i].Detail, 160)
		items[i].Badge = clip(items[i].Badge, 24)
		items[i].Value = clip(items[i].Value, 24)
		items[i].Status = oneOf(strings.ToLower(items[i].Status), "ok", "warn", "alert")
		items[i].Link = safeLink(items[i].Link)
	}
	s.Items = items
	return s, key, nil
}

// widgetCap is widget.show for routines.
type widgetCap struct{ a *App }

func (widgetCap) Capabilities() []string { return []string{"widget.show"} }

func (c widgetCap) Call(ctx context.Context, _, _ string, args any) (any, error) {
	m, ok := args.(map[string]any)
	if !ok {
		var in map[string]any
		if err := connector.Args(args, &in); err != nil {
			return nil, err
		}
		m = in
	}
	snap, key, err := cleanWidget(m)
	if err != nil {
		return nil, err
	}
	source := host.SourceOf(ctx)
	routine, isRoutine := strings.CutPrefix(source, "routine:")
	if !isRoutine {
		// An exploration shows what the widget would look like; the routine
		// it becomes keeps the real one.
		return map[string]any{"ok": true, "preview": true, "widget": snap}, nil
	}
	routine, _, _ = strings.Cut(routine, "#")
	body, _ := json.Marshal(snap)
	w, err := c.a.Store.SaveWidget(ctx, store.Widget{ID: newWidgetID(), Person: personField(people.Norm(people.From(ctx))), Routine: routine, Key: key, Kind: snap.Kind, Title: snap.Title, Snapshot: body})
	if err != nil {
		return nil, err
	}
	if (snap.Kind == "metric" || snap.Kind == "progress") && snap.Value != nil {
		c.a.Store.AddPoint(ctx, w.ID, *snap.Value)
	}
	c.a.Events.Append(ctx, "widget.updated", source, map[string]string{"id": w.ID, "person": people.Norm(people.From(ctx))})
	return map[string]any{"ok": true, "widget": w.ID}, nil
}

func newWidgetID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "w_" + hex.EncodeToString(b)
}

// ---------- What a person sees ----------

// widgetView is a widget ready to draw.
type widgetView struct {
	ID       string        `json:"id"`
	Source   string        `json:"source"` // routine, builtin, status, company
	Routine  string        `json:"routine,omitempty"`
	Kind     string        `json:"kind"`
	Title    string        `json:"title"`
	Snapshot any           `json:"snapshot"`
	History  []store.Point `json:"history,omitempty"`
	Updated  time.Time     `json:"updated"`
	Stale    bool          `json:"stale,omitempty"`
	Shared   bool          `json:"shared,omitempty"`
	Mine     bool          `json:"mine"`
}

func (a *App) routineName(ctx context.Context, id string) string {
	if rt, err := a.Store.Routine(ctx, id); err == nil {
		return rt.Body.Name
	}
	return id
}

// stale says a routine's widget is older than twice the routine's usual gap.
func (a *App) stale(ctx context.Context, routine string, updated time.Time) bool {
	runs, err := a.Store.Runs(ctx, routine, 3)
	if err != nil || len(runs) < 2 {
		return false
	}
	gap := runs[0].StartedAt.Sub(runs[1].StartedAt)
	return gap > 0 && time.Since(updated) > 2*gap+time.Minute
}

func (a *App) viewWidget(ctx context.Context, w store.Widget, withHistory bool) widgetView {
	v := widgetView{ID: w.ID, Source: "routine", Routine: w.Routine, Kind: w.Kind, Title: w.Title, Snapshot: w.Snapshot, Updated: w.Updated, Shared: w.Shared, Mine: mine(ctx, w.Person)}
	if withHistory {
		v.History, _ = a.Store.History(ctx, w.ID)
	}
	v.Stale = a.stale(ctx, w.Routine, w.Updated)
	return v
}

// myWidget is a routine widget the person may see: theirs, or shared.
func (a *App) myWidget(ctx context.Context, id string) (store.Widget, error) {
	w, err := a.Store.Widget(ctx, id)
	if err != nil {
		return w, err
	}
	if !mine(ctx, w.Person) && !w.Shared {
		return store.Widget{}, store.ErrNotFound
	}
	return w, nil
}

// builtin widgets are drawn from what Pimpo already knows, for the person
// asking.
var builtinWidgets = []string{"builtin:needs", "builtin:today", "builtin:spent", "builtin:budget", "builtin:reminders"}

func (a *App) builtinWidget(ctx context.Context, id string) (widgetView, bool) {
	now := time.Now()
	v := widgetView{ID: id, Source: "builtin", Updated: now, Mine: true}
	var s widgetSnap
	switch id {
	case "builtin:needs":
		approvals := a.myApprovals(ctx)
		var items []widgetItem
		for _, q := range approvals {
			items = append(items, widgetItem{Title: clip(q.Text, 100), Badge: "approval", Status: "warn"})
		}
		broken := 0
		if list, err := a.myRoutines(ctx); err == nil {
			for _, r := range list {
				if r.State == store.RoutineBroken {
					broken++
					items = append(items, widgetItem{Title: r.Body.Name, Badge: "broken", Status: "alert"})
				}
			}
		}
		n := float64(len(approvals) + broken)
		s = widgetSnap{Kind: "status", Title: "Needs you", Value: &n, Status: map[bool]string{true: "ok", false: "warn"}[n == 0], Items: items[:min(len(items), maxWidgetItems)]}
		if broken > 0 {
			s.Status = "alert"
		}
	case "builtin:today":
		var items []widgetItem
		mineIDs := map[string]string{}
		if list, err := a.myRoutines(ctx); err == nil {
			for _, r := range list {
				mineIDs[r.ID] = r.Body.Name
			}
		}
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if runs, err := a.Store.RecentRuns(ctx, "", 0, 100); err == nil {
			for _, r := range runs {
				name, ok := mineIDs[r.Routine]
				if !ok || r.StartedAt.Before(start) || len(items) == 8 {
					continue
				}
				st := map[string]string{store.RunOK: "ok", store.RunFailed: "alert"}[r.Outcome]
				items = append(items, widgetItem{Title: name, Detail: r.Outcome, Value: r.StartedAt.UTC().Format(time.RFC3339), Status: st})
			}
		}
		if a.Scheduler != nil {
			for id, name := range mineIDs {
				if next := a.Scheduler.Next(id); !next.IsZero() && next.Before(start.Add(24*time.Hour)) && len(items) < maxWidgetItems {
					items = append(items, widgetItem{Title: name, Detail: "next", Value: next.UTC().Format(time.RFC3339)})
				}
			}
		}
		if len(items) == 0 {
			s = widgetSnap{Kind: "text", Title: "Today", Text: "-", Meta: map[string]string{"empty": "today"}}
		} else {
			s = widgetSnap{Kind: "list", Title: "Today", Items: items}
		}
	case "builtin:spent", "builtin:budget":
		spent, _ := a.Budget.Today(ctx)
		limit := a.Budget.Limit(ctx)
		if id == "builtin:budget" {
			s = widgetSnap{Kind: "progress", Title: "Today's limit", Value: &spent, Goal: &limit, Unit: "USD"}
			if limit <= 0 {
				s = widgetSnap{Kind: "metric", Title: "Today's limit", Value: &spent, Unit: "USD", Meta: map[string]string{"nolimit": "1"}}
			}
			break
		}
		// This month by day, from the cost log.
		days := map[string]float64{}
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		evs, _ := a.Events.List(ctx, event.Query{Types: []string{"cost.recorded"}})
		for _, e := range evs {
			if e.Time.Before(monthStart) {
				continue
			}
			var c struct {
				USD float64 `json:"usd"`
			}
			e.Decode(&c)
			days[e.Time.In(now.Location()).Format("02")] += c.USD
		}
		var pts []widgetPoint
		for d := monthStart; !d.After(now); d = d.AddDate(0, 0, 1) {
			k := d.Format("02")
			pts = append(pts, widgetPoint{Label: k, Y: days[k]})
		}
		s = widgetSnap{Kind: "chart", Title: "Spent this month", Chart: "bar", Unit: "USD", Value: &spent, Series: []widgetSeries{{Name: "USD", Points: pts}}}
	case "builtin:reminders":
		out, _ := reminderCap{a}.Call(ctx, "reminder.list", "", map[string]any{})
		b, _ := json.Marshal(out)
		var list []struct {
			Text string `json:"text"`
			At   string `json:"at"`
		}
		json.Unmarshal(b, &list)
		var items []widgetItem
		for _, r := range list {
			items = append(items, widgetItem{Title: clip(r.Text, 100), Value: r.At, Detail: "at"})
		}
		if len(items) == 0 {
			s = widgetSnap{Kind: "text", Title: "Reminders", Text: "-", Meta: map[string]string{"empty": "reminders"}}
		} else {
			s = widgetSnap{Kind: "list", Title: "Reminders", Items: items[:min(len(items), maxWidgetItems)]}
		}
	default:
		return v, false
	}
	v.Kind, v.Title, v.Snapshot = s.Kind, s.Title, s
	return v, true
}

// statusWidget is any routine of the person's, from its own runs: no new
// version needed.
func (a *App) statusWidget(ctx context.Context, routineID string) (widgetView, bool) {
	rt, err := a.myRoutine(ctx, routineID)
	if err != nil {
		return widgetView{}, false
	}
	runs, _ := a.Store.Runs(ctx, routineID, 14)
	var pts []widgetPoint
	ok := 0
	for i := len(runs) - 1; i >= 0; i-- {
		y := 0.0
		if runs[i].Outcome == store.RunOK {
			y, ok = 1, ok+1
		}
		pts = append(pts, widgetPoint{Label: runs[i].StartedAt.UTC().Format(time.RFC3339), Y: y})
	}
	s := widgetSnap{Kind: "status", Title: rt.Body.Name, Status: "ok", Meta: map[string]string{"state": rt.State}}
	if rt.State == store.RoutineBroken {
		s.Status = "alert"
	} else if rt.State != store.RoutineActive {
		s.Status = "warn"
	}
	if len(runs) > 0 {
		last := runs[0]
		s.Meta["last"], s.Meta["outcome"] = last.StartedAt.UTC().Format(time.RFC3339), last.Outcome
		rate := float64(ok) / float64(len(runs)) * 100
		s.Value, s.Unit = &rate, "%"
		if last.Outcome == store.RunFailed && s.Status == "ok" {
			s.Status = "warn"
		}
	}
	if a.Scheduler != nil {
		if next := a.Scheduler.Next(routineID); !next.IsZero() {
			s.Meta["next"] = next.UTC().Format(time.RFC3339)
		}
	}
	if len(pts) > 0 {
		s.Series = []widgetSeries{{Name: "runs", Points: pts}}
	}
	v := widgetView{ID: "status:" + routineID, Source: "status", Routine: routineID, Kind: "status", Title: rt.Body.Name, Snapshot: s, Updated: time.Now(), Mine: true}
	if len(runs) > 0 {
		v.Updated = runs[0].StartedAt
	}
	return v, true
}

// anyWidget is a widget of any kind by id, as the person may see it.
func (a *App) anyWidget(ctx context.Context, id string, withHistory bool) (widgetView, bool) {
	switch {
	case strings.HasPrefix(id, "builtin:"):
		return a.builtinWidget(ctx, id)
	case strings.HasPrefix(id, "status:"):
		return a.statusWidget(ctx, strings.TrimPrefix(id, "status:"))
	case strings.HasPrefix(id, "company:"):
		return a.companyWidget(ctx, id)
	}
	w, err := a.myWidget(ctx, id)
	if err != nil {
		return widgetView{}, false
	}
	return a.viewWidget(ctx, w, withHistory), true
}

// ---------- Dashboards ----------

type layoutItem struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	W  int    `json:"w"`
	H  int    `json:"h"`
}

func cleanLayout(raw json.RawMessage) (json.RawMessage, error) {
	var items []layoutItem
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, server.StatusError{Status: 400, Msg: "the layout is not a list of widgets"}
		}
	}
	if len(items) > maxLayoutItems {
		items = items[:maxLayoutItems]
	}
	kept := []layoutItem{}
	seen := map[string]bool{}
	for _, it := range items {
		if it.ID == "" || seen[it.ID] || len(it.ID) > 80 {
			continue
		}
		seen[it.ID] = true
		it.W, it.H = min(max(it.W, 2), 12), min(max(it.H, 2), 8)
		it.X, it.Y = min(max(it.X, 0), 12-it.W), min(max(it.Y, 0), 200)
		kept = append(kept, it)
	}
	b, _ := json.Marshal(kept)
	return b, nil
}

type dashboardView struct {
	store.Dashboard
	Mine bool `json:"mine"`
}

// dashboards are the person's own and the house's shared ones. A person
// with none gets a first one, with the built-in widgets.
func (a *App) dashboards(ctx context.Context) []dashboardView {
	all, _ := a.Store.Dashboards(ctx)
	out := []dashboardView{}
	own := 0
	for _, d := range all {
		if mine(ctx, d.Person) {
			own++
			out = append(out, dashboardView{d, true})
		} else if d.Shared {
			out = append(out, dashboardView{d, false})
		}
	}
	if own == 0 {
		layout, _ := json.Marshal([]layoutItem{
			{ID: "builtin:needs", X: 0, Y: 0, W: 4, H: 3}, {ID: "builtin:budget", X: 4, Y: 0, W: 4, H: 3},
			{ID: "builtin:reminders", X: 8, Y: 0, W: 4, H: 3}, {ID: "builtin:today", X: 0, Y: 3, W: 6, H: 4},
			{ID: "builtin:spent", X: 6, Y: 3, W: 6, H: 4},
		})
		d := store.Dashboard{ID: newWidgetID()[2:], Person: personField(people.Norm(people.From(ctx))), Name: "Home", Emoji: "🏠", Layout: layout}
		if a.Store.SaveDashboard(ctx, d) == nil {
			d.Updated = time.Now()
			out = append([]dashboardView{{d, true}}, out...)
		}
	}
	return out
}

func (a *App) myDashboard(ctx context.Context, id string) (store.Dashboard, error) {
	all, err := a.Store.Dashboards(ctx)
	if err != nil {
		return store.Dashboard{}, err
	}
	for _, d := range all {
		if d.ID == id && mine(ctx, d.Person) {
			return d, nil
		}
	}
	return store.Dashboard{}, store.ErrNotFound
}

// canSeeDashboard: the person's own, or a shared one.
func (a *App) canSeeDashboard(ctx context.Context, id string) (store.Dashboard, bool) {
	all, _ := a.Store.Dashboards(ctx)
	for _, d := range all {
		if d.ID == id && (mine(ctx, d.Person) || d.Shared) {
			return d, true
		}
	}
	return store.Dashboard{}, false
}

func (a *App) widgetRoutes() {
	s := a.Server
	s.Handle("GET /api/dashboards", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, 200, a.dashboards(r.Context()))
	})
	s.Handle("POST /api/dashboards", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var req struct {
			Name  string `json:"name"`
			Emoji string `json:"emoji"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		name := clip(strings.TrimSpace(req.Name), 40)
		if name == "" {
			server.WriteError(w, server.StatusError{Status: 400, Msg: "a dashboard needs a name"})
			return
		}
		pos := 0
		for _, d := range a.dashboards(ctx) {
			if d.Mine && d.Position >= pos {
				pos = d.Position + 1
			}
		}
		d := store.Dashboard{ID: newWidgetID()[2:], Person: personField(people.Norm(people.From(ctx))), Name: name, Emoji: clip(req.Emoji, 8), Position: pos, Layout: json.RawMessage("[]")}
		if err := a.Store.SaveDashboard(ctx, d); err != nil {
			server.WriteError(w, err)
			return
		}
		a.Events.Append(ctx, "dashboard.changed", actor(ctx), map[string]string{"id": d.ID, "person": people.From(ctx)})
		server.WriteJSON(w, 201, dashboardView{d, true})
	})
	s.Handle("PUT /api/dashboards/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		d, err := a.myDashboard(ctx, r.PathValue("id"))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such dashboard"})
			return
		}
		var req struct {
			Name     *string         `json:"name"`
			Emoji    *string         `json:"emoji"`
			Position *int            `json:"position"`
			Shared   *bool           `json:"shared"`
			Layout   json.RawMessage `json:"layout"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
			d.Name = clip(strings.TrimSpace(*req.Name), 40)
		}
		if req.Emoji != nil {
			d.Emoji = clip(*req.Emoji, 8)
		}
		if req.Position != nil {
			d.Position = *req.Position
		}
		if req.Shared != nil {
			d.Shared = *req.Shared
		}
		if req.Layout != nil {
			if d.Layout, err = cleanLayout(req.Layout); err != nil {
				server.WriteError(w, err)
				return
			}
		}
		if err := a.Store.SaveDashboard(ctx, d); err != nil {
			server.WriteError(w, err)
			return
		}
		a.Events.Append(ctx, "dashboard.changed", actor(ctx), map[string]string{"id": d.ID, "person": people.From(ctx)})
		server.WriteJSON(w, 200, dashboardView{d, true})
	})
	s.Handle("DELETE /api/dashboards/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		d, err := a.myDashboard(ctx, r.PathValue("id"))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such dashboard"})
			return
		}
		a.Store.DeleteDashboard(ctx, d.ID)
		server.WriteJSON(w, 200, map[string]string{"removed": d.ID})
	})
	// Everything the person can put on a dashboard.
	s.Handle("GET /api/widgets", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		out := []widgetView{}
		for _, id := range builtinWidgets {
			if v, ok := a.builtinWidget(ctx, id); ok {
				out = append(out, v)
			}
		}
		if all, err := a.Store.Widgets(ctx); err == nil {
			for _, x := range all {
				if mine(ctx, x.Person) || x.Shared {
					out = append(out, a.viewWidget(ctx, x, false))
				}
			}
		}
		if list, err := a.myRoutines(ctx); err == nil {
			for _, rt := range list {
				if v, ok := a.statusWidget(ctx, rt.ID); ok {
					out = append(out, v)
				}
			}
		}
		for _, id := range a.companyWidgetIDs(ctx) {
			if v, ok := a.companyWidget(ctx, id); ok {
				out = append(out, v)
			}
		}
		server.WriteJSON(w, 200, out)
	})
	// A dashboard's widgets, drawn for the person looking: a shared
	// dashboard shows others only what is shared, or theirs to see.
	s.Handle("GET /api/dashboards/{id}/widgets", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		d, ok := a.canSeeDashboard(ctx, r.PathValue("id"))
		if !ok {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such dashboard"})
			return
		}
		var items []layoutItem
		json.Unmarshal(d.Layout, &items)
		out := map[string]any{}
		for _, it := range items {
			if v, ok := a.anyWidget(ctx, it.ID, true); ok {
				out[it.ID] = v
			} else {
				out[it.ID] = map[string]any{"id": it.ID, "hidden": true}
			}
		}
		server.WriteJSON(w, 200, out)
	})
	s.Handle("GET /api/widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, ok := a.anyWidget(r.Context(), r.PathValue("id"), true)
		if !ok {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such widget"})
			return
		}
		server.WriteJSON(w, 200, v)
	})
	s.Handle("PUT /api/widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		x, err := a.Store.Widget(ctx, r.PathValue("id"))
		if err != nil || !mine(ctx, x.Person) {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such widget"})
			return
		}
		var req struct {
			Shared bool `json:"shared"`
		}
		if err := server.Decode(r, &req); err != nil {
			server.WriteError(w, err)
			return
		}
		a.Store.SetWidgetShared(ctx, x.ID, req.Shared)
		x.Shared = req.Shared
		server.WriteJSON(w, 200, a.viewWidget(ctx, x, false))
	})
	s.Handle("DELETE /api/widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		x, err := a.Store.Widget(ctx, r.PathValue("id"))
		if err != nil || !mine(ctx, x.Person) {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such widget"})
			return
		}
		a.Store.DeleteWidget(ctx, x.ID)
		server.WriteJSON(w, 200, map[string]string{"removed": x.ID})
	})
	// Refresh now runs the widget's routine once. A routine that uses a
	// model and cost more than a cent last time says so first.
	s.Handle("POST /api/widgets/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := r.PathValue("id")
		routine := strings.TrimPrefix(id, "status:")
		if !strings.HasPrefix(id, "status:") {
			x, err := a.myWidget(ctx, id)
			if err != nil || !mine(ctx, x.Person) {
				server.WriteError(w, server.StatusError{Status: 404, Msg: "no such widget"})
				return
			}
			routine = x.Routine
		}
		rt, err := a.myRoutine(ctx, routine)
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "no such routine"})
			return
		}
		var req struct {
			Confirm bool `json:"confirm"`
		}
		server.Decode(r, &req)
		usesModel := len(rt.Body.Manifest.Judgments) > 0 || len(rt.Body.Manifest.Writes) > 0
		if usesModel && !req.Confirm {
			if runs, _ := a.Store.Runs(ctx, routine, 1); len(runs) > 0 && runs[0].CostUSD >= 0.01 {
				server.WriteJSON(w, 409, map[string]any{"error": "this refresh uses a model", "code": "widget.cost", "cost_usd": runs[0].CostUSD})
				return
			}
		}
		run, err := a.Scheduler.RunNow(context.WithoutCancel(ctx), routine, actor(ctx))
		if err != nil {
			server.WriteError(w, server.StatusError{Status: 422, Msg: err.Error()})
			return
		}
		server.WriteJSON(w, 200, map[string]any{"run": run})
	})
}

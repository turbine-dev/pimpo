package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/llm"
	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
	"github.com/turbine-dev/pimpo/internal/store"
)

// A routine shows a widget; it lands on a dashboard with its history, and
// the next run replaces it.
func TestARoutineShowsAWidgetOnADashboard(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	code := `async function run() {
	  const n = (state.get("n") || 0) + 1; state.set("n", n)
	  await widget.show({kind: "metric", title: "Dólar", value: 5.1 + n / 10, unit: "BRL", trend: 1.2, link: "https://example.com/usd"})
	  await widget.show({key: "rates", kind: "table", title: "Moedas", columns: ["Moeda", "Valor"], rows: [["USD", 5.2], ["EUR", 5.9]]})
	}`
	if _, err := ta.Store.SaveRoutine(ctx, "dolar", routine.Routine{Name: "Dólar", Code: code, Manifest: runtime.Manifest{Schedule: "0 * * * *", Capabilities: []string{"widget.show"}}}, "test", "owner"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if run, err := ta.Scheduler.RunNow(ctx, "dolar", "test"); err != nil || run.Outcome != store.RunOK {
			t.Fatalf("run %d: %+v %v", i, run, err)
		}
	}
	_, out := ta.do(t, "GET", "/api/widgets", nil)
	var metric, table map[string]any
	for _, w := range out["list"].([]any) {
		m := w.(map[string]any)
		switch m["title"] {
		case "Dólar":
			if m["source"] == "routine" {
				metric = m
			}
		case "Moedas":
			table = m
		}
	}
	if metric == nil || table == nil {
		t.Fatalf("widgets: %v", out)
	}
	_, one := ta.do(t, "GET", "/api/widgets/"+metric["id"].(string), nil)
	snap := one["snapshot"].(map[string]any)
	if snap["value"].(float64) != 5.3 || snap["link"] != "https://example.com/usd" || len(one["history"].([]any)) != 2 {
		t.Fatalf("the metric: %v", one)
	}
	if rows := table["snapshot"].(map[string]any)["rows"].([]any); rows[0].([]any)[1] != "5.2" {
		t.Fatalf("table cells are text: %v", rows)
	}

	// The first dashboard is made with the built-in widgets; a second tab
	// takes the routine's.
	_, dash := ta.do(t, "GET", "/api/dashboards", nil)
	if list := dash["list"].([]any); len(list) != 1 || list[0].(map[string]any)["name"] != "Home" {
		t.Fatalf("first dashboard: %v", dash)
	}
	code2, made := ta.do(t, "POST", "/api/dashboards", map[string]string{"name": "Dinheiro", "emoji": "💰"})
	if code2 != 201 {
		t.Fatal(code2)
	}
	id := made["id"].(string)
	layout := []map[string]any{{"id": metric["id"], "x": 0, "y": 0, "w": 4, "h": 3}, {"id": table["id"], "x": 20, "y": 0, "w": 30, "h": 1}, {"id": "builtin:budget", "x": 4, "y": 0, "w": 4, "h": 3}}
	if code, saved := ta.do(t, "PUT", "/api/dashboards/"+id, map[string]any{"layout": layout}); code != 200 {
		t.Fatalf("%d %v", code, saved)
	}
	_, drawn := ta.do(t, "GET", "/api/dashboards/"+id+"/widgets", nil)
	if len(drawn) != 3 || drawn["builtin:budget"].(map[string]any)["kind"] != "progress" {
		t.Fatalf("drawn: %v", drawn)
	}
	_, dash = ta.do(t, "GET", "/api/dashboards", nil)
	var lay []map[string]any
	for _, d := range dash["list"].([]any) {
		if d.(map[string]any)["id"] == id {
			b, _ := json.Marshal(d.(map[string]any)["layout"])
			json.Unmarshal(b, &lay)
		}
	}
	if lay[1]["w"].(float64) != 12 || lay[1]["x"].(float64) != 0 || lay[1]["h"].(float64) != 2 {
		t.Fatalf("a widget off the grid was not brought back: %v", lay[1])
	}
}

// What a widget may carry is checked: fixed kinds, capped sizes, https
// links only.
func TestWidgetsCarryOnlyData(t *testing.T) {
	bad := []map[string]any{
		{"kind": "html", "title": "x"},
		{"kind": "metric", "title": "no value"},
		{"kind": "metric", "value": 1},
		{"kind": "progress", "title": "x", "value": 1, "goal": 0},
		{"kind": "table", "title": "x", "columns": []any{"a"}},
		{"kind": "chart", "title": "x"},
	}
	for _, b := range bad {
		if _, _, err := cleanWidget(b); err == nil {
			t.Errorf("accepted %v", b)
		}
	}
	items := []any{}
	for i := 0; i < 50; i++ {
		items = append(items, map[string]any{"title": strings.Repeat("x", 300), "link": "javascript:alert(1)", "status": "boom"})
	}
	s, key, err := cleanWidget(map[string]any{"kind": "list", "title": "ok", "items": items, "link": "http://plain.example", "status": "ALERT"})
	if err != nil || key != "main" {
		t.Fatal(err, key)
	}
	if len(s.Items) != maxWidgetItems || s.Items[0].Link != "" || s.Items[0].Status != "" || len([]rune(s.Items[0].Title)) > 101 || s.Link != "" || s.Status != "alert" {
		t.Fatalf("%+v", s.Items[0])
	}
	c, _, err := cleanWidget(map[string]any{"kind": "chart", "title": "c", "chart": "pie", "values": []any{1.0, 2.0, 3.0}})
	if err != nil || c.Chart != "line" || len(c.Series[0].Points) != 3 {
		t.Fatalf("%+v %v", c, err)
	}
}

// An exploration only previews a widget: the routine it becomes keeps it.
func TestAnExplorationOnlyPreviewsAWidget(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	out, err := (widgetCap{ta.App}).Call(host.WithSource(ctx, "exploration:abc"), "widget.show", "", map[string]any{"kind": "text", "title": "Prévia", "text": "oi"})
	if err != nil || out.(map[string]any)["preview"] != true {
		t.Fatalf("%v %v", out, err)
	}
	if all, _ := ta.Store.Widgets(ctx); len(all) != 0 {
		t.Fatal("an exploration kept a widget")
	}
}

// Shared dashboards show the house only shared widgets.
func TestSharedDashboardsShowOnlySharedWidgets(t *testing.T) {
	h := newHouse(t)
	own := h.owner["dashboard"]
	h.do(t, "PUT", "/api/dashboards/"+own, map[string]any{"shared": true})
	code, body := h.raw(t, h.ana, "GET", "/api/dashboards/"+own+"/widgets", nil)
	if code != 200 || strings.Contains(body, h.ownerMark) || !strings.Contains(body, `"hidden":true`) {
		t.Fatalf("Ana saw a widget that is not shared: %d %s", code, body)
	}
	if code, _ := h.raw(t, h.ana, "PUT", "/api/dashboards/"+own, js(map[string]any{"name": "mine now"})); code != 404 {
		t.Fatal("Ana changed the owner's shared dashboard")
	}
	h.do(t, "PUT", "/api/widgets/"+h.owner["widget"], map[string]any{"shared": true})
	if _, body := h.raw(t, h.ana, "GET", "/api/dashboards/"+own+"/widgets", nil); !strings.Contains(body, h.ownerMark) {
		t.Fatalf("a shared widget did not show: %s", body)
	}
	if code, _ := h.raw(t, h.ana, "DELETE", "/api/widgets/"+h.owner["widget"], nil); code != 404 {
		t.Fatal("Ana removed the owner's shared widget")
	}
}

// Refresh now asks first when the routine uses a model and costs.
func TestRefreshNowAsksBeforeSpending(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	ctx := context.Background()
	ta.Store.SaveRoutine(ctx, "caro", routine.Routine{Name: "Caro", Code: `async function run() {}`, Manifest: runtime.Manifest{Schedule: "0 * * * *", Capabilities: []string{"widget.show"}, Judgments: map[string]string{"ok": "is it ok?"}}}, "test", "owner")
	run, _ := ta.Store.StartRun(ctx, "caro", 1)
	ta.Store.FinishRun(ctx, run, store.RunOK, "", 0.05, 1)
	if code, out := ta.do(t, "POST", "/api/widgets/status:caro/refresh", map[string]any{}); code != 409 || out["code"] != "widget.cost" {
		t.Fatalf("%d %v", code, out)
	}
	if code, out := ta.do(t, "POST", "/api/widgets/status:caro/refresh", map[string]any{"confirm": true}); code != 200 {
		t.Fatalf("confirmed refresh: %d %v", code, out)
	}
}

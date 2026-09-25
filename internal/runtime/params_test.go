package runtime

import (
	"context"
	"strings"
	"testing"
)

type recorder struct{ sent []any }

func (r *recorder) Call(_ context.Context, name, _ string, args any) (any, error) {
	r.sent = append(r.sent, args)
	return map[string]any{"ok": true}, nil
}
func (r *recorder) Judge(context.Context, string, string, any) (float64, error) { return 0, nil }

var weather = Manifest{Capabilities: []string{"telegram.send"}, Params: []Param{
	{Name: "city", Label: "Cidade", Type: "location", Default: map[string]any{"name": "São Paulo", "latitude": -23.55, "longitude": -46.63}},
	{Name: "rain", Label: "Avisar chuva a partir de", Type: "number", Default: 50.0},
	{Name: "units", Type: "select", Options: []string{"celsius", "fahrenheit"}, Default: "celsius"},
	{Name: "days", Type: "multiselect", Options: []string{"seg", "ter", "qua"}, Default: []any{"seg"}},
	{Name: "hour", Type: "time", Default: "07:00"},
}}

const code = `async function run() {
  await telegram.send({text: params.city.name + " " + params.city.latitude + " " + params.rain + " " + params.units + " " + params.days.join("+") + " " + params.hour});
  try { params.rain = 1 } catch (e) {}
  try { params.city.name = "x" } catch (e) {}
  await telegram.send({text: params.rain + " " + params.city.name});
}`

func TestParamsReachTheRoutineFrozen(t *testing.T) {
	r := &recorder{}
	if _, err := Run(context.Background(), code, weather, r, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := r.sent[0].(map[string]any)["text"]; got != "São Paulo -23.55 50 celsius seg 07:00" {
		t.Fatalf("defaults: %v", got)
	}
	if got := r.sent[1].(map[string]any)["text"]; got != "50 São Paulo" {
		t.Fatalf("params changed inside the routine: %v", got)
	}
	r = &recorder{}
	lisbon := map[string]any{"name": "Lisboa", "latitude": 38.72, "longitude": -9.14, "timezone": "Europe/Lisbon"}
	if _, err := Run(context.Background(), code, weather, r, Options{Params: map[string]any{"city": lisbon, "rain": "70,5", "days": []any{"ter", "qua"}}}); err != nil {
		t.Fatal(err)
	}
	if got := r.sent[0].(map[string]any)["text"]; got != "Lisboa 38.72 70.5 celsius ter+qua 07:00" {
		t.Fatalf("values: %v", got)
	}
}

func TestParamsAreChecked(t *testing.T) {
	for _, c := range []struct {
		values map[string]any
		want   string
	}{
		{map[string]any{"units": "kelvin"}, "must be one of"},
		{map[string]any{"rain": "muito"}, "must be a number"},
		{map[string]any{"hour": "25:00"}, "must be a time"},
		{map[string]any{"days": []any{"dom"}}, "not an option"},
		{map[string]any{"city": map[string]any{"name": "X", "latitude": 200.0, "longitude": 0.0}}, "must be a place"},
		{map[string]any{"ghost": 1}, "no setting"},
	} {
		if _, err := weather.ResolveParams(c.values); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v", c.values, err)
		}
	}
	bad := []Manifest{
		{Capabilities: []string{"telegram.send"}, Params: []Param{{Name: "1x", Type: "text"}}},
		{Capabilities: []string{"telegram.send"}, Params: []Param{{Name: "a", Type: "color"}}},
		{Capabilities: []string{"telegram.send"}, Params: []Param{{Name: "a", Type: "select"}}},
		{Capabilities: []string{"telegram.send"}, Params: []Param{{Name: "a", Type: "email", Default: "not-an-email"}}},
		{Capabilities: []string{"telegram.send"}, Params: []Param{{Name: "a", Type: "text"}, {Name: "a", Type: "text"}}},
	}
	for _, m := range bad {
		if err := m.Validate(); err == nil {
			t.Errorf("accepted %+v", m.Params)
		}
	}
	if _, err := (Manifest{Capabilities: []string{"telegram.send"}, Params: []Param{{Name: "to", Type: "email"}}}).ResolveParams(nil); err == nil {
		t.Error("a required setting without a value was accepted")
	}
}

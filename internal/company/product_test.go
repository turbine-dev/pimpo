package company

import (
	"testing"
	"time"
)

func TestABriefScoresAsTheRoadmapSays(t *testing.T) {
	s := Scores{Value: 4, Differentiation: 3, Adoption: 5, BuildRisk: 2, SafetyRisk: 1}
	if got := s.Score(); got != 2*4+3+2*5-2-1 {
		t.Fatalf("score = %d", got)
	}
	b := Brief{Title: "Offline mode", Problem: "p", Proposal: "q", Scores: s, Claims: []Claim{{Text: "Users ask for it", Source: "https://github.com/a/b/issues/1"}}, Predictions: []Prediction{{Metric: "weekly users", Expected: "+10%"}}}
	if err := b.Check(); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]func(*Brief){
		"a claim without a source": func(b *Brief) { b.Claims = []Claim{{Text: "Everyone wants it"}} },
		"no predictions":           func(b *Brief) { b.Predictions = nil },
		"a score of 9":             func(b *Brief) { b.Scores.Value = 9 },
		"no claims":                func(b *Brief) { b.Claims = nil },
	} {
		c := b
		bad(&c)
		if c.Check() == nil {
			t.Errorf("%s passed", name)
		}
	}
}

func TestTheSameSignalCountsOnce(t *testing.T) {
	for _, c := range []struct {
		a, b Signal
		same bool
	}{
		{Signal{URL: "https://www.example.com/post/?utm_source=x"}, Signal{URL: "https://example.com/post"}, true},
		{Signal{URL: "https://example.com/a"}, Signal{URL: "https://example.com/b"}, false},
		{Signal{Title: "Dark mode for the dashboard, please"}, Signal{Title: "Please: dark mode for the dashboard"}, true},
		{Signal{Title: "Dark mode for the dashboard"}, Signal{Title: "Export the dashboard to PDF"}, false},
		{Signal{Title: "Exportação para PDF"}, Signal{Title: "exportacao para pdf"}, true},
	} {
		if got := sameSignal(c.a, c.b); got != c.same {
			t.Errorf("%+v vs %+v = %v", c.a, c.b, got)
		}
	}
}

func TestAShippedBriefIsCheckedOnDays30And90(t *testing.T) {
	shipped := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	b := Brief{State: BriefShipped, Shipped: shipped}
	if _, ok := b.Due(shipped.AddDate(0, 0, 29)); ok {
		t.Fatal("due before day 30")
	}
	if d, ok := b.Due(shipped.AddDate(0, 0, 31)); !ok || d != 30 {
		t.Fatalf("day 31 = %d %v", d, ok)
	}
	b.Reviews = []Review{{Day: 30, Results: []Result{{Met: true}, {Met: false}}}}
	if _, ok := b.Due(shipped.AddDate(0, 0, 60)); ok {
		t.Fatal("due again before day 90")
	}
	if d, ok := b.Due(shipped.AddDate(0, 0, 95)); !ok || d != 90 {
		t.Fatalf("day 95 = %d %v", d, ok)
	}
	b.Author = "po"
	if met, checked := Accuracy([]Brief{b, {Author: "other", Reviews: b.Reviews}}, "po"); met != 1 || checked != 2 {
		t.Fatalf("accuracy = %d/%d", met, checked)
	}
	if _, ok := (Brief{State: BriefAccepted, Shipped: shipped}).Due(shipped.AddDate(1, 0, 0)); ok {
		t.Fatal("a brief that did not ship owes a review")
	}
}

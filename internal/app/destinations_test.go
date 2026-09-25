package app

import "testing"

func TestSchedulesRunAtMostEveryFiveMinutes(t *testing.T) {
	for expr, often := range map[string]bool{"* * * * *": true, "*/2 * * * *": true, "0,3 * * * *": true, "*/5 * * * *": false, "*/15 * * * *": false, "0 7 * * *": false} {
		s, err := cronParser.Parse(expr)
		if err != nil || tooOften(s) != often {
			t.Errorf("%s: %v %v", expr, tooOften(s), err)
		}
	}
}

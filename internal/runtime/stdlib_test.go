package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func eval(t *testing.T, expr string) string {
	t.Helper()
	sp, _ := time.LoadLocation("America/Sao_Paulo")
	h := &recordingHost{}
	code := `async function run() { log(String(` + expr + `)); }`
	res, err := Run(context.Background(), code, Manifest{Capabilities: []string{"telegram.send"}}, h, Options{Now: time.Date(2026, 9, 24, 10, 30, 0, 0, time.UTC), Zone: sp})
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	return res.Logs[0]
}

func TestDates(t *testing.T) {
	for expr, want := range map[string]string{
		`now()`:                      "2026-09-24T07:30:00-03:00",
		`dates.today()`:              "2026-09-24T00:00:00-03:00",
		`dates.startOfDay(now(), 1)`: "2026-09-25T00:00:00-03:00",
		`dates.format("2026-09-25T14:05:00-03:00", "EEEE, dd/MM HH:mm")`:     "sexta-feira, 25/09 14:05",
		`dates.format("2026-09-25", "d 'de' MMMM")`:                          "25 de setembro",
		`dates.format("2026-09-24T23:30:00Z", "EEE dd")`:                     "qui 24",
		`dates.diffDays(now(), "2026-09-30T08:00:00-03:00")`:                 "6",
		`dates.parse("Vencimento: 29/09/2026.")`:                             "2026-09-29T00:00:00-03:00",
		`dates.parse("vence em 30/09")`:                                      "2026-09-30T00:00:00-03:00",
		`dates.parse("até 5 de outubro")`:                                    "2026-10-05T00:00:00-03:00",
		`dates.parse("due Oct 3, 2026")`:                                     "2026-10-03T00:00:00-03:00",
		`dates.parse("sem data aqui")`:                                       "null",
		`dates.sameDay("2026-09-24T02:00:00Z", "2026-09-23T12:00:00-03:00")`: "true",
	} {
		if got := eval(t, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

func TestMoney(t *testing.T) {
	for expr, want := range map[string]string{
		`money.parse("Total a pagar R$ 1.482,35 até sexta")`: "1482.35",
		`money.parse("$1,234.50 due")`:                       "1234.5",
		`money.parse("R$ 89,9")`:                             "89.9",
		`money.parse("nada")`:                                "null",
		`money.find("valor de R$ 2.350,00, vencimento")`:     "R$ 2.350,00",
		`money.format(2350, "BRL")`:                          "R$ 2.350,00",
		`money.format(1234.5, "USD")`:                        "$1,234.50",
	} {
		if got := eval(t, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

func TestBadDateIsAClearError(t *testing.T) {
	_, err := Run(context.Background(), `async function run() { dates.format("soon", "dd"); }`, Manifest{Capabilities: []string{"telegram.send"}}, &recordingHost{}, Options{})
	if err == nil || !strings.Contains(err.Error(), "not an ISO date") {
		t.Fatalf("got %v", err)
	}
}

func TestManifestLocale(t *testing.T) {
	h := &recordingHost{}
	m := Manifest{Capabilities: []string{"telegram.send"}, Locale: "en-US"}
	res, err := Run(context.Background(), `async function run() { log(dates.format("2026-09-25", "EEE, MMMM d")); }`, m, h, Options{})
	if err != nil || res.Logs[0] != "Fri, September 25" {
		t.Fatalf("%v %v", res.Logs, err)
	}
}

package services

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/connector"
)

func init() {
	register(Kind{
		ID: "stripe", Title: "Stripe", Description: "Saldo e cobranças da sua conta, só leitura.",
		Help:   "Em Stripe › Desenvolvedores › Chaves de API, crie uma chave restrita só com leitura de Balance e Charges. Nada é cobrado nem movido por aqui.",
		Fields: []Field{{Name: "key", Label: "Chave restrita", Placeholder: "rk_live_…", Secret: true}},
		Specs: []capability.Spec{
			{Name: "stripe.balance", Risk: capability.Read, Signature: "stripe.balance()", Returns: "{available: [{amount, currency}], pending: [{amount, currency}]} amounts in the currency's units, e.g. 12.50",
				Schema: obj(``)},
			{Name: "stripe.charges", Risk: capability.Read, Signature: "stripe.charges({days, max})", Returns: "{charges: [{amount, currency, status, refunded, description, created}], total: {currency: amount}} of succeeded charges in the last days (30 by default)",
				Schema: obj(`"days":{"type":"integer"},"max":{"type":"integer"}`)},
		},
		Call: callStripe,
		Probe: func(ctx context.Context, cfg Config) error {
			_, err := callStripe(ctx, cfg, "stripe.balance", "", map[string]any{})
			return err
		},
	})
}

// zeroDecimal are the currencies Stripe counts in whole units.
var zeroDecimal = map[string]bool{"bif": true, "clp": true, "djf": true, "gnf": true, "jpy": true, "kmf": true, "krw": true, "mga": true, "pyg": true, "rwf": true, "ugx": true, "vnd": true, "vuv": true, "xaf": true, "xof": true, "xpf": true}

func units(amount int64, currency string) float64 {
	if zeroDecimal[strings.ToLower(currency)] {
		return float64(amount)
	}
	return float64(amount) / 100
}

func callStripe(ctx context.Context, cfg Config, name, _ string, args any) (any, error) {
	v, err := need(ctx, cfg, "Stripe", "key")
	if err != nil {
		return nil, err
	}
	h := map[string]string{"Authorization": "Bearer " + v[0]}
	api := base("stripe", "https://api.stripe.com")
	switch name {
	case "stripe.balance":
		var raw struct {
			Available []struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"available"`
			Pending []struct {
				Amount   int64  `json:"amount"`
				Currency string `json:"currency"`
			} `json:"pending"`
		}
		if err := doJSON(ctx, "GET", api+"/v1/balance", h, nil, &raw); err != nil {
			return nil, err
		}
		out := map[string][]map[string]any{"available": {}, "pending": {}}
		for _, b := range raw.Available {
			out["available"] = append(out["available"], map[string]any{"amount": units(b.Amount, b.Currency), "currency": b.Currency})
		}
		for _, b := range raw.Pending {
			out["pending"] = append(out["pending"], map[string]any{"amount": units(b.Amount, b.Currency), "currency": b.Currency})
		}
		return out, nil
	case "stripe.charges":
		var a struct {
			Days int `json:"days"`
			Max  int `json:"max"`
		}
		if err := connector.Args(args, &a); err != nil {
			return nil, err
		}
		if a.Days <= 0 || a.Days > 366 {
			a.Days = 30
		}
		if a.Max <= 0 || a.Max > 100 {
			a.Max = 100
		}
		q := url.Values{"limit": {fmt.Sprint(a.Max)}, "created[gte]": {fmt.Sprint(time.Now().AddDate(0, 0, -a.Days).Unix())}}
		var raw struct {
			Data []struct {
				Amount         int64  `json:"amount"`
				AmountRefunded int64  `json:"amount_refunded"`
				Currency       string `json:"currency"`
				Status         string `json:"status"`
				Refunded       bool   `json:"refunded"`
				Description    string `json:"description"`
				Created        int64  `json:"created"`
			} `json:"data"`
		}
		if err := doJSON(ctx, "GET", api+"/v1/charges?"+q.Encode(), h, nil, &raw); err != nil {
			return nil, err
		}
		charges, total := []map[string]any{}, map[string]float64{}
		for _, c := range raw.Data {
			charges = append(charges, map[string]any{"amount": units(c.Amount, c.Currency), "currency": c.Currency, "status": c.Status, "refunded": c.Refunded,
				"description": c.Description, "created": time.Unix(c.Created, 0).UTC().Format(time.RFC3339)})
			if c.Status == "succeeded" {
				total[c.Currency] += units(c.Amount-c.AmountRefunded, c.Currency)
			}
		}
		return map[string]any{"charges": charges, "total": total}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

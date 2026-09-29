//go:build live

package external

import (
	"context"
	"testing"
	"time"
)

func TestOpenAPILive(t *testing.T) {
	ctx := context.Background()
	for _, u := range []string{
		"https://petstore3.swagger.io/api/v3/openapi.json",
		"https://raw.githubusercontent.com/github/rest-api-description/main/descriptions/api.github.com/api.github.com.yaml",
		"https://api.apis.guru/v2/specs/xkcd.com/1.0.0/openapi.yaml",
	} {
		start := time.Now()
		raw, err := FetchSpec(ctx, u)
		if err != nil {
			t.Errorf("%s: %v", u, err)
			continue
		}
		fetched := time.Since(start)
		api, err := ParseOpenAPI(raw, u)
		if err != nil {
			t.Errorf("%s: %v", u, err)
			continue
		}
		t.Logf("%s: %d KB fetched in %v, parsed in %v; base %s, %d ops, %d unsupported, keys %v", api.Title, len(raw)>>10, fetched, time.Since(start)-fetched, api.Base, len(api.Operations), len(api.Unsupported), api.Keys)
		chosen := map[string]string{}
		for _, o := range api.Operations {
			if o.Method == "GET" && len(chosen) < 20 {
				chosen[o.ID] = "read"
			}
		}
		m, err := api.Manifest("livetest", "", u, chosen)
		if err != nil {
			t.Error(err)
			continue
		}
		if err := m.HTTP.validate(m.Capabilities, m.Env); err != nil {
			t.Error(err)
		}
		problems := Check(ctx, m, func(context.Context, string) (string, error) { return "", nil })
		t.Logf("  %d contract cases, problems: %v", len(m.Contract), problems)
		for _, c := range m.Capabilities[:min(3, len(m.Capabilities))] {
			t.Logf("  %s  %s", c.Signature, c.Returns)
		}
	}
}

package i18n

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var slotRe = regexp.MustCompile(`\{(\w+)\}`)

func slotsOf(s string) string {
	var out []string
	for _, m := range slotRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	slices.Sort(out)
	return strings.Join(out, ",")
}

// Every catalog has the keys and {slots} of the Portuguese one, and the
// plural forms its language needs.
func TestCatalogsMatchTheSource(t *testing.T) {
	once.Do(load)
	src := catalogs["pt"]
	want := []string{"pt", "en", "es", "fr", "de", "it", "ja", "zh", "ko", "ru"}
	for _, lang := range want {
		c, ok := catalogs[lang]
		if !ok {
			t.Errorf("no %s catalog", lang)
			continue
		}
		for k, v := range src {
			got, ok := c[k]
			if !ok || strings.TrimSpace(got) == "" {
				t.Errorf("%s: missing %s", lang, k)
			} else if slotsOf(got) != slotsOf(v) {
				t.Errorf("%s: %s has slots %q, want %q", lang, k, slotsOf(got), slotsOf(v))
			}
		}
		for k := range c {
			if _, ok := src[k]; ok {
				continue
			}
			i := strings.LastIndex(k, "_")
			if i < 0 {
				t.Errorf("%s: unknown key %s", lang, k)
				continue
			}
			if _, ok := src[k[:i]+"_other"]; !ok || !slices.Contains([]string{"few", "many"}, k[i+1:]) {
				t.Errorf("%s: unknown key %s", lang, k)
			}
		}
		if lang == "ru" {
			for k := range src {
				if b, ok := strings.CutSuffix(k, "_other"); ok {
					for _, form := range []string{"_few", "_many"} {
						if _, ok := c[b+form]; !ok {
							t.Errorf("ru: missing %s", b+form)
						}
					}
				}
			}
		}
	}
}

func TestTranslateFollowsTheOwner(t *testing.T) {
	tag := "de-DE"
	SetLocale(func(context.Context) string { return tag })
	defer SetLocale(nil)
	ctx := context.Background()
	if got := T(ctx, "msg.routine.created", "name", "X", "next", "9:00"); !strings.Contains(got, "X") || strings.Contains(got, "Rotina") {
		t.Fatalf("de: %q", got)
	}
	tag = "pt-BR"
	if got := N(ctx, "msg.explore.simulated", 2); got != "(2 ações foram só simuladas; nada foi alterado.)" {
		t.Fatalf("plural: %q", got)
	}
	tag = "xx"
	if got := T(ctx, "btn.deny"); got != "Deny" {
		t.Fatalf("unknown language falls back to English: %q", got)
	}
	if got := T(ctx, "no.such.key"); got != "no.such.key" {
		t.Fatalf("missing key: %q", got)
	}
	for n, want := range map[int]string{1: "one", 2: "few", 5: "many", 11: "many", 21: "one", 22: "few"} {
		if got := plural("ru", n); got != want {
			t.Errorf("ru %d: %s, want %s", n, got, want)
		}
	}
}

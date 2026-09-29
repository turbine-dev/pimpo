// Package i18n holds the messages Pimpo sends outside the web app (notices,
// chat replies, approval requests) and the texts of the built-in
// connectors, in every language the app offers. The web app has its own
// dictionaries; these follow the same language setting.
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
)

//go:embed locales/*.json
var files embed.FS

var (
	once     sync.Once
	catalogs map[string]map[string]string
)

func load() {
	catalogs = map[string]map[string]string{}
	entries, _ := files.ReadDir("locales")
	for _, e := range entries {
		b, err := files.ReadFile(path.Join("locales", e.Name()))
		if err != nil {
			continue
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			panic(fmt.Sprintf("i18n: %s: %v", e.Name(), err))
		}
		catalogs[strings.TrimSuffix(e.Name(), ".json")] = m
	}
}

// Languages lists the catalogs, e.g. "pt", "en".
func Languages() []string {
	once.Do(load)
	var out []string
	for l := range catalogs {
		out = append(out, l)
	}
	return out
}

// locale gives the owner's language tag (e.g. "pt-BR") for a request; the
// app sets it from its settings with SetLocale. Without it messages are in
// Portuguese.
var locale atomic.Pointer[func(ctx context.Context) string]

// SetLocale sets where the owner's language comes from (nil for none).
func SetLocale(f func(ctx context.Context) string) {
	if f == nil {
		locale.Store(nil)
		return
	}
	locale.Store(&f)
}

// Lang is the language of a tag: "pt-BR" -> "pt". Unknown ones become "en".
func Lang(tag string) string {
	once.Do(load)
	l := strings.ToLower(strings.SplitN(strings.ReplaceAll(tag, "_", "-"), "-", 2)[0])
	if _, ok := catalogs[l]; ok {
		return l
	}
	return "en"
}

// Of is the owner's language for this request.
func Of(ctx context.Context) string {
	f := locale.Load()
	if f == nil {
		return "pt"
	}
	return Lang((*f)(ctx))
}

// Lookup finds a message in one language only.
func Lookup(lang, key string) (string, bool) {
	once.Do(load)
	s, ok := catalogs[lang][key]
	return s, ok && s != ""
}

var slot = regexp.MustCompile(`\{(\w+)\}`)

// In translates key into lang, falling back to English, then Portuguese,
// then the key itself. vars are name, value pairs filling {name} slots.
func In(lang, key string, vars ...any) string {
	once.Do(load)
	s, ok := Lookup(lang, key)
	if !ok {
		if s, ok = Lookup("en", key); !ok {
			if s, ok = Lookup("pt", key); !ok {
				s = key
			}
		}
	}
	if len(vars) == 0 {
		return s
	}
	vals := map[string]string{}
	for i := 0; i+1 < len(vars); i += 2 {
		vals[fmt.Sprint(vars[i])] = fmt.Sprint(vars[i+1])
	}
	return slot.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := vals[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

// T translates key into the owner's language.
func T(ctx context.Context, key string, vars ...any) string { return In(Of(ctx), key, vars...) }

// N picks the plural form of key (key_one, key_few, key_many, key_other)
// for n in the owner's language and fills {count}.
func N(ctx context.Context, key string, n int, vars ...any) string {
	lang := Of(ctx)
	vars = append(vars, "count", n)
	if _, ok := Lookup(lang, key+"_"+plural(lang, n)); ok {
		return In(lang, key+"_"+plural(lang, n), vars...)
	}
	if n == 1 {
		return In(lang, key+"_one", vars...)
	}
	return In(lang, key+"_other", vars...)
}

// plural is the CLDR category of a whole number, for the languages here.
func plural(lang string, n int) string {
	if n < 0 {
		n = -n
	}
	switch lang {
	case "ja", "zh", "ko":
		return "other"
	case "ru":
		switch {
		case n%10 == 1 && n%100 != 11:
			return "one"
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return "few"
		default:
			return "many"
		}
	case "fr":
		if n == 0 || n == 1 {
			return "one"
		}
		if n != 0 && n%1000000 == 0 {
			return "many"
		}
		return "other"
	}
	if n == 1 {
		return "one"
	}
	return "other"
}

package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/denerFernandes/zodim/internal/capability"
)

type hit struct {
	Method, Path, Auth string
	Body               map[string]any
}

// fake answers by "METHOD /path" and records every request.
func fake(t *testing.T, routes map[string]string) (*httptest.Server, *[]hit) {
	var mu sync.Mutex
	var hits []hit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		mu.Lock()
		hits = append(hits, hit{r.Method, r.URL.Path, r.Header.Get("Authorization"), body})
		mu.Unlock()
		if out, ok := routes[r.Method+" "+r.URL.Path]; ok {
			w.Write([]byte(out))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func cfg(values map[string]string) Config {
	return func(_ context.Context, f string) (string, error) { return values[f], nil }
}

func call(t *testing.T, kind string, c Config, name string, args any) (any, error) {
	t.Helper()
	k, ok := Get(kind)
	if !ok {
		t.Fatalf("no kind %s", kind)
	}
	return k.Call(context.Background(), c, name, "", args)
}

func TestEveryCapabilityIsDocumented(t *testing.T) {
	for _, k := range All() {
		for _, s := range k.Specs {
			if !strings.HasPrefix(s.Name, strings.SplitN(s.Name, ".", 2)[0]+".") || s.Signature == "" || s.Returns == "" || !json.Valid([]byte(s.Schema)) {
				t.Errorf("%s: %+v", k.ID, s)
			}
			if capability.Catalog[s.Name].Name != s.Name {
				t.Errorf("%s not registered", s.Name)
			}
		}
	}
	if len(All()) != 9 {
		t.Fatalf("catalog has %d kinds", len(All()))
	}
}

func TestRSSAndAtom(t *testing.T) {
	srv, _ := fake(t, map[string]string{
		"GET /rss":  `<?xml version="1.0"?><rss><channel><item><title>Go 1.26 &amp; mais</title><link>https://go.dev/1</link><pubDate>Tue, 22 Sep 2026 10:00:00 +0000</pubDate><description>&lt;p&gt;Novidades &lt;b&gt;boas&lt;/b&gt;&lt;/p&gt;</description></item><item><title>B</title></item></channel></rss>`,
		"GET /atom": `<feed xmlns="http://www.w3.org/2005/Atom"><entry><title>Release v2</title><link rel="alternate" href="https://x.dev/v2"/><updated>2026-09-20T12:00:00Z</updated><summary>Big one</summary></entry></feed>`,
	})
	out, err := call(t, "rss", nil, "rss.read", map[string]any{"url": srv.URL + "/rss", "max": 1})
	items := out.([]map[string]any)
	if err != nil || len(items) != 1 || items[0]["title"] != "Go 1.26 & mais" || items[0]["summary"] != "Novidades boas" || items[0]["published"] != "2026-09-22T10:00:00Z" {
		t.Fatalf("rss %v %v", out, err)
	}
	out, _ = call(t, "rss", nil, "rss.read", map[string]any{"url": srv.URL + "/atom"})
	if items := out.([]map[string]any); items[0]["link"] != "https://x.dev/v2" || items[0]["summary"] != "Big one" {
		t.Fatalf("atom %v", out)
	}
	if _, err := call(t, "rss", nil, "rss.read", map[string]any{"url": "file:///etc/passwd"}); err == nil {
		t.Fatal("read a file URL")
	}
}

func TestGitHub(t *testing.T) {
	srv, hits := fake(t, map[string]string{
		"GET /repos/ana/app/issues":             `[{"number":7,"title":"Crash","state":"open","html_url":"u","updated_at":"t","user":{"login":"bob"},"labels":[{"name":"bug"}]},{"number":8,"title":"PR","pull_request":{}}]`,
		"POST /repos/ana/app/issues/7/comments": `{"html_url":"https://github.com/ana/app/issues/7#c1"}`,
	})
	BaseURL["github"] = srv.URL
	defer delete(BaseURL, "github")
	c := cfg(map[string]string{"token": "gh"})
	out, err := call(t, "github", c, "github.issues", map[string]any{"repo": "ana/app"})
	list := out.([]map[string]any)
	if err != nil || len(list) != 2 || list[0]["author"] != "bob" || list[1]["pull_request"] != true || (*hits)[0].Auth != "Bearer gh" {
		t.Fatalf("%v %v", out, err)
	}
	if out, err := call(t, "github", c, "github.comment", map[string]any{"repo": "ana/app", "number": 7, "body": "Obrigado!"}); err != nil || (*hits)[1].Body["body"] != "Obrigado!" || out.(map[string]any)["url"] == "" {
		t.Fatalf("comment %v %v", out, err)
	}
	if _, err := call(t, "github", c, "github.issues", map[string]any{"repo": "../../user"}); err == nil {
		t.Fatal("accepted a path as repo")
	}
	if _, err := call(t, "github", cfg(nil), "github.issues", map[string]any{"repo": "a/b"}); err == nil || !strings.Contains(err.Error(), "Connections") {
		t.Fatalf("missing token: %v", err)
	}
}

func TestTodoist(t *testing.T) {
	srv, hits := fake(t, map[string]string{
		"GET /tasks":          `[{"id":"1","content":"Pagar luz","priority":4,"project_id":"p","due":{"date":"2026-09-24"}},{"id":"2","content":"Sem data"}]`,
		"POST /tasks":         `{"id":"9"}`,
		"POST /tasks/1/close": ``,
	})
	BaseURL["todoist"] = srv.URL
	defer delete(BaseURL, "todoist")
	c := cfg(map[string]string{"token": "td"})
	out, _ := call(t, "todoist", c, "todoist.tasks", map[string]any{"filter": "today"})
	if l := out.([]map[string]any); len(l) != 2 || l[0]["due"] != "2026-09-24" || l[1]["due"] != "" {
		t.Fatalf("%v", out)
	}
	if out, err := call(t, "todoist", c, "todoist.add", map[string]any{"content": "Ligar para Ana", "due": "amanhã 9h"}); err != nil || out.(map[string]any)["id"] != "9" || (*hits)[1].Body["due_string"] != "amanhã 9h" {
		t.Fatalf("add %v %v", out, err)
	}
	if _, err := call(t, "todoist", c, "todoist.close", map[string]any{"id": "1"}); err != nil {
		t.Fatal(err)
	}
}

func TestNotion(t *testing.T) {
	srv, hits := fake(t, map[string]string{
		"POST /search":              `{"results":[{"id":"pg","url":"u","last_edited_time":"t","properties":{"Name":{"title":[{"plain_text":"Viagem "},{"plain_text":"2027"}]}}}]}`,
		"PATCH /blocks/pg/children": `{}`,
	})
	BaseURL["notion"] = srv.URL
	defer delete(BaseURL, "notion")
	c := cfg(map[string]string{"token": "nt"})
	out, _ := call(t, "notion", c, "notion.search", map[string]any{"query": "viagem"})
	if l := out.([]map[string]any); len(l) != 1 || l[0]["title"] != "Viagem 2027" {
		t.Fatalf("%v", out)
	}
	if _, err := call(t, "notion", c, "notion.append", map[string]any{"page_id": "pg", "text": "Reservar hotel"}); err != nil || !strings.Contains(flat((*hits)[1].Body), "Reservar hotel") {
		t.Fatalf("append %v %v", err, (*hits)[1].Body)
	}
}

func flat(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestObsidian(t *testing.T) {
	vault := t.TempDir()
	os.MkdirAll(filepath.Join(vault, "Diário"), 0o755)
	os.WriteFile(filepath.Join(vault, "Diário", "hoje.md"), []byte("# Hoje\nComprar pão\nLigar para Ana"), 0o644)
	os.MkdirAll(filepath.Join(vault, ".obsidian"), 0o755)
	os.WriteFile(filepath.Join(vault, ".obsidian", "x.md"), []byte("Ana secreta"), 0o644)
	c := cfg(map[string]string{"vault": vault})
	out, _ := call(t, "obsidian", c, "obsidian.search", map[string]any{"query": "ana"})
	if l := out.([]map[string]any); len(l) != 1 || l[0]["note"] != "Diário/hoje.md" || l[0]["line"] != 3 {
		t.Fatalf("%v", out)
	}
	if _, err := call(t, "obsidian", c, "obsidian.append", map[string]any{"note": "Diário/hoje", "text": "- Pagar luz"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(vault, "Diário", "hoje.md"))
	if !strings.HasSuffix(string(b), "Ligar para Ana\n- Pagar luz\n") {
		t.Fatalf("note %q", b)
	}
	kept, _ := filepath.Glob(filepath.Join(vault, ".zodim", "history", "Diário", "hoje.md.*"))
	if len(kept) != 1 {
		t.Fatal("previous version not kept")
	}
	for _, bad := range []string{"../fora.md", "/etc/passwd", ".obsidian/workspace", ".zodim/history/x"} {
		if _, err := call(t, "obsidian", c, "obsidian.append", map[string]any{"note": bad, "text": "x"}); err == nil && !strings.HasPrefix(bad, "../") {
			t.Fatalf("wrote to %s", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(vault), "fora.md")); err == nil {
		t.Fatal("wrote outside the vault")
	}
}

func TestHomeAssistant(t *testing.T) {
	srv, hits := fake(t, map[string]string{
		"GET /api/states":                   `[{"entity_id":"light.sala","state":"on","attributes":{"friendly_name":"Sala"}},{"entity_id":"lock.porta","state":"locked","attributes":{}}]`,
		"POST /api/services/light/turn_off": `[]`,
		"POST /api/services/lock/unlock":    `[]`,
	})
	c := cfg(map[string]string{"url": srv.URL, "token": "ha"})
	out, _ := call(t, "homeassistant", c, "ha.states", map[string]any{"domain": "light"})
	if l := out.([]map[string]any); len(l) != 1 || l[0]["name"] != "Sala" {
		t.Fatalf("%v", out)
	}
	if _, err := call(t, "homeassistant", c, "ha.call", map[string]any{"domain": "light", "service": "turn_off", "entity_id": "light.sala"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, "homeassistant", c, "ha.call", map[string]any{"domain": "lock", "service": "unlock", "entity_id": "lock.porta"}); err == nil {
		t.Fatal("a lock opened through ha.call")
	}
	if _, err := call(t, "homeassistant", c, "ha.call", map[string]any{"domain": "light", "service": "turn_on", "entity_id": "lock.porta"}); err == nil {
		t.Fatal("entity of another domain accepted")
	}
	if _, err := call(t, "homeassistant", c, "ha.critical", map[string]any{"domain": "lock", "service": "unlock", "entity_id": "lock.porta"}); err != nil {
		t.Fatal(err)
	}
	if capability.Catalog["ha.critical"].Risk != capability.Irreversible || (*hits)[len(*hits)-1].Auth != "Bearer ha" {
		t.Fatal("critical actions must be irreversible")
	}
}

func TestWebhooks(t *testing.T) {
	srv, hits := fake(t, map[string]string{"POST /hook": `ok`})
	if _, err := call(t, "slack", cfg(map[string]string{"webhook": srv.URL + "/hook"}), "slack.send", map[string]any{"text": "oi"}); err == nil {
		t.Fatal("slack accepted a webhook outside hooks.slack.com")
	}
	BaseURL["discord"] = srv.URL
	defer delete(BaseURL, "discord")
	if _, err := call(t, "discord", cfg(map[string]string{"webhook": srv.URL + "/hook"}), "discord.send", map[string]any{"text": "@everyone oi"}); err != nil {
		t.Fatal(err)
	}
	if (*hits)[0].Body["content"] != "@everyone oi" || !strings.Contains(flat((*hits)[0].Body["allowed_mentions"]), `"parse":[]`) {
		t.Fatalf("discord body %v", (*hits)[0].Body)
	}
}

package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/zodim/internal/llm"
	"github.com/denerFernandes/zodim/internal/memory"
)

func TestImportFromHermes(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	home := t.TempDir()
	for name, body := range map[string]string{
		".env":               "TELEGRAM_BOT_TOKEN=1:t\nEMAIL_ADDRESS=me@x.com\nEMAIL_PASSWORD=pw\nEMAIL_IMAP_HOST=imap.x.com\nEMAIL_SMTP_HOST=smtp.x.com\n",
		"memories/MEMORY.md": "Dentist is Dr. Lima\n§\nGym on Tuesdays",
		"SOUL.md":            "Never pay without asking.",
		"cron/jobs.json":     `{"jobs":[{"name":"weather","prompt":"Tell me the weather in Lisbon","schedule":{"kind":"cron","expr":"0 7 * * *"}}]}`,
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(home, name)), 0o755)
		os.WriteFile(filepath.Join(home, name), []byte(body), 0o600)
	}

	code, plan := ta.do(t, "POST", "/api/migrate/preview", map[string]any{"from": "hermes", "home": home})
	if code != 200 || len(plan["tasks"].([]any)) != 1 || plan["telegram"].(map[string]any)["has_bot"] != true {
		t.Fatalf("preview %d %v", code, plan)
	}
	if _, leaked := plan["telegram"].(map[string]any)["token"]; leaked {
		t.Fatal("the preview exposed the Telegram token")
	}

	code, n := ta.do(t, "POST", "/api/migrate/apply", map[string]any{"from": "hermes", "home": home, "memories": true, "rules": true, "tasks": true})
	if code != 200 || n["memories"] != 2.0 || n["rules"] != 1.0 || n["tasks"] != 1.0 || n["telegram"] != false {
		t.Fatalf("apply %d %v", code, n)
	}
	facts, _ := ta.Memory.List()
	for _, f := range facts {
		if f.Trust != memory.Low {
			t.Fatalf("imported fact trusted without asking: %+v", f)
		}
	}
	if _, err := ta.Vault.Get(context.Background(), "telegram.token"); err == nil {
		t.Fatal("secrets copied without asking")
	}

	imported, _ := ta.Store.Explorations(context.Background(), "imported")
	if len(imported) != 1 {
		t.Fatalf("imported tasks %+v", imported)
	}
	code, started := ta.do(t, "POST", "/api/explorations/"+imported[0].ID+"/explore", nil)
	if code != 202 || started["id"] == "" {
		t.Fatalf("explore %d %v", code, started)
	}
	if left, _ := ta.Store.Explorations(context.Background(), "imported"); len(left) != 0 {
		t.Fatal("the imported task stayed after exploring it")
	}
	if code, _ := ta.do(t, "POST", "/api/explorations/"+imported[0].ID+"/explore", nil); code != 404 {
		t.Fatalf("explored the same import twice: %d", code)
	}

	ta.do(t, "POST", "/api/migrate/apply", map[string]any{"from": "hermes", "home": home, "secrets": true})
	if tok, _ := ta.Vault.Get(context.Background(), "telegram.token"); tok != "1:t" {
		t.Fatal("telegram token not imported")
	}
	if pw, _ := ta.Vault.Get(context.Background(), "mail.password"); pw != "pw" {
		t.Fatal("mail password not imported")
	}
	if smtp, _ := ta.Events.Get(context.Background(), "mail.smtp"); smtp != "smtp.x.com:465" {
		t.Fatalf("smtp %q", smtp)
	}
}

func TestDevicePairingAndRevocation(t *testing.T) {
	ta := newApp(t, weatherAgent, &llm.Fake{})
	if code, _ := ta.do(t, "POST", "/api/pairing", map[string]string{"base": "http://zodim.example.com", "device": "Celular"}); code != 400 {
		t.Fatalf("accepted plain http on a public host: %d", code)
	}
	if code, out := ta.do(t, "POST", "/api/pairing", map[string]string{"base": "http://192.168.1.20:7788"}); code != 200 || out["link"] != nil {
		t.Fatalf("home network %d %v", code, out)
	}
	_, out := ta.do(t, "POST", "/api/pairing", map[string]string{"base": "https://zodim.tail1.ts.net/x", "device": "Celular da Ana"})
	link, _ := out["link"].(string)
	if !strings.HasPrefix(link, "https://zodim.tail1.ts.net/auth?token=") || strings.Contains(link, "token=tok") {
		t.Fatalf("link %q", link)
	}
	token := strings.TrimPrefix(link, "https://zodim.tail1.ts.net/auth?token=")
	get := func(tok string) int {
		req, _ := http.NewRequest("GET", ta.srv.URL+"/api/routines", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, _ := http.DefaultClient.Do(req)
		return resp.StatusCode
	}
	if get(token) != 200 {
		t.Fatal("the device token does not open Zodim")
	}
	resp, _ := http.Get(ta.srv.URL + "/auth?token=" + token)
	if resp.Request.URL.Path != "/" {
		t.Fatalf("login with the device link: %s", resp.Request.URL)
	}
	_, list := ta.do(t, "GET", "/api/pairing", nil)
	devs := list["devices"].([]any)
	if len(devs) != 1 || devs[0].(map[string]any)["name"] != "Celular da Ana" || devs[0].(map[string]any)["hash"] != nil {
		t.Fatalf("devices %v", devs)
	}
	ta.do(t, "DELETE", "/api/devices/"+devs[0].(map[string]any)["id"].(string), nil)
	if get(token) != 401 {
		t.Fatal("a revoked device still gets in")
	}
	if get("tok") != 200 {
		t.Fatal("revoking a device locked out the owner")
	}
}

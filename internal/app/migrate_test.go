package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/denerFernandes/vigia/internal/llm"
	"github.com/denerFernandes/vigia/internal/memory"
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

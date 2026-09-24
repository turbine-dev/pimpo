package migrate

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHermes(t *testing.T) {
	home := t.TempDir()
	write(t, home, map[string]string{
		"config.yaml":                  "model:\n  default: x\ntimezone: America/Sao_Paulo\n",
		".env":                         "TELEGRAM_BOT_TOKEN=\"123:abc\"\nTELEGRAM_ALLOWED_USERS=42, 43\nEMAIL_ADDRESS=me@example.com\nEMAIL_PASSWORD=pw\nEMAIL_IMAP_HOST=imap.example.com\n",
		"memories/MEMORY.md":           "Dentist is Dr. Lima\non Rua A\n§\nGym on Tuesdays",
		"memories/USER.md":             "Prefers short answers",
		"SOUL.md":                      "You are terse.",
		"cron/jobs.json":               `{"jobs":[{"id":"a1","name":"daily-brief","prompt":"Summarize my unread email","schedule":{"kind":"cron","expr":"0 9 * * *"},"enabled":true,"state":"scheduled","deliver":"telegram:42"},{"id":"a2","name":"poll","prompt":"Check prices","schedule":{"kind":"interval","minutes":30},"state":"paused"},{"id":"a3","name":"backup","script":"backup.sh","no_agent":true,"schedule":{"kind":"cron","expr":"0 3 * * *"}}]}`,
		"skills/email/triage/SKILL.md": "---\nname: triage\ndescription: Sort the inbox and label newsletters\n---\nLook at each email.\n",
		"skills/dev/deploy/SKILL.md":   "---\nname: deploy\ndescription: Deploy the site\nrequired_environment_variables:\n  - name: VERCEL_TOKEN\n---\n```bash\nvercel deploy\n```\n",
		"skills/.hub/x/SKILL.md":       "---\nname: hidden\n---\n",
	})
	p, err := Read("hermes", home)
	if err != nil {
		t.Fatal(err)
	}
	if p.Timezone != "America/Sao_Paulo" || len(p.Memories) != 3 || p.Memories[0].Text != "Dentist is Dr. Lima\non Rua A" || p.Memories[2].Topic != "sobre mim" {
		t.Fatalf("memories %+v", p.Memories)
	}
	if len(p.Rules) != 1 || p.Rules[0].File != "SOUL.md" {
		t.Fatalf("rules %+v", p.Rules)
	}
	if len(p.Tasks) != 2 || p.Tasks[0].Schedule != "cron 0 9 * * *" || !p.Tasks[0].Enabled || p.Tasks[1].Enabled || p.Tasks[1].Schedule != "a cada 30 minutos" {
		t.Fatalf("tasks %+v", p.Tasks)
	}
	if req := p.Tasks[0].Request(); !strings.Contains(req, "Quando: cron 0 9 * * * (America/Sao_Paulo)") || !strings.Contains(req, "telegram:42") {
		t.Fatalf("request %q", req)
	}
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "backup") {
		t.Fatalf("warnings %v", p.Warnings)
	}
	if len(p.Skills) != 2 || p.Skills[0].Name != "deploy" || p.Skills[0].Verdict != "no" || p.Skills[0].Secrets[0] != "VERCEL_TOKEN" || p.Skills[1].Verdict != "works" {
		t.Fatalf("skills %+v", p.Skills)
	}
	if p.Telegram.Token != "123:abc" || strings.Join(p.Telegram.Allowed, ",") != "42,43" {
		t.Fatalf("telegram %+v", p.Telegram)
	}
	if p.Mail.IMAP != "imap.example.com:993" || p.Mail.Password != "pw" || p.Mail.SMTP != "" {
		t.Fatalf("mail %+v", p.Mail)
	}
}

func TestOpenClaw(t *testing.T) {
	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	write(t, home, map[string]string{
		"openclaw.json": `{
  // JSON5, as OpenClaw writes it
  agents: { defaults: { workspace: "` + ws + `", userTimezone: "Europe/Lisbon" } },
  channels: { telegram: { enabled: true, botToken: "9:xyz", allowFrom: [42], dmPolicy: "allowlist" } },
  plugins: { entries: { imap: { config: { accounts: { main: { host: "imap.x.com", user: "me@x.com", password: { source: "env", id: "IMAP_PW" } } } } } } },
}`,
		"skills/weather/SKILL.md": "---\nname: weather\ndescription: Fetch the weather forecast from a JSON API and notify me\nmetadata:\n  openclaw: { requires: { bins: [curl] } }\n---\n",
	})
	write(t, ws, map[string]string{
		"MEMORY.md":            "# Family\n- Ana is my sister\n- Birthday on May 3\n\n# Work\nI work at Beta.\n",
		"USER.md":              "<!-- observed: 2026-01-02 | status: superseded -->\n- Answer in English\n<!-- observed: 2026-03-01 | status: active -->\n- Answer in Portuguese\n",
		"AGENTS.md":            "Never pay anything without asking.",
		"memory/2026-09-01.md": "chat log",
	})
	os.MkdirAll(filepath.Join(home, "state"), 0o755)
	db, err := sql.Open("sqlite", filepath.Join(home, "state", "openclaw.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`CREATE TABLE cron_jobs (store_key TEXT, job_id TEXT, name TEXT, description TEXT, enabled INTEGER, agent_id TEXT, payload_kind TEXT, job_json TEXT, state_json TEXT, sort_order INTEGER, updated_at TEXT)`)
	db.Exec(`INSERT INTO cron_jobs (name, enabled, job_json, sort_order) VALUES
	  ('brief', 1, '{"schedule":{"kind":"cron","expr":"0 9 * * *","tz":"America/New_York"},"payload":{"kind":"agentTurn","message":"Morning brief"},"delivery":{"mode":"announce","channel":"telegram","to":"42"}}', 1),
	  ('tick', 0, '{"schedule":{"kind":"every","everyMs":900000},"payload":{"kind":"agentTurn","message":"Check"}}', 2),
	  ('script', 1, '{"schedule":{"kind":"cron","expr":"0 1 * * *"},"payload":{"kind":"script"}}', 3),
	  ('watch', 1, '{"schedule":{"kind":"stream"},"payload":{"kind":"agentTurn","message":"x"}}', 4)`)
	db.Close()

	p, err := Read("openclaw", home)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, m := range p.Memories {
		texts = append(texts, m.Topic+": "+m.Text)
	}
	if got := strings.Join(texts, "|"); got != "Family: Ana is my sister|Family: Birthday on May 3|Work: I work at Beta.|sobre mim: Answer in Portuguese" {
		t.Fatalf("memories %s", got)
	}
	if len(p.Rules) != 1 || p.Rules[0].File != "AGENTS.md" {
		t.Fatalf("rules %+v", p.Rules)
	}
	if len(p.Tasks) != 2 || p.Tasks[0].Timezone != "America/New_York" || p.Tasks[0].Deliver != "telegram" || p.Tasks[1].Schedule != "a cada 15m0s" || p.Tasks[1].Enabled || p.Tasks[1].Timezone != "Europe/Lisbon" {
		t.Fatalf("tasks %+v", p.Tasks)
	}
	if len(p.Skills) != 1 || p.Skills[0].Verdict != "partial" || !strings.Contains(strings.Join(p.Skills[0].Missing, ","), "curl") {
		t.Fatalf("skills %+v", p.Skills)
	}
	if p.Telegram.Token != "9:xyz" || p.Telegram.Allowed[0] != "42" || p.Mail.Address != "me@x.com" || p.Mail.Password != "" {
		t.Fatalf("channels %+v %+v", p.Telegram, p.Mail)
	}
	if w := strings.Join(p.Warnings, "\n"); !strings.Contains(w, "1 notas diárias") || !strings.Contains(w, "script") || !strings.Contains(w, "stream") || !strings.Contains(w, "SMTP") {
		t.Fatalf("warnings %s", w)
	}
}

func TestMissingHome(t *testing.T) {
	if _, err := Read("hermes", filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("read a home that does not exist")
	}
	if _, err := Read("nanobot", t.TempDir()); err == nil {
		t.Fatal("accepted an unknown source")
	}
}

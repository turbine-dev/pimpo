package migrate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/titanous/json5"
	_ "modernc.org/sqlite"
)

type openclawConfig struct {
	Agents struct {
		Defaults struct {
			Workspace    string `json:"workspace"`
			UserTimezone string `json:"userTimezone"`
		} `json:"defaults"`
	} `json:"agents"`
	Channels struct {
		Telegram struct {
			BotToken  any   `json:"botToken"`
			AllowFrom []any `json:"allowFrom"`
		} `json:"telegram"`
	} `json:"channels"`
	Plugins struct {
		Entries struct {
			IMAP struct {
				Config struct {
					Accounts map[string]struct {
						Host     string `json:"host"`
						User     string `json:"user"`
						Password any    `json:"password"`
					} `json:"accounts"`
				} `json:"config"`
			} `json:"imap"`
		} `json:"entries"`
	} `json:"plugins"`
}

type openclawJob struct {
	Name     string `json:"name"`
	Enabled  *bool  `json:"enabled"`
	Schedule struct {
		Kind    string  `json:"kind"`
		Expr    string  `json:"expr"`
		TZ      string  `json:"tz"`
		EveryMs float64 `json:"everyMs"`
		At      any     `json:"at"`
	} `json:"schedule"`
	Payload struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
		Text    string `json:"text"`
	} `json:"payload"`
	Delivery struct {
		Mode    string `json:"mode"`
		Channel string `json:"channel"`
		To      string `json:"to"`
	} `json:"delivery"`
}

func readOpenClaw(p *Plan) error {
	var cfg openclawConfig
	if raw := readFile(envOr("OPENCLAW_CONFIG_PATH", filepath.Join(p.Home, "openclaw.json"))); raw != "" {
		if err := json5.Unmarshal([]byte(raw), &cfg); err != nil {
			return fmt.Errorf("openclaw.json: %w", err)
		}
	}
	p.Timezone = cfg.Agents.Defaults.UserTimezone
	ws := envOr("OPENCLAW_WORKSPACE_DIR", cfg.Agents.Defaults.Workspace)
	if ws == "" {
		ws = filepath.Join(p.Home, "workspace")
	}
	ws = expandHome(ws)

	for _, m := range markdownEntries(readFile(filepath.Join(ws, "MEMORY.md")), "memória") {
		m.Source = "openclaw:MEMORY.md"
		p.Memories = append(p.Memories, m)
	}
	for _, d := range userDirectives(readFile(filepath.Join(ws, "USER.md"))) {
		p.Memories = append(p.Memories, Memory{Text: d, Topic: "sobre mim", Source: "openclaw:USER.md"})
	}
	if notes, _ := filepath.Glob(filepath.Join(ws, "memory", "*.md")); len(notes) > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%d notas diárias em memory/ ficaram de fora: são registros de conversa, não fatos", len(notes)))
	}
	p.rule(ws, "AGENTS.md")
	p.rule(ws, "SOUL.md")

	jobs, err := openclawJobs(p.Home)
	if err != nil {
		p.Warnings = append(p.Warnings, "não consegui ler as tarefas agendadas: "+err.Error())
	}
	for _, j := range jobs {
		t := Task{Name: j.Name, Prompt: j.Payload.Message, Timezone: j.Schedule.TZ, Enabled: j.Enabled == nil || *j.Enabled}
		if t.Prompt == "" {
			t.Prompt = j.Payload.Text
		}
		if t.Timezone == "" {
			t.Timezone = p.Timezone
		}
		switch j.Schedule.Kind {
		case "cron":
			t.Schedule = "cron " + j.Schedule.Expr
		case "every":
			t.Schedule = "a cada " + (time.Duration(j.Schedule.EveryMs) * time.Millisecond).String()
		case "at":
			t.Schedule = fmt.Sprint("uma vez, em ", j.Schedule.At)
		default:
			p.Warnings = append(p.Warnings, fmt.Sprintf("tarefa %q usa um gatilho %q que não tem equivalente", j.Name, j.Schedule.Kind))
			continue
		}
		if j.Delivery.Mode == "announce" && j.Delivery.Channel != "" {
			t.Deliver = j.Delivery.Channel
		}
		if t.Prompt == "" {
			p.Warnings = append(p.Warnings, fmt.Sprintf("tarefa %q roda um %s, não um pedido; rotinas do Pimpo não executam programas", j.Name, j.Payload.Kind))
			continue
		}
		p.Tasks = append(p.Tasks, t)
	}

	for _, dir := range []string{filepath.Join(ws, "skills"), filepath.Join(ws, ".agents", "skills"), filepath.Join(p.Home, "skills")} {
		skills, err := findSkills(dir)
		if err != nil {
			return err
		}
		p.Skills = append(p.Skills, skills...)
	}

	tg := cfg.Channels.Telegram
	if tok, ok := tg.BotToken.(string); ok && tok != "" {
		p.Telegram = Telegram{Token: tok, HasBot: true}
		for _, id := range tg.AllowFrom {
			p.Telegram.Allowed = append(p.Telegram.Allowed, strings.TrimSuffix(fmt.Sprint(id), ".0"))
		}
	} else if tg.BotToken != nil {
		p.Telegram.HasBot = true
		p.Warnings = append(p.Warnings, "o token do Telegram está num cofre do OpenClaw; cole-o em Conexões")
	}
	for name, acct := range cfg.Plugins.Entries.IMAP.Config.Accounts {
		p.Mail = Mail{Address: acct.User, IMAP: hostPort(acct.Host, "", "993")}
		if pw, ok := acct.Password.(string); ok {
			p.Mail.Password = pw
		}
		p.Warnings = append(p.Warnings, fmt.Sprintf("conta de e-mail %q: o OpenClaw só lia; configure o envio (SMTP) em Conexões", name))
		break
	}
	return nil
}

// openclawJobs reads scheduled jobs from the state database, or from the
// legacy jobs.json that older versions kept.
func openclawJobs(home string) ([]openclawJob, error) {
	var out []openclawJob
	dbPath := filepath.Join(home, "state", "openclaw.sqlite")
	if _, err := os.Stat(dbPath); err == nil {
		db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
		if err != nil {
			return nil, err
		}
		defer db.Close()
		rows, err := db.Query(`SELECT name, enabled, job_json FROM cron_jobs ORDER BY sort_order`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var name, raw string
			var enabled bool
			if err := rows.Scan(&name, &enabled, &raw); err != nil {
				return nil, err
			}
			var j openclawJob
			json.Unmarshal([]byte(raw), &j)
			if j.Name == "" {
				j.Name = name
			}
			j.Enabled = &enabled
			out = append(out, j)
		}
		return out, rows.Err()
	}
	if raw := readFile(filepath.Join(home, "cron", "jobs.json")); raw != "" {
		var file struct {
			Jobs []openclawJob `json:"jobs"`
		}
		if err := json5.Unmarshal([]byte(raw), &file); err != nil {
			return nil, err
		}
		out = file.Jobs
	}
	return out, nil
}

// userDirectives returns the active entries of an OpenClaw USER.md: each is
// a "<!-- observed: ... | status: ... -->" marker followed by one line.
func userDirectives(text string) []string {
	var out []string
	skip := false
	marked := false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "<!--") {
			marked = true
			skip = strings.Contains(t, "status: superseded")
			continue
		}
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if marked && skip {
			skip = false
			continue
		}
		out = append(out, strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(t, "- "), "* ")))
	}
	return out
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		h, _ := os.UserHomeDir()
		return filepath.Join(h, p[2:])
	}
	return p
}

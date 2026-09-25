package migrate

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func readHermes(p *Plan) error {
	var cfg struct {
		Timezone string `yaml:"timezone"`
	}
	yaml.Unmarshal([]byte(readFile(filepath.Join(p.Home, "config.yaml"))), &cfg)
	env := dotenv(readFile(filepath.Join(p.Home, ".env")))
	p.Timezone = cfg.Timezone
	if tz := env["HERMES_TIMEZONE"]; tz != "" {
		p.Timezone = tz
	}

	for _, f := range []struct{ file, topic string }{{"MEMORY.md", "memória"}, {"USER.md", "sobre mim"}} {
		for _, entry := range strings.Split(readFile(filepath.Join(p.Home, "memories", f.file)), "\n§\n") {
			if t := strings.TrimSpace(entry); t != "" {
				p.Memories = append(p.Memories, Memory{Text: t, Topic: f.topic, Source: "hermes:" + f.file})
			}
		}
	}
	p.rule(p.Home, "SOUL.md")

	if err := hermesJobs(p); err != nil {
		return err
	}
	skills, err := findSkills(filepath.Join(p.Home, "skills"))
	if err != nil {
		return err
	}
	p.Skills = append(p.Skills, skills...)

	if tok := env["TELEGRAM_BOT_TOKEN"]; tok != "" {
		p.Telegram = Telegram{Token: tok, HasBot: true, Allowed: list(env["TELEGRAM_ALLOWED_USERS"])}
	}
	if addr := env["EMAIL_ADDRESS"]; addr != "" {
		p.Mail = Mail{Address: addr, Password: env["EMAIL_PASSWORD"],
			IMAP: hostPort(env["EMAIL_IMAP_HOST"], env["EMAIL_IMAP_PORT"], "993"),
			SMTP: hostPort(env["EMAIL_SMTP_HOST"], env["EMAIL_SMTP_PORT"], "465")}
	}
	return nil
}

func hermesJobs(p *Plan) error {
	raw := readFile(filepath.Join(p.Home, "cron", "jobs.json"))
	if raw == "" {
		return nil
	}
	var file struct {
		Jobs []struct {
			Name     string   `json:"name"`
			Prompt   string   `json:"prompt"`
			Skills   []string `json:"skills"`
			Script   string   `json:"script"`
			Enabled  *bool    `json:"enabled"`
			State    string   `json:"state"`
			Deliver  string   `json:"deliver"`
			Schedule struct {
				Kind    string  `json:"kind"`
				Expr    string  `json:"expr"`
				Minutes float64 `json:"minutes"`
				RunAt   string  `json:"run_at"`
			} `json:"schedule"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(raw), &file); err != nil {
		return fmt.Errorf("hermes cron/jobs.json: %w", err)
	}
	for _, j := range file.Jobs {
		t := Task{Name: j.Name, Prompt: j.Prompt, Timezone: p.Timezone, Deliver: j.Deliver,
			Enabled: (j.Enabled == nil || *j.Enabled) && j.State != "paused"}
		switch j.Schedule.Kind {
		case "cron":
			t.Schedule = "cron " + j.Schedule.Expr
		case "interval":
			t.Schedule = fmt.Sprintf("a cada %g minutos", j.Schedule.Minutes)
		case "once":
			t.Schedule = "uma vez, em " + j.Schedule.RunAt
		}
		if len(j.Skills) > 0 {
			t.Prompt += "\n(usava as skills: " + strings.Join(j.Skills, ", ") + ")"
		}
		if t.Prompt == "" && j.Script != "" {
			p.Warnings = append(p.Warnings, fmt.Sprintf("tarefa %q só roda um script (%s); rotinas do Zodim não executam programas", j.Name, j.Script))
			continue
		}
		p.Tasks = append(p.Tasks, t)
	}
	return nil
}

func dotenv(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}

func list(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func hostPort(host, port, fallback string) string {
	if host == "" {
		return ""
	}
	if port == "" {
		port = fallback
	}
	return host + ":" + port
}

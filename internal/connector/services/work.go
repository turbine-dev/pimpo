package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/denerFernandes/pimpo/internal/capability"
	"github.com/denerFernandes/pimpo/internal/connector"
)

func init() {
	register(Kind{
		ID: "github", Title: "GitHub", Description: "Issues e pull requests dos seus repositórios.",
		Help:   "Crie um token fine-grained em github.com/settings/tokens com acesso só de leitura a issues, e escrita se quiser que eu comente.",
		Fields: []Field{{Name: "token", Label: "Token", Placeholder: "github_pat_…", Secret: true}},
		Specs: []capability.Spec{
			{Name: "github.issues", Risk: capability.Read, Signature: "github.issues({repo, state, max})", Returns: "[{number, title, state, author, labels, url, updated, pull_request: bool}] repo is owner/name",
				Schema: obj(`"repo":{"type":"string","description":"owner/name"},"state":{"type":"string","enum":["open","closed","all"]},"max":{"type":"integer"}`, "repo")},
			{Name: "github.comment", Risk: capability.Irreversible, Signature: "github.comment({repo, number, body})", Returns: "{ok, url}; everyone watching the issue sees it",
				Schema: obj(`"repo":{"type":"string"},"number":{"type":"integer"},"body":{"type":"string"}`, "repo", "number", "body")},
		},
		Call: callGitHub,
		Probe: func(ctx context.Context, cfg Config) error {
			v, err := need(ctx, cfg, "GitHub", "token")
			if err != nil {
				return err
			}
			return doJSON(ctx, "GET", base("github", "https://api.github.com")+"/user", map[string]string{"Authorization": "Bearer " + v[0]}, nil, nil)
		},
	})
	register(Kind{
		ID: "todoist", Title: "Todoist", Description: "Suas tarefas: ler, criar e concluir.",
		Help:   "Em Todoist › Configurações › Integrações › Desenvolvedor, copie o token da API.",
		Fields: []Field{{Name: "token", Label: "Token da API", Secret: true}},
		Specs: []capability.Spec{
			{Name: "todoist.tasks", Risk: capability.Read, Signature: "todoist.tasks({filter})", Returns: "[{id, content, due, priority, project_id, labels}] filter uses Todoist syntax, e.g. today | overdue",
				Schema: obj(`"filter":{"type":"string"}`)},
			{Name: "todoist.add", Risk: capability.Reversible, Signature: "todoist.add({content, due})", Returns: "{id}; due is natural language, e.g. tomorrow 9am",
				Schema: obj(`"content":{"type":"string"},"due":{"type":"string"}`, "content")},
			{Name: "todoist.close", Risk: capability.Reversible, Signature: "todoist.close({id})", Returns: "{ok}; can be reopened",
				Schema: obj(`"id":{"type":"string"}`, "id")},
		},
		Call: callTodoist,
		Probe: func(ctx context.Context, cfg Config) error {
			_, err := callTodoist(ctx, cfg, "todoist.tasks", "", map[string]any{"filter": "today"})
			return err
		},
	})
	register(Kind{
		ID: "notion", Title: "Notion", Description: "Procura páginas e acrescenta anotações.",
		Help:   "Crie uma integração interna em notion.so/my-integrations e compartilhe com ela só as páginas que eu posso ver.",
		Fields: []Field{{Name: "token", Label: "Token da integração", Placeholder: "ntn_…", Secret: true}},
		Specs: []capability.Spec{
			{Name: "notion.search", Risk: capability.Read, Signature: "notion.search({query})", Returns: "[{id, title, url, edited}]",
				Schema: obj(`"query":{"type":"string"}`)},
			{Name: "notion.append", Risk: capability.Reversible, Signature: "notion.append({page_id, text})", Returns: "{ok}; adds a paragraph at the end of the page, which keeps its history",
				Schema: obj(`"page_id":{"type":"string"},"text":{"type":"string"}`, "page_id", "text")},
		},
		Call: callNotion,
		Probe: func(ctx context.Context, cfg Config) error {
			_, err := callNotion(ctx, cfg, "notion.search", "", map[string]any{"query": ""})
			return err
		},
	})
	register(Kind{
		ID: "obsidian", Title: "Obsidian", Description: "Suas notas em Markdown, na pasta do cofre.",
		Help:   "Informe a pasta do cofre neste computador. Antes de mudar uma nota, guardo uma cópia em .pimpo/history.",
		Fields: []Field{{Name: "vault", Label: "Pasta do cofre", Placeholder: "/Users/voce/Documents/Notas"}},
		Specs: []capability.Spec{
			{Name: "obsidian.search", Risk: capability.Read, Signature: "obsidian.search({query, max})", Returns: "[{note, line, text}] note is the path inside the vault",
				Schema: obj(`"query":{"type":"string"},"max":{"type":"integer"}`, "query")},
			{Name: "obsidian.append", Risk: capability.Reversible, Signature: "obsidian.append({note, text})", Returns: "{ok}; appends to the note (creates it if missing); the previous version is kept",
				Schema: obj(`"note":{"type":"string","description":"path inside the vault, e.g. Diário/2026-09-24.md"},"text":{"type":"string"}`, "note", "text")},
		},
		Call: callObsidian,
		Probe: func(ctx context.Context, cfg Config) error {
			_, err := callObsidian(ctx, cfg, "obsidian.search", "", map[string]any{"query": "pimpo-probe"})
			return err
		},
	})
}

var repoName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func callGitHub(ctx context.Context, cfg Config, name, _ string, args any) (any, error) {
	v, err := need(ctx, cfg, "GitHub", "token")
	if err != nil {
		return nil, err
	}
	h := map[string]string{"Authorization": "Bearer " + v[0], "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"}
	api := base("github", "https://api.github.com")
	var a struct {
		Repo   string `json:"repo"`
		State  string `json:"state"`
		Max    int    `json:"max"`
		Number int    `json:"number"`
		Body   string `json:"body"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	if !repoName.MatchString(a.Repo) {
		return nil, errors.New("repo must look like owner/name")
	}
	switch name {
	case "github.issues":
		if a.State == "" {
			a.State = "open"
		}
		if a.Max <= 0 || a.Max > 100 {
			a.Max = 30
		}
		var raw []struct {
			Number    int    `json:"number"`
			Title     string `json:"title"`
			State     string `json:"state"`
			HTMLURL   string `json:"html_url"`
			UpdatedAt string `json:"updated_at"`
			User      struct {
				Login string `json:"login"`
			} `json:"user"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
			PullRequest *struct{} `json:"pull_request"`
		}
		q := url.Values{"state": {a.State}, "per_page": {fmt.Sprint(a.Max)}, "sort": {"updated"}}
		if err := doJSON(ctx, "GET", api+"/repos/"+a.Repo+"/issues?"+q.Encode(), h, nil, &raw); err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, i := range raw {
			labels := []string{}
			for _, l := range i.Labels {
				labels = append(labels, l.Name)
			}
			out = append(out, map[string]any{"number": i.Number, "title": i.Title, "state": i.State, "author": i.User.Login, "labels": labels, "url": i.HTMLURL, "updated": i.UpdatedAt, "pull_request": i.PullRequest != nil})
		}
		return out, nil
	case "github.comment":
		if strings.TrimSpace(a.Body) == "" || a.Number <= 0 {
			return nil, errors.New("number and body are required")
		}
		var res struct {
			HTMLURL string `json:"html_url"`
		}
		if err := doJSON(ctx, "POST", fmt.Sprintf("%s/repos/%s/issues/%d/comments", api, a.Repo, a.Number), h, map[string]string{"body": a.Body}, &res); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "url": res.HTMLURL}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

func callTodoist(ctx context.Context, cfg Config, name, _ string, args any) (any, error) {
	v, err := need(ctx, cfg, "Todoist", "token")
	if err != nil {
		return nil, err
	}
	h := map[string]string{"Authorization": "Bearer " + v[0]}
	api := base("todoist", "https://api.todoist.com/rest/v2")
	var a struct {
		Filter  string `json:"filter"`
		Content string `json:"content"`
		Due     string `json:"due"`
		ID      string `json:"id"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	switch name {
	case "todoist.tasks":
		var raw []struct {
			ID        string   `json:"id"`
			Content   string   `json:"content"`
			Priority  int      `json:"priority"`
			ProjectID string   `json:"project_id"`
			Labels    []string `json:"labels"`
			Due       *struct {
				Date     string `json:"date"`
				Datetime string `json:"datetime"`
			} `json:"due"`
		}
		u := api + "/tasks"
		if a.Filter != "" {
			u += "?filter=" + url.QueryEscape(a.Filter)
		}
		if err := doJSON(ctx, "GET", u, h, nil, &raw); err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, t := range raw {
			due := ""
			if t.Due != nil {
				due = t.Due.Date
				if t.Due.Datetime != "" {
					due = t.Due.Datetime
				}
			}
			if t.Labels == nil {
				t.Labels = []string{}
			}
			out = append(out, map[string]any{"id": t.ID, "content": t.Content, "due": due, "priority": t.Priority, "project_id": t.ProjectID, "labels": t.Labels})
		}
		return out, nil
	case "todoist.add":
		if strings.TrimSpace(a.Content) == "" {
			return nil, errEmpty
		}
		body := map[string]string{"content": a.Content}
		if a.Due != "" {
			body["due_string"] = a.Due
		}
		var res struct {
			ID string `json:"id"`
		}
		if err := doJSON(ctx, "POST", api+"/tasks", h, body, &res); err != nil {
			return nil, err
		}
		return map[string]any{"id": res.ID}, nil
	case "todoist.close":
		if a.ID == "" {
			return nil, errors.New("id is required")
		}
		if err := doJSON(ctx, "POST", api+"/tasks/"+url.PathEscape(a.ID)+"/close", h, nil, nil); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

func callNotion(ctx context.Context, cfg Config, name, _ string, args any) (any, error) {
	v, err := need(ctx, cfg, "Notion", "token")
	if err != nil {
		return nil, err
	}
	h := map[string]string{"Authorization": "Bearer " + v[0], "Notion-Version": "2022-06-28"}
	api := base("notion", "https://api.notion.com/v1")
	var a struct {
		Query  string `json:"query"`
		PageID string `json:"page_id"`
		Text   string `json:"text"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	switch name {
	case "notion.search":
		var res struct {
			Results []struct {
				ID             string                 `json:"id"`
				URL            string                 `json:"url"`
				LastEditedTime string                 `json:"last_edited_time"`
				Properties     map[string]notionProps `json:"properties"`
			} `json:"results"`
		}
		body := map[string]any{"query": a.Query, "page_size": 20, "filter": map[string]string{"property": "object", "value": "page"}}
		if err := doJSON(ctx, "POST", api+"/search", h, body, &res); err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, r := range res.Results {
			title := ""
			for _, p := range r.Properties {
				for _, t := range p.Title {
					title += t.PlainText
				}
			}
			out = append(out, map[string]any{"id": r.ID, "title": title, "url": r.URL, "edited": r.LastEditedTime})
		}
		return out, nil
	case "notion.append":
		if strings.TrimSpace(a.Text) == "" || a.PageID == "" {
			return nil, errors.New("page_id and text are required")
		}
		block := map[string]any{"children": []any{map[string]any{"object": "block", "type": "paragraph",
			"paragraph": map[string]any{"rich_text": []any{map[string]any{"type": "text", "text": map[string]string{"content": a.Text}}}}}}}
		if err := doJSON(ctx, "PATCH", api+"/blocks/"+url.PathEscape(a.PageID)+"/children", h, block, nil); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

type notionProps struct {
	Title []struct {
		PlainText string `json:"plain_text"`
	} `json:"title"`
}

// inVault resolves a note path and refuses anything that leaves the vault.
func inVault(vault, note string) (string, error) {
	if note == "" || filepath.IsAbs(note) {
		return "", errors.New("note must be a path inside the vault")
	}
	p := filepath.Join(vault, filepath.Clean("/"+note))
	rel, err := filepath.Rel(vault, p)
	if err != nil || strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, ".pimpo") || strings.HasPrefix(rel, ".obsidian") {
		return "", errors.New("note must be a path inside the vault")
	}
	if !strings.HasSuffix(p, ".md") {
		p += ".md"
	}
	return p, nil
}

func callObsidian(ctx context.Context, cfg Config, name, _ string, args any) (any, error) {
	v, err := need(ctx, cfg, "Obsidian", "vault")
	if err != nil {
		return nil, err
	}
	vault := v[0]
	if st, err := os.Stat(vault); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("the Obsidian vault %s is not a folder", vault)
	}
	var a struct {
		Query string `json:"query"`
		Max   int    `json:"max"`
		Note  string `json:"note"`
		Text  string `json:"text"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	switch name {
	case "obsidian.search":
		if a.Max <= 0 || a.Max > 100 {
			a.Max = 30
		}
		words := strings.Fields(strings.ToLower(a.Query))
		out := []map[string]any{}
		filepath.WalkDir(vault, func(p string, d os.DirEntry, err error) error {
			if err != nil || len(out) >= a.Max {
				return filepath.SkipDir
			}
			if d.IsDir() && strings.HasPrefix(d.Name(), ".") && p != vault {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(vault, p)
			for i, line := range strings.Split(string(b), "\n") {
				low := strings.ToLower(line)
				all := len(words) > 0
				for _, w := range words {
					all = all && strings.Contains(low, w)
				}
				if all {
					out = append(out, map[string]any{"note": filepath.ToSlash(rel), "line": i + 1, "text": strings.TrimSpace(line)})
					if len(out) >= a.Max {
						break
					}
				}
			}
			return nil
		})
		sort.SliceStable(out, func(i, j int) bool { return out[i]["note"].(string) < out[j]["note"].(string) })
		return out, nil
	case "obsidian.append":
		if strings.TrimSpace(a.Text) == "" {
			return nil, errEmpty
		}
		p, err := inVault(vault, a.Note)
		if err != nil {
			return nil, err
		}
		old, _ := os.ReadFile(p)
		if len(old) > 0 {
			rel, _ := filepath.Rel(vault, p)
			keep := filepath.Join(vault, ".pimpo", "history", rel+"."+time.Now().Format("20060102-150405"))
			os.MkdirAll(filepath.Dir(keep), 0o755)
			if err := os.WriteFile(keep, old, 0o644); err != nil {
				return nil, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		text := a.Text
		if len(old) > 0 && !strings.HasSuffix(string(old), "\n") {
			text = "\n" + text
		}
		if err := os.WriteFile(p, append(old, []byte(text+"\n")...), 0o644); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

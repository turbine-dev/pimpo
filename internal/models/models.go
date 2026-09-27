// Package models finds the language models the owner can use: tools
// installed on this computer, local model servers, and the models each
// provider offers, with prices from OpenRouter's public catalog so the
// spending limit can count them. It never guesses a price.
package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/pimpo/internal/llm"
)

// Provider describes a model provider for the setup screen.
type Provider struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	KeyURL   string `json:"key_url,omitempty"`
	NeedsKey bool   `json:"needs_key"`
	Local    bool   `json:"local,omitempty"`
}

// Providers in the order the setup screen shows them.
var Providers = []Provider{
	{"anthropic", "Anthropic", "https://console.anthropic.com/settings/keys", true, false},
	{"openai", "OpenAI", "https://platform.openai.com/api-keys", true, false},
	{"google", "Google Gemini", "https://aistudio.google.com/apikey", true, false},
	{"openrouter", "OpenRouter", "https://openrouter.ai/settings/keys", true, false},
	{"dashscope", "Qwen (Alibaba DashScope)", "https://modelstudio.console.alibabacloud.com/?tab=api#/api", true, false},
	{"deepseek", "DeepSeek", "https://platform.deepseek.com/api_keys", true, false},
	{"groq", "Groq", "https://console.groq.com/keys", true, false},
	{"mistral", "Mistral", "https://console.mistral.ai/api-keys", true, false},
	{"xai", "xAI Grok", "https://console.x.ai", true, false},
	{"ollama", "Ollama", "", false, true},
	{"lmstudio", "LM Studio", "", false, true},
	{"custom", "OpenAI-compatible", "", false, false},
}

// Get returns a provider by id.
func Get(id string) (Provider, bool) {
	for _, p := range Providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// Model is one model a provider offers. Prices are USD per million
// tokens; Priced is false when no trustworthy price was found.
type Model struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	PriceIn  float64 `json:"price_in"`
	PriceOut float64 `json:"price_out"`
	Priced   bool    `json:"priced"`
	Context  int     `json:"context,omitempty"`
	Free     bool    `json:"free,omitempty"`
}

// Client reaches providers; HTTP and the OpenRouter address can be
// replaced in tests.
type Client struct {
	HTTP       *http.Client
	OpenRouter string

	mu      sync.Mutex
	priced  []orModel
	fetched time.Time
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

type orModel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Context int    `json:"context_length"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

// catalog is OpenRouter's public model list, kept for six hours.
func (c *Client) catalog(ctx context.Context) ([]orModel, error) {
	c.mu.Lock()
	if time.Since(c.fetched) < 6*time.Hour && c.priced != nil {
		out := c.priced
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()
	base := c.OpenRouter
	if base == "" {
		base = llm.Bases["openrouter"]
	}
	var r struct {
		Data []orModel `json:"data"`
	}
	if err := c.get(ctx, base+"/models", nil, &r); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.priced, c.fetched = r.Data, time.Now()
	c.mu.Unlock()
	return r.Data, nil
}

func (c *Client) get(ctx context.Context, url string, header map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return ErrKey
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s answered %d", url, resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

// ErrKey means the provider refused the key.
var ErrKey = errors.New("the provider refused the key")

// orPrefix is how OpenRouter names each provider's models.
// DashScope serves Qwen and other open models, so any prefix may match.
var orPrefix = map[string]string{"anthropic": "anthropic/", "openai": "openai/", "google": "google/", "deepseek": "deepseek/", "mistral": "mistralai/", "xai": "x-ai/", "dashscope": ""}

var dated = regexp.MustCompile(`-(\d{8}|\d{4}-\d{2}-\d{2})$`)

// norm makes provider and OpenRouter ids comparable: claude-sonnet-4-5-20250929
// and anthropic/claude-sonnet-4.5 both become claude-sonnet-4-5.
func norm(id string) string {
	id = strings.ToLower(id)
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	id = strings.TrimPrefix(id, "models/")
	id = dated.ReplaceAllString(id, "")
	return strings.NewReplacer(".", "-", "_", "-", ":", "-").Replace(id)
}

func perMillion(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	return v * 1e6, true
}

// Endpoint is where a provider is reached, with its key if it needs one.
type Endpoint struct {
	Provider string
	Base     string
	Key      string
}

// List returns the models a provider offers the owner, priced from
// OpenRouter where it lists the same model.
func (c *Client) List(ctx context.Context, e Endpoint) ([]Model, error) {
	base := strings.TrimRight(e.Base, "/")
	if base == "" {
		base = llm.Bases[e.Provider]
	}
	var ids []string
	switch e.Provider {
	case "anthropic":
		var r struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
		}
		if err := c.get(ctx, base+"/models?limit=100", map[string]string{"x-api-key": e.Key, "anthropic-version": "2023-06-01"}, &r); err != nil {
			return nil, err
		}
		for _, m := range r.Data {
			ids = append(ids, m.ID)
		}
	case "openrouter":
		all, err := c.catalog(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]Model, 0, len(all))
		for _, m := range all {
			out = append(out, fromOR(m.ID, m))
		}
		sortModels(out)
		return out, nil
	default:
		h := map[string]string{}
		if e.Key != "" {
			h["Authorization"] = "Bearer " + e.Key
		}
		var r struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := c.get(ctx, base+"/models", h, &r); err != nil {
			return nil, err
		}
		for _, m := range r.Data {
			ids = append(ids, strings.TrimPrefix(m.ID, "models/"))
		}
	}
	local := e.Provider == "ollama" || e.Provider == "lmstudio"
	var priced map[string]orModel
	if prefix, ok := orPrefix[e.Provider]; ok {
		if all, err := c.catalog(ctx); err == nil {
			priced = map[string]orModel{}
			for _, m := range all {
				if strings.HasPrefix(m.ID, prefix) {
					priced[norm(m.ID)] = m
				}
			}
		}
	}
	out := make([]Model, 0, len(ids))
	for _, id := range ids {
		m := Model{ID: id, Name: id}
		if local {
			m.Free, m.Priced = true, true
		} else if or, ok := priced[norm(id)]; ok {
			m = fromOR(id, or)
		}
		out = append(out, m)
	}
	sortModels(out)
	return out, nil
}

func fromOR(id string, m orModel) Model {
	in, okIn := perMillion(m.Pricing.Prompt)
	outp, okOut := perMillion(m.Pricing.Completion)
	return Model{ID: id, Name: m.Name, PriceIn: round(in), PriceOut: round(outp), Priced: okIn && okOut, Context: m.Context, Free: okIn && okOut && in == 0 && outp == 0}
}

func round(v float64) float64 { return float64(int64(v*10000+0.5)) / 10000 }

func sortModels(ms []Model) {
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
}

// Found is what is installed or running on this computer.
type Found struct {
	ClaudeCode string  `json:"claude_code,omitempty"`
	Ollama     []Model `json:"ollama"`
	OllamaURL  string  `json:"ollama_url"`
	// OllamaUp and LMUp tell a server with no models from one not running.
	OllamaUp bool    `json:"ollama_up"`
	LMStudio []Model `json:"lmstudio"`
	LMURL    string  `json:"lmstudio_url"`
	LMUp     bool    `json:"lmstudio_up"`
	// Codex is the Codex CLI (in the ChatGPT app, or installed); CodexLogin
	// says a ChatGPT login is saved for it.
	Codex      string `json:"codex,omitempty"`
	CodexLogin bool   `json:"codex_login"`
	// QwenCode is the Qwen Code CLI; Apps are chat apps found, which offer
	// no way for other programs to use their models.
	QwenCode string   `json:"qwen_code,omitempty"`
	Apps     []string `json:"apps"`
}

// Detect looks for the Claude Code CLI and for Ollama and LM Studio
// answering at their addresses, quickly: a server that is not running is
// just absent.
func (c *Client) Detect(ctx context.Context, ollamaURL, lmURL string) Found {
	f := Found{Ollama: []Model{}, LMStudio: []Model{}, OllamaURL: ollamaURL, LMURL: lmURL, Apps: []string{}}
	f.Codex = llm.CodexBinary()
	if home, err := os.UserHomeDir(); err == nil {
		_, err := os.Stat(filepath.Join(home, ".codex", "auth.json"))
		f.CodexLogin = f.Codex != "" && err == nil
		for _, app := range []struct{ name, path string }{{"ChatGPT", "/Applications/ChatGPT.app"}, {"Qwen", "/Applications/Qwen.app"}, {"Claude", "/Applications/Claude.app"}} {
			if _, err := os.Stat(app.path); err == nil {
				f.Apps = append(f.Apps, app.name)
			} else if _, err := os.Stat(filepath.Join(home, strings.TrimPrefix(app.path, "/"))); err == nil {
				f.Apps = append(f.Apps, app.name)
			}
		}
	}
	if p, err := exec.LookPath("qwen"); err == nil {
		f.QwenCode = p
	}
	if f.OllamaURL == "" {
		f.OllamaURL = strings.TrimSuffix(llm.Bases["ollama"], "/v1")
	}
	if f.LMURL == "" {
		f.LMURL = strings.TrimSuffix(llm.Bases["lmstudio"], "/v1")
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if path, err := exec.LookPath("claude"); err == nil {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			out, _ := exec.CommandContext(ctx, path, "--version").Output()
			f.ClaudeCode = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(string(out)), "(Claude Code)"))
			if f.ClaudeCode == "" {
				f.ClaudeCode = "installed"
			}
		}
	}()
	probe := func(provider, base string, into *[]Model, up *bool) {
		defer wg.Done()
		if !reachable(base) {
			return
		}
		*up = true
		ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		if ms, err := c.List(ctx, Endpoint{Provider: provider, Base: strings.TrimRight(base, "/") + "/v1"}); err == nil {
			*into = ms
		}
	}
	go probe("ollama", f.OllamaURL, &f.Ollama, &f.OllamaUp)
	go probe("lmstudio", f.LMURL, &f.LMStudio, &f.LMUp)
	wg.Wait()
	return f
}

// reachable is a quick TCP check before asking a local server anything.
func reachable(base string) bool {
	host := strings.TrimPrefix(strings.TrimPrefix(base, "http://"), "https://")
	host, _, _ = strings.Cut(host, "/")
	conn, err := net.DialTimeout("tcp", host, 400*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Problem turns a failed call into a kind the owner can act on: key,
// credits, rate, model, network or other.
func Problem(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, ErrKey) || strings.Contains(msg, "refused the api key") || strings.Contains(msg, "401") || strings.Contains(msg, "invalid api key") || strings.Contains(msg, "authentication"):
		return "key"
	case strings.Contains(msg, "credit") || strings.Contains(msg, "billing") || strings.Contains(msg, "insufficient") || strings.Contains(msg, "quota") || strings.Contains(msg, "402"):
		return "credits"
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "rate_limit") || strings.Contains(msg, "overloaded") || strings.Contains(msg, "529") || strings.Contains(msg, "usage limit") || strings.Contains(msg, "limit reached"):
		return "rate"
	case strings.Contains(msg, "404") || strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist") || strings.Contains(msg, "model_not_found"):
		return "model"
	case strings.Contains(msg, "executable file not found") || strings.Contains(msg, "not installed"):
		return "missing"
	case strings.Contains(msg, "unreachable") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "timeout") || strings.Contains(msg, "no such host") || strings.Contains(msg, "deadline"):
		return "network"
	}
	return "other"
}

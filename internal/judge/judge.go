// Package judge answers the yes/no judgments inside routines ("is this
// email important?") with a probability. Backends: Jev (calibrated,
// needs a TypeSafe key), a cheap LLM, or a local model through Ollama.
package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/denerFernandes/vigia/internal/llm"
)

type Answer struct {
	P       float64
	CostUSD float64
	Backend string
}

type Judge interface {
	Ask(ctx context.Context, question string, item any) (Answer, error)
}

// Jev asks TypeSafe's Jev a Noul question: the probability that the
// statement is true for the item.
type Jev struct {
	Key     func(ctx context.Context) (string, error)
	BaseURL string
	HTTP    *http.Client
}

func (j Jev) Ask(ctx context.Context, question string, item any) (Answer, error) {
	key, err := j.Key(ctx)
	if err != nil {
		return Answer{}, err
	}
	base := j.BaseURL
	if base == "" {
		base = "https://api.typesafe.ai/v1/systemone"
	}
	body, _ := json.Marshal(map[string]any{
		"model": "jev-latest",
		"state": map[string]any{"item": item},
		"questions": map[string]any{"q": map[string]any{
			"type":         "noul",
			"instructions": question + " Answer about `item`.",
		}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := j.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Answer{}, fmt.Errorf("jev unreachable")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return Answer{}, fmt.Errorf("jev answered %d", resp.StatusCode)
	}
	var out struct {
		Answers map[string]struct {
			Noul float64 `json:"noul"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Answer{}, errors.New("jev returned an unreadable answer")
	}
	a, ok := out.Answers["q"]
	if !ok {
		return Answer{}, errors.New("jev returned no answer")
	}
	return Answer{P: clamp(a.Noul), Backend: "jev"}, nil
}

// LLM asks a language model for a probability. Less calibrated than Jev,
// but works with whatever model the owner already uses.
type LLM struct {
	Model llm.Model
	Name  string
}

var probSchema = json.RawMessage(`{"type":"object","required":["p"],"properties":{"p":{"type":"number","minimum":0,"maximum":1}},"additionalProperties":false}`)

func (l LLM) Ask(ctx context.Context, question string, item any) (Answer, error) {
	b, _ := json.Marshal(item)
	resp, err := l.Model.Generate(ctx, llm.Request{
		System:     "You judge one item. Reply with p, the probability (0 to 1) that the answer to the question is yes. Be calibrated: 0.5 means you cannot tell.",
		Prompt:     "Question: " + question + "\nItem: " + string(b),
		Schema:     probSchema,
		Model:      l.Name,
		MaxCostUSD: 0.01,
	})
	if err != nil {
		return Answer{}, err
	}
	var out struct {
		P float64 `json:"p"`
	}
	if err := json.Unmarshal(resp.Structured, &out); err != nil {
		return Answer{}, err
	}
	return Answer{P: clamp(out.P), CostUSD: resp.CostUSD, Backend: "llm"}, nil
}

// Ollama asks a local model through Ollama's API, free and offline. It reads
// the token probabilities of "yes" and "no" when the model exposes them.
type Ollama struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

func (o Ollama) Ask(ctx context.Context, question string, item any) (Answer, error) {
	base := o.BaseURL
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	b, _ := json.Marshal(item)
	body, _ := json.Marshal(map[string]any{
		"model":        o.Model,
		"stream":       false,
		"logprobs":     true,
		"top_logprobs": 5,
		"options":      map[string]any{"temperature": 0, "num_predict": 1},
		"messages": []map[string]string{
			{"role": "system", "content": "Answer the question about the item with a single word: yes or no."},
			{"role": "user", "content": "Question: " + question + "\nItem: " + string(b)},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	client := o.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Answer{}, fmt.Errorf("local model unreachable; is Ollama running?")
	}
	defer resp.Body.Close()
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Logprobs []struct {
			TopLogprobs []struct {
				Token   string  `json:"token"`
				Logprob float64 `json:"logprob"`
			} `json:"top_logprobs"`
		} `json:"logprobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Answer{}, errors.New("local model returned an unreadable answer")
	}
	if len(out.Logprobs) > 0 {
		var yes, no float64
		for _, t := range out.Logprobs[0].TopLogprobs {
			switch strings.ToLower(strings.TrimSpace(t.Token)) {
			case "yes":
				yes += math.Exp(t.Logprob)
			case "no":
				no += math.Exp(t.Logprob)
			}
		}
		if yes+no > 0 {
			return Answer{P: clamp(yes / (yes + no)), Backend: "local"}, nil
		}
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(out.Message.Content)), "yes") {
		return Answer{P: 0.8, Backend: "local"}, nil
	}
	return Answer{P: 0.2, Backend: "local"}, nil
}

// Local asks Vigia's own small judgment model (tools/judge/serve.py), which
// reads the model's probability of "yes" directly.
type Local struct {
	URL  string
	HTTP *http.Client
}

func (l Local) Ask(ctx context.Context, question string, item any) (Answer, error) {
	url := l.URL
	if url == "" {
		url = "http://127.0.0.1:11500"
	}
	body, _ := json.Marshal(map[string]any{"question": question, "item": item})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(url, "/")+"/judge", bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := l.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Answer{}, errors.New("local judgment model unreachable")
	}
	defer resp.Body.Close()
	var out struct {
		P float64 `json:"p"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&out) != nil {
		return Answer{}, errors.New("local judgment model returned an unreadable answer")
	}
	return Answer{P: clamp(out.P), Backend: "local"}, nil
}

// Chain tries backends in order until one answers.
type Chain []Judge

func (c Chain) Ask(ctx context.Context, question string, item any) (Answer, error) {
	var errs []string
	for _, j := range c {
		a, err := j.Ask(ctx, question, item)
		if err == nil {
			return a, nil
		}
		errs = append(errs, err.Error())
	}
	if len(errs) == 0 {
		return Answer{}, errors.New("no judgment backend configured")
	}
	return Answer{}, errors.New(strings.Join(errs, "; "))
}

func clamp(p float64) float64 { return math.Max(0, math.Min(1, p)) }

package app

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/llm"
)

// Claude Code signed in with a Claude plan, and Codex signed in with
// ChatGPT, are paid by the owner's subscription: the dollars they report
// are what the same work would cost on the API, not money spent. Pimpo
// keeps those apart from real spending and out of the daily limit.

// onSubscription says whether a model is paid by a subscription; tests
// replace it.
var onSubscription = func(model string) bool {
	switch {
	case isCodex(model):
		return subscriptionLogin("codex", func() bool {
			bin := llm.CodexBinary()
			if bin == "" {
				return false
			}
			out, _ := exec.Command(bin, "login", "status").CombinedOutput()
			return strings.Contains(string(out), "ChatGPT")
		})
	case llm.IsOpencode(model):
		provider, _, _ := strings.Cut(strings.TrimPrefix(model, "opencode:"), "/")
		return subscriptionLogin("opencode:"+provider, func() bool { return llm.OpencodeSubscription(context.Background(), provider) })
	case isClaudeCode(model):
		return subscriptionLogin("claude", func() bool {
			out, err := exec.Command("claude", "auth", "status").Output()
			if err != nil {
				return false
			}
			var s struct {
				AuthMethod string `json:"authMethod"`
			}
			json.Unmarshal(out, &s)
			return s.AuthMethod == "claude.ai"
		})
	}
	return false
}

func isClaudeCode(model string) bool {
	return model == "sonnet" || model == "opus" || model == "haiku" || strings.HasPrefix(model, "claude-")
}

var logins = struct {
	sync.Mutex
	at  map[string]time.Time
	sub map[string]bool
}{at: map[string]time.Time{}, sub: map[string]bool{}}

// subscriptionLogin asks a CLI how it is signed in, at most every ten
// minutes.
func subscriptionLogin(cli string, ask func() bool) bool {
	logins.Lock()
	if t, ok := logins.at[cli]; ok && time.Since(t) < 10*time.Minute {
		defer logins.Unlock()
		return logins.sub[cli]
	}
	logins.Unlock()
	sub := ask()
	logins.Lock()
	logins.at[cli], logins.sub[cli] = time.Now(), sub
	logins.Unlock()
	return sub
}

// billed moves a subscription call's reported cost out of spending: the
// response costs nothing, and the equivalent is recorded apart.
func (a *App) billed(ctx context.Context, job, model string, resp llm.Response) llm.Response {
	if resp.CostUSD <= 0 || !onSubscription(model) {
		return resp
	}
	a.Events.Append(ctx, "subscription.used", "system", map[string]any{"job": job, "model": model, "usd": resp.CostUSD})
	resp.CostUSD = 0
	return resp
}

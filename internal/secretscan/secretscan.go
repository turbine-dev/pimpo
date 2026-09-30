// Package secretscan finds passwords and keys pasted into a message, so
// Pimpo can take them out before the message reaches a model, a log or a
// conversation. It redacts and never blocks: a false alarm costs a few
// characters of the message, a miss would cost the secret.
package secretscan

import (
	"regexp"
	"strings"
)

// Mark replaces what was taken out.
const Mark = "[secret removed]"

// keys are formats that are a secret by their shape alone. Each needs a
// known prefix and enough random-looking characters that ordinary words,
// ids and commit hashes do not match.
var keys = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`),                       // Anthropic
	regexp.MustCompile(`\bsk-(?:proj-|or-v1-|svcacct-)?[A-Za-z0-9_-]{20,}`), // OpenAI, OpenRouter
	regexp.MustCompile(`\bsk_[a-f0-9]{40,}\b`),                              // ElevenLabs
	regexp.MustCompile(`\b(?:sk|rk|pk)_(?:live|test)_[A-Za-z0-9]{16,}`),     // Stripe
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}`),                      // GitHub
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{40,}`),
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}`),      // GitLab
	regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`), // Slack
	regexp.MustCompile(`\bxapp-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),                                    // AWS
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`),                                          // Google API key
	regexp.MustCompile(`\bya29\.[0-9A-Za-z_-]{20,}`),                                       // Google OAuth
	regexp.MustCompile(`\b1//[0-9A-Za-z_-]{30,}`),                                          // Google refresh token
	regexp.MustCompile(`\b(?:ntn|secret)_[A-Za-z0-9]{40,}`),                                // Notion
	regexp.MustCompile(`\bpplx-[A-Za-z0-9]{40,}`),                                          // Perplexity
	regexp.MustCompile(`\bgsk_[A-Za-z0-9]{40,}`),                                           // Groq
	regexp.MustCompile(`\bhf_[A-Za-z0-9]{30,}`),                                            // Hugging Face
	regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}`),                                            // npm
	regexp.MustCompile(`\b[0-9]{8,10}:AA[A-Za-z0-9_-]{33}\b`),                              // Telegram bot token
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{16,}`), // JWT
}

// labelled is a value written right after a word that names a secret,
// "password: hunter22" or "api_key=abc123": only the value goes.
var labelled = regexp.MustCompile(`(?i)\b(pass(?:word|wd)?|senha|contrase[ñn]a|mot de passe|passwort|parola d'ordine|пароль|api[ _-]?key|access[ _-]?key|secret(?:[ _-]?key)?|client[ _-]?secret|token|bearer|chave(?: da api)?)(\s*[:=]\s*|\s+)("[^"\n]{4,}"|'[^'\n]{4,}'|[^\s"',;]{6,})`)

// anyURL finds links in text; webhookURL then decides, by the exact host,
// which of them are chat webhooks whose address is itself the secret.
var anyURL = regexp.MustCompile(`https://[A-Za-z0-9.-]+/[A-Za-z0-9/_-]+`)

// webhookHosts are the hosts whose webhook links carry their own key, and
// the path those links start with.
var webhookHosts = map[string]string{
	"hooks.slack.com":    "/services/",
	"discord.com":        "/api/webhooks/",
	"ptb.discord.com":    "/api/webhooks/",
	"canary.discord.com": "/api/webhooks/",
	"discordapp.com":     "/api/webhooks/",
}

func webhookURL(u string) bool {
	rest := strings.TrimPrefix(u, "https://")
	host, path, ok := strings.Cut(rest, "/")
	prefix, known := webhookHosts[strings.ToLower(host)]
	return ok && known && strings.HasPrefix("/"+path, prefix) && len("/"+path) > len(prefix)
}

// Redact takes out what looks like a secret and says whether it did.
func Redact(text string) (string, bool) {
	out := text
	for _, re := range keys {
		out = re.ReplaceAllString(out, Mark)
	}
	out = anyURL.ReplaceAllStringFunc(out, func(u string) string {
		if webhookURL(u) {
			return Mark
		}
		return u
	})
	out = labelled.ReplaceAllStringFunc(out, func(m string) string {
		g := labelled.FindStringSubmatch(m)
		value := strings.Trim(g[3], `"'`)
		// "token: [secret removed]" was already handled; a bare word
		// after "password " is only a secret when it has the look of
		// one, so "password reset" or "token limit" stay.
		if strings.Contains(value, Mark) || (strings.TrimSpace(g[2]) == "" && !looksRandom(value)) {
			return m
		}
		return g[1] + g[2] + Mark
	})
	return out, out != text
}

// Only says whether nothing but removed secrets is left of a message.
func Only(redacted string) bool {
	if !strings.Contains(redacted, Mark) {
		return false
	}
	rest := strings.ReplaceAll(redacted, Mark, "")
	n := 0
	for _, r := range rest {
		if r > ' ' && !strings.ContainsRune(`.,;:!?"'()[]{}-_=`, r) {
			n++
		}
	}
	return n < 3
}

// looksRandom says whether a word mixes letters with digits or symbols
// the way generated passwords and keys do.
func looksRandom(s string) bool {
	var lower, upper, digit, other bool
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			other = true
		}
	}
	kinds := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			kinds++
		}
	}
	return len(s) >= 8 && kinds >= 3 || len(s) >= 6 && digit && (lower || upper)
}

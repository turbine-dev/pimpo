package push

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"unicode/utf8"
)

// SignatureOK checks GitHub's X-Hub-Signature-256 header: sha256= and the
// HMAC of the raw body with the webhook's secret, compared in constant time.
func SignatureOK(secret string, body []byte, header string) bool {
	hexSum, ok := strings.CutPrefix(header, "sha256=")
	if !ok || secret == "" {
		return false
	}
	got, err := hex.DecodeString(hexSum)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// Trim keeps what a routine can use of a GitHub payload: it drops the
// API links (keys ending in _url, except html_url), clips long texts to
// 2,000 characters and lists to 30 items, and stops 6 levels deep.
func Trim(v any) any { return trim(v, 0) }

func trim(v any, depth int) any {
	switch x := v.(type) {
	case map[string]any:
		if depth >= 6 {
			return "…"
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := map[string]any{}
		for _, k := range keys {
			if (strings.HasSuffix(k, "_url") && k != "html_url") || k == "url" || k == "node_id" {
				continue
			}
			out[k] = trim(x[k], depth+1)
		}
		return out
	case []any:
		if depth >= 6 {
			return "…"
		}
		n := min(len(x), 30)
		out := make([]any, n)
		for i := range n {
			out[i] = trim(x[i], depth+1)
		}
		return out
	case string:
		if utf8.RuneCountInString(x) > 2000 {
			return string([]rune(x)[:2000]) + "…"
		}
		return x
	}
	return v
}

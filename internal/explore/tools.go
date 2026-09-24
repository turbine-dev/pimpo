package explore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/denerFernandes/vigia/internal/capability"
	"github.com/denerFernandes/vigia/internal/host"
	"github.com/denerFernandes/vigia/internal/mcp"
)

// toolName maps "gmail.search" to "gmail_search"; MCP tool names cannot
// contain dots.
func toolName(c string) string { return strings.ReplaceAll(c, ".", "_") }

var schemas = map[string]string{
	"calendar.events": `{"type":"object","required":["from","to"],"properties":{"from":{"type":"string","description":"ISO date or time"},"to":{"type":"string","description":"ISO date or time"}}}`,
	"gmail.search":    `{"type":"object","properties":{"query":{"type":"string","description":"Gmail search syntax: from: to: subject: is:unread newer_than:3d OR"},"days":{"type":"integer"},"unread":{"type":"boolean"},"max":{"type":"integer"}}}`,
	"gmail.archive":   `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`,
	"gmail.label":     `{"type":"object","required":["id","label"],"properties":{"id":{"type":"string"},"label":{"type":"string"}}}`,
	"http.getJSON":    `{"type":"object","required":["url"],"properties":{"url":{"type":"string","description":"https URL returning JSON"}}}`,
	"telegram.send":   `{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}`,
}

// tools exposes every capability plus `decide`, which records a subjective
// decision so the compiled routine can make it again with a judgment.
func tools(h *host.Host) []mcp.Tool {
	var out []mcp.Tool
	for _, name := range capability.Names() {
		spec := capability.Catalog[name]
		name := name
		desc := fmt.Sprintf("%s -> %s. Risk: %s.", spec.Signature, spec.Returns, spec.Risk)
		if spec.Risk >= capability.Reversible {
			desc += " While exploring this is simulated: it is recorded and shown to the owner, not done."
		}
		schema := schemas[name]
		if schema == "" {
			schema = `{"type":"object"}`
		}
		out = append(out, mcp.Tool{Name: toolName(name), Description: desc, InputSchema: json.RawMessage(schema),
			Handle: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var args map[string]any
				json.Unmarshal(raw, &args)
				if args == nil {
					args = map[string]any{}
				}
				var callArgs any = args
				scope := ""
				if name == "http.getJSON" {
					url, _ := args["url"].(string)
					callArgs = url
					scope = hostOf(url)
				}
				return h.Call(ctx, name, scope, callArgs)
			}})
	}
	out = append(out, mcp.Tool{
		Name:        "decide",
		Description: "REQUIRED for every subjective decision: record your yes/no about ONE item (is this email important? is this a promotion?). Call it for EVERY item you judged, including the ones you leave out (yes=false), before acting. Without it the automatic routine cannot repeat your judgment.",
		InputSchema: json.RawMessage(`{"type":"object","required":["judgment","question","item","yes"],"properties":{
		  "judgment":{"type":"string","description":"short identifier, e.g. important, newsletter, needs_reply; reuse the same one for the same kind of decision"},
		  "question":{"type":"string","description":"the yes/no question, about one item"},
		  "item":{"type":"string","description":"the item's id exactly as the tool returned it (e.g. INBOX/12); if it has no id, a short unique piece of its text"},
		  "yes":{"type":"boolean"},
		  "confidence":{"type":"number","minimum":0.5,"maximum":1}}}`),
		Handle: func(_ context.Context, raw json.RawMessage) (any, error) {
			var d struct {
				Judgment   string  `json:"judgment"`
				Question   string  `json:"question"`
				Item       string  `json:"item"`
				Yes        bool    `json:"yes"`
				Confidence float64 `json:"confidence"`
			}
			if err := json.Unmarshal(raw, &d); err != nil || d.Judgment == "" || d.Item == "" {
				return nil, fmt.Errorf("decide needs judgment, item and yes")
			}
			c := d.Confidence
			if c < 0.5 || c > 1 {
				c = 0.9
			}
			p := c
			if !d.Yes {
				p = 1 - c
			}
			h.Label(ident(d.Judgment), d.Item, p)
			h.SetQuestion(ident(d.Judgment), d.Question)
			return map[string]bool{"recorded": true}, nil
		},
	})
	return out
}

func hostOf(url string) string {
	h := url
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	h, _, _ = strings.Cut(h, "/")
	h, _, _ = strings.Cut(h, "?")
	h, _, _ = strings.Cut(h, ":")
	return strings.ToLower(h)
}

// ident turns "needs reply" into "needs_reply" so it can be a JavaScript name.
func ident(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9' && b.Len() > 0:
			b.WriteRune(r)
		case b.Len() > 0:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

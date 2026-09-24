package routine

import (
	"strings"
	"time"
)

// Canned responses are recorded for one set of arguments, but a routine may
// ask differently, e.g. filter by sender on the server instead of in code.
// These filters make scenario responses behave like the real service for
// the arguments the routine actually sent.

// FilterResponse applies a read capability's arguments to canned data, the
// way the real service would. The demo mailbox uses it too.
func FilterResponse(capability string, args any, v any, now time.Time) any {
	return filterResponse(capability, args, v, now)
}

func filterResponse(capability string, args any, v any, now time.Time) any {
	a, _ := args.(map[string]any)
	list, ok := v.([]any)
	if !ok || a == nil {
		return v
	}
	switch capability {
	case "gmail.search":
		return filterMail(list, a, now)
	case "calendar.events":
		return filterEvents(list, a, now.Location())
	}
	return v
}

func filterMail(list []any, a map[string]any, now time.Time) []any {
	q, _ := a["query"].(string)
	match := mailMatcher(q, now)
	unread, _ := a["unread"].(bool)
	days := toInt(a["days"])
	var out []any
	for _, it := range list {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		if unread && !isUnread(m) {
			continue
		}
		if days > 0 {
			if d, err := time.Parse(time.RFC3339, str(m["date"])); err == nil && d.Before(now.AddDate(0, 0, -days)) {
				continue
			}
		}
		if match(m) {
			out = append(out, m)
		}
	}
	if max := toInt(a["max"]); max > 0 && len(out) > max {
		out = out[:max]
	}
	if out == nil {
		return []any{}
	}
	return out
}

func isUnread(m map[string]any) bool {
	if u, ok := m["unread"].(bool); ok {
		return u
	}
	labels, _ := m["labels"].([]any)
	for _, l := range labels {
		if str(l) == "UNREAD" {
			return true
		}
	}
	return len(labels) == 0
}

// mailMatcher understands from: to: subject: is:unread/read, -negation,
// OR, and bare words; unknown operators match everything.
func mailMatcher(q string, now time.Time) func(map[string]any) bool {
	toks := fields(q)
	type term struct {
		neg bool
		fn  func(map[string]any) bool
	}
	var groups [][]term // AND of groups, each group an OR of terms
	for i := 0; i < len(toks); i++ {
		if strings.EqualFold(toks[i], "OR") {
			continue
		}
		t := toks[i]
		neg := strings.HasPrefix(t, "-")
		t = strings.TrimPrefix(t, "-")
		fn := mailTerm(strings.Trim(t, `"()`), now)
		cur := term{neg, fn}
		if len(groups) > 0 && i > 0 && strings.EqualFold(toks[i-1], "OR") {
			groups[len(groups)-1] = append(groups[len(groups)-1], cur)
		} else {
			groups = append(groups, []term{cur})
		}
	}
	return func(m map[string]any) bool {
		for _, g := range groups {
			any := false
			for _, t := range g {
				if t.fn(m) != t.neg {
					any = true
					break
				}
			}
			if !any {
				return false
			}
		}
		return true
	}
}

// fields splits on spaces outside double quotes.
func fields(q string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range q {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func mailTerm(t string, now time.Time) func(map[string]any) bool {
	key, val, ok := strings.Cut(t, ":")
	has := func(field string, v string) func(map[string]any) bool {
		return func(m map[string]any) bool {
			return strings.Contains(strings.ToLower(flatten(m[field])), strings.ToLower(v))
		}
	}
	if !ok {
		return func(m map[string]any) bool {
			text := strings.ToLower(str(m["subject"]) + " " + str(m["snippet"]) + " " + str(m["from"]) + " " + str(m["from_name"]))
			return strings.Contains(text, strings.ToLower(t))
		}
	}
	switch strings.ToLower(key) {
	case "from":
		if strings.Contains(val, "@") {
			return exactAddress("from", val)
		}
		return func(m map[string]any) bool { return has("from", val)(m) || has("from_name", val)(m) }
	case "to":
		if strings.Contains(val, "@") {
			return exactAddress("to", val)
		}
		return has("to", val)
	case "subject":
		return has("subject", val)
	case "is":
		switch strings.ToLower(val) {
		case "unread":
			return isUnread
		case "read":
			return func(m map[string]any) bool { return !isUnread(m) }
		}
	}
	return func(map[string]any) bool { return true }
}

// exactAddress matches a full address the way Gmail does: ana@acme.com
// does not match mariana@acme.com.
func exactAddress(field, addr string) func(map[string]any) bool {
	addr = strings.ToLower(addr)
	return func(m map[string]any) bool {
		switch v := m[field].(type) {
		case string:
			return strings.ToLower(v) == addr
		case []any:
			for _, x := range v {
				if strings.ToLower(str(x)) == addr {
					return true
				}
			}
		}
		return false
	}
}

func filterEvents(list []any, a map[string]any, zone *time.Location) []any {
	from, to := str(a["from"]), str(a["to"])
	if from == "" || to == "" {
		return list
	}
	lo, errLo := parseAny(from, zone)
	hi, errHi := parseAny(to, zone)
	if errLo != nil || errHi != nil {
		return list
	}
	var out []any
	for _, it := range list {
		m, _ := it.(map[string]any)
		start, err := parseAny(str(m["start"]), zone)
		if err != nil {
			out = append(out, it)
			continue
		}
		end, err := parseAny(str(m["end"]), zone)
		if err != nil {
			end = start
		}
		if end.After(lo) && start.Before(hi) || start.Equal(lo) {
			out = append(out, it)
		}
	}
	if out == nil {
		return []any{}
	}
	return out
}

// parseAny reads times the way the calendar connector does: RFC 3339, or
// a local date and time (or date) in the scenario's zone.
func parseAny(s string, zone *time.Location) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	var err error
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		var t time.Time
		if t, err = time.ParseInLocation(layout, s, zone); err == nil {
			return t, nil
		}
	}
	return time.Time{}, err
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	}
	return 0
}

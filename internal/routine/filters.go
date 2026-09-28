package routine

import (
	"regexp"
	"strconv"
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
	return filterResponse(capability, args, v, now, true)
}

func filterResponse(capability string, args any, v any, now time.Time, strict bool) any {
	a, _ := args.(map[string]any)
	list, ok := v.([]any)
	if !ok || a == nil {
		return v
	}
	switch capability {
	case "gmail.search":
		return filterMail(list, a, now, strict)
	case "calendar.events":
		return filterEvents(list, a, now.Location())
	}
	return v
}

func filterMail(list []any, a map[string]any, now time.Time, strict bool) []any {
	q, _ := a["query"].(string)
	match := mailMatcher(q, now, strict)
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

// mailMatcher understands from: to: subject: is:unread/read, after:,
// before:, newer_than:, older_than:, category:, label:, in:, grouped
// values like from:(a OR b), -negation, OR, and bare words; unknown
// operators match everything. Dates, categories, labels and folders are
// applied only when strict: a routine's own tests answer its question
// directly and need not label every item.
func mailMatcher(q string, now time.Time, strict bool) func(map[string]any) bool {
	toks := fields(expandGroups(q))
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
		fn := mailTerm(strings.Trim(t, `"()`), now, strict)
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

var grouped = regexp.MustCompile(`(-?)(\w+):\(([^)]*)\)`)

// expandGroups rewrites key:(a OR b) as key:a OR key:b, and key:(a b) as
// key:a key:b, the way Gmail reads them.
func expandGroups(q string) string {
	return grouped.ReplaceAllStringFunc(q, func(g string) string {
		m := grouped.FindStringSubmatch(g)
		var parts []string
		for _, v := range strings.Fields(m[3]) {
			if strings.EqualFold(v, "OR") {
				parts = append(parts, "OR")
				continue
			}
			parts = append(parts, m[1]+m[2]+":"+v)
		}
		return strings.Join(parts, " ")
	})
}

// mailDate reads the dates Gmail takes in after: and before:: 2026/09/24,
// 2026-09-24, or seconds since 1970. Days start at midnight where the
// owner is.
func mailDate(v string, zone *time.Location) (time.Time, bool) {
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 100000 {
		return time.Unix(n, 0), true
	}
	for _, layout := range []string{"2006/01/02", "2006-01-02", "2006/1/2"} {
		if d, err := time.ParseInLocation(layout, v, zone); err == nil {
			return d, true
		}
	}
	return time.Time{}, false
}

// mailAge reads newer_than: and older_than: values: 2d, 3m, 1y.
func mailAge(v string, now time.Time) (time.Time, bool) {
	if len(v) < 2 {
		return time.Time{}, false
	}
	n, err := strconv.Atoi(v[:len(v)-1])
	if err != nil {
		return time.Time{}, false
	}
	switch strings.ToLower(v[len(v)-1:]) {
	case "d":
		return now.AddDate(0, 0, -n), true
	case "m":
		return now.AddDate(0, -n, 0), true
	case "y":
		return now.AddDate(-n, 0, 0), true
	}
	return time.Time{}, false
}

func hasLabel(m map[string]any, label string) bool {
	labels, _ := m["labels"].([]any)
	for _, l := range labels {
		if strings.EqualFold(str(l), label) {
			return true
		}
	}
	return false
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

func mailTerm(t string, now time.Time, strict bool) func(map[string]any) bool {
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
	if strings.EqualFold(val, "me") && (strings.EqualFold(key, "from") || strings.EqualFold(key, "to")) {
		// "me" is the owner: their own messages are not among what was
		// recorded, and everything recorded was sent to them.
		isTo := strings.EqualFold(key, "to")
		return func(map[string]any) bool { return isTo }
	}
	if !strict {
		switch strings.ToLower(key) {
		case "after", "before", "newer_than", "older_than", "category", "label", "in":
			return func(map[string]any) bool { return true }
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
	case "after", "before", "newer_than", "older_than":
		var at time.Time
		var ok bool
		if k := strings.ToLower(key); k == "after" || k == "before" {
			at, ok = mailDate(val, now.Location())
		} else {
			at, ok = mailAge(val, now)
		}
		if !ok {
			break
		}
		later := strings.EqualFold(key, "after") || strings.EqualFold(key, "newer_than")
		return func(m map[string]any) bool {
			d, err := time.Parse(time.RFC3339, str(m["date"]))
			if err != nil {
				return true
			}
			if later {
				return !d.Before(at)
			}
			return d.Before(at)
		}
	case "category":
		return func(m map[string]any) bool {
			if strings.EqualFold(val, "primary") {
				for _, c := range []string{"CATEGORY_PROMOTIONS", "CATEGORY_SOCIAL", "CATEGORY_UPDATES", "CATEGORY_FORUMS"} {
					if hasLabel(m, c) {
						return false
					}
				}
				return true
			}
			return hasLabel(m, "CATEGORY_"+val)
		}
	case "label", "in":
		// Items recorded without labels say nothing about where they are.
		return func(m map[string]any) bool {
			labels, _ := m["labels"].([]any)
			if len(labels) == 0 || strings.EqualFold(val, "anywhere") {
				return true
			}
			return hasLabel(m, val) || hasLabel(m, strings.ReplaceAll(val, "-", " "))
		}
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

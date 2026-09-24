package mail

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/emersion/go-imap/v2"
)

// Criteria translates the common subset of Gmail search syntax into IMAP
// SEARCH, so the same routine works with any IMAP provider:
// from: to: subject: is:unread is:read newer_than:3d older_than:1w
// after:2026/09/01 before:2026/10/01 "exact phrase" a OR b, -word, bare words.
func Criteria(query string, now time.Time) (*imap.SearchCriteria, error) {
	toks, err := tokenize(query)
	if err != nil {
		return nil, err
	}
	c := &imap.SearchCriteria{}
	for i := 0; i < len(toks); i++ {
		if i+2 < len(toks) && strings.EqualFold(toks[i+1], "OR") {
			a, err := term(toks[i], now)
			if err != nil {
				return nil, err
			}
			b, err := term(toks[i+2], now)
			if err != nil {
				return nil, err
			}
			or := [2]imap.SearchCriteria{*a, *b}
			i += 2
			// Chains like a OR b OR c nest to the right.
			for i+2 < len(toks) && strings.EqualFold(toks[i+1], "OR") {
				n, err := term(toks[i+2], now)
				if err != nil {
					return nil, err
				}
				or = [2]imap.SearchCriteria{{Or: [][2]imap.SearchCriteria{or}}, *n}
				i += 2
			}
			c.Or = append(c.Or, or)
			continue
		}
		t, err := term(toks[i], now)
		if err != nil {
			return nil, err
		}
		merge(c, t)
	}
	return c, nil
}

func merge(dst, src *imap.SearchCriteria) {
	dst.Header = append(dst.Header, src.Header...)
	dst.Text = append(dst.Text, src.Text...)
	dst.Flag = append(dst.Flag, src.Flag...)
	dst.NotFlag = append(dst.NotFlag, src.NotFlag...)
	dst.Not = append(dst.Not, src.Not...)
	dst.Or = append(dst.Or, src.Or...)
	if !src.Since.IsZero() && (dst.Since.IsZero() || src.Since.After(dst.Since)) {
		dst.Since = src.Since
	}
	if !src.Before.IsZero() && (dst.Before.IsZero() || src.Before.Before(dst.Before)) {
		dst.Before = src.Before
	}
}

func term(tok string, now time.Time) (*imap.SearchCriteria, error) {
	c := &imap.SearchCriteria{}
	if strings.HasPrefix(tok, "-") && len(tok) > 1 {
		inner, err := term(tok[1:], now)
		if err != nil {
			return nil, err
		}
		c.Not = []imap.SearchCriteria{*inner}
		return c, nil
	}
	key, val, hasKey := strings.Cut(tok, ":")
	if !hasKey || strings.HasPrefix(tok, `"`) {
		c.Text = []string{strings.Trim(tok, `"`)}
		return c, nil
	}
	val = strings.Trim(val, `"`)
	switch strings.ToLower(key) {
	case "from", "to", "cc", "subject":
		c.Header = []imap.SearchCriteriaHeaderField{{Key: map[string]string{"from": "From", "to": "To", "cc": "Cc", "subject": "Subject"}[strings.ToLower(key)], Value: val}}
	case "is":
		switch strings.ToLower(val) {
		case "unread":
			c.NotFlag = []imap.Flag{imap.FlagSeen}
		case "read":
			c.Flag = []imap.Flag{imap.FlagSeen}
		case "starred", "flagged":
			c.Flag = []imap.Flag{imap.FlagFlagged}
		case "answered", "replied":
			c.Flag = []imap.Flag{imap.FlagAnswered}
		}
	case "newer_than", "older_than":
		d, err := age(val)
		if err != nil {
			return nil, err
		}
		if strings.ToLower(key) == "newer_than" {
			c.Since = now.Add(-d)
		} else {
			c.Before = now.Add(-d)
		}
	case "after", "before":
		t, err := time.Parse("2006/01/02", strings.ReplaceAll(val, "-", "/"))
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a date like 2026/09/01", key, val)
		}
		if strings.ToLower(key) == "after" {
			c.Since = t
		} else {
			c.Before = t
		}
	case "in", "label", "category", "has":
		// Mailbox selection happens elsewhere; categories are Gmail-only.
	default:
		c.Text = []string{tok}
	}
	return c, nil
}

func age(v string) (time.Duration, error) {
	if len(v) < 2 {
		return 0, fmt.Errorf("bad age %q", v)
	}
	n, err := strconv.Atoi(v[:len(v)-1])
	if err != nil {
		return 0, fmt.Errorf("bad age %q", v)
	}
	unit := map[byte]time.Duration{'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour, 'm': 30 * 24 * time.Hour, 'y': 365 * 24 * time.Hour}[v[len(v)-1]]
	if unit == 0 {
		return 0, fmt.Errorf("bad age unit in %q", v)
	}
	return time.Duration(n) * unit, nil
}

func tokenize(q string) ([]string, error) {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range q {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case unicode.IsSpace(r) && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		case (r == '(' || r == ')') && !quoted:
			// Grouping is flattened; OR binds adjacent terms.
		default:
			cur.WriteRune(r)
		}
	}
	if quoted {
		return nil, fmt.Errorf("unclosed quote in %q", q)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

package secretscan

import (
	"strings"
	"testing"
)

func TestRedactsKnownFormats(t *testing.T) {
	for _, secret := range []string{
		"sk-ant-api03-" + strings.Repeat("aB3_", 10),
		"sk-proj-" + strings.Repeat("Xy9", 12),
		"ghp_" + strings.Repeat("a1B2", 9),
		"github_pat_" + strings.Repeat("11AB_cd", 8),
		"xoxb-1234567890-abcdefghij",
		"AKIAIOSFODNN7EXAMPLE",
		"AIza" + strings.Repeat("Sy_9", 9)[:35],
		"ntn_" + strings.Repeat("a1", 22),
		"123456789:AA" + strings.Repeat("h", 33),
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"https://hooks.slack.com/services/T000/B000/XXXXXXXX",
		"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----",
	} {
		got, found := Redact("here it is: " + secret + " thanks")
		if !found || strings.Contains(got, secret) || !strings.Contains(got, Mark) || !strings.HasSuffix(got, " thanks") {
			t.Errorf("%q → %q", secret, got)
		}
	}
}

func TestRedactsLabelledValues(t *testing.T) {
	for in, want := range map[string]string{
		"my password: hunter22!":                "my password: " + Mark,
		"senha=Tr0ub4dor&3":                     "senha=" + Mark,
		`api_key: "abc def 123"`:                "api_key: " + Mark,
		"the token is ignored, token Zx81kq9Pw": "the token is ignored, token " + Mark,
	} {
		if got, found := Redact(in); !found || got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

// Ordinary sentences about passwords and tokens, ids and hashes stay as
// they are.
func TestLeavesOrdinaryText(t *testing.T) {
	for _, s := range []string{
		"how do I reset my password?",
		"the token limit is too low",
		"commit 3f786850e387550fdab836ed7e6dc881de23001b broke the build",
		"call me at +55 11 91234-5678 tomorrow",
		"my order is 123456789",
		"use the secret recipe for dinner",
		"skip the desk-top-organizer idea",
		"https://example.com/docs?page=2",
	} {
		if got, found := Redact(s); found {
			t.Errorf("%q → %q", s, got)
		}
	}
}

func TestOnly(t *testing.T) {
	if !Only(Mark) || !Only("  "+Mark+" .") {
		t.Fatal("a lone key is only a key")
	}
	if Only("please save "+Mark+" for Notion") || Only("hi") {
		t.Fatal("a sentence is more than a key")
	}
}

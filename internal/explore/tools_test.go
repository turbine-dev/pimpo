package explore

import "testing"

// The scope the owner is asked about must be the host Go connects to.
func TestHostOfIsTheHostReached(t *testing.T) {
	for raw, want := range map[string]string{
		"https://allowed.com:x@attacker.tld/?d=SECRET": "",
		"https://allowed.com@attacker.tld/":            "",
		"https://Allowed.com:8443/feed":                "allowed.com",
		"//evil.tld/":                                  "",
	} {
		if got := hostOf(raw); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", raw, got, want)
		}
	}
}

// Package protect is the shared protection list: exfiltration domains,
// malicious skills and dangerous action patterns reported by people who
// run Vigia, reviewed, and published as a signed static file. Clients
// only download it; nothing about the person travels, and nothing needs a
// server.
package protect

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultURL is where the community list is published.
const DefaultURL = "https://raw.githubusercontent.com/denerFernandes/vigia-protection/main/list.json"

type Entry struct {
	ID string `json:"id"`
	// Kind is "domain", "skill" (SHA-256 of a SKILL.md) or "pattern".
	Kind  string `json:"kind"`
	Value string `json:"value"`
	// Capability limits a pattern to one capability; empty means any.
	Capability string `json:"capability,omitempty"`
	Reason     string `json:"reason"`
	// Reports is how many people reported it before review.
	Reports int    `json:"reports"`
	Added   string `json:"added"`
}

type List struct {
	Version int     `json:"version"`
	Updated string  `json:"updated"`
	Entries []Entry `json:"entries"`
	// Signature covers the canonical JSON of the list without it.
	Signature string `json:"signature,omitempty"`
}

func (l List) payload() []byte {
	l.Signature = ""
	b, _ := json.Marshal(l)
	return b
}

// Sign signs a list with a maintainer key (base64 Ed25519 private key).
func Sign(l List, private string) (List, error) {
	key, err := base64.StdEncoding.DecodeString(private)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return l, errors.New("not an Ed25519 private key")
	}
	for _, e := range l.Entries {
		if err := e.valid(); err != nil {
			return l, err
		}
	}
	l.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key), l.payload()))
	return l, nil
}

func (e Entry) valid() error {
	switch e.Kind {
	case "domain":
		if strings.ContainsAny(e.Value, "/:@ ") || !strings.Contains(e.Value, ".") {
			return fmt.Errorf("%s: a domain looks like example.com", e.ID)
		}
	case "skill":
		if len(e.Value) != 64 {
			return fmt.Errorf("%s: a skill is the SHA-256 of its SKILL.md", e.ID)
		}
	case "pattern":
		if _, err := regexp.Compile(e.Value); err != nil {
			return fmt.Errorf("%s: %w", e.ID, err)
		}
	default:
		return fmt.Errorf("%s: unknown kind %q", e.ID, e.Kind)
	}
	if e.ID == "" || e.Reason == "" {
		return errors.New("every entry needs an id and a reason")
	}
	return nil
}

// Verify checks the signature against any of the trusted keys.
func (l List) Verify(keys []string) error {
	sig, err := base64.StdEncoding.DecodeString(l.Signature)
	if err != nil || l.Signature == "" {
		return errors.New("the protection list is not signed")
	}
	for _, k := range keys {
		pub, err := base64.StdEncoding.DecodeString(k)
		if err == nil && len(pub) == ed25519.PublicKeySize && ed25519.Verify(ed25519.PublicKey(pub), l.payload(), sig) {
			return nil
		}
	}
	return errors.New("the protection list signature does not match a trusted key")
}

// Guard holds the current list and answers whether something is flagged.
type Guard struct {
	// Keys are the maintainers' public keys the list must be signed with.
	Keys []string
	URL  string
	// Starter is the list shipped with this version, used until a newer
	// one is downloaded.
	Starter []byte

	mu       sync.RWMutex
	list     List
	patterns map[string]*regexp.Regexp
	fetched  time.Time
}

func (g *Guard) set(l List) {
	pats := map[string]*regexp.Regexp{}
	for _, e := range l.Entries {
		if e.Kind == "pattern" {
			pats[e.ID] = regexp.MustCompile("(?i)" + e.Value)
		}
	}
	g.mu.Lock()
	g.list, g.patterns = l, pats
	g.mu.Unlock()
}

// Load installs a list after checking its signature; older versions are
// refused so a stale copy cannot replace a newer one.
func (g *Guard) Load(raw []byte) error {
	var l List
	if err := json.Unmarshal(raw, &l); err != nil {
		return fmt.Errorf("protection list: %w", err)
	}
	if err := l.Verify(g.Keys); err != nil {
		return err
	}
	for _, e := range l.Entries {
		if err := e.valid(); err != nil {
			return err
		}
	}
	g.mu.RLock()
	current := g.list.Version
	g.mu.RUnlock()
	if l.Version < current {
		return fmt.Errorf("protection list version %d is older than %d", l.Version, current)
	}
	g.set(l)
	return nil
}

// Init loads the starter list shipped with the binary.
func (g *Guard) Init() error {
	if len(g.Starter) == 0 {
		return nil
	}
	return g.Load(g.Starter)
}

// Refresh downloads the published list.
func (g *Guard) Refresh(ctx context.Context) error {
	src := g.URL
	if src == "" {
		src = DefaultURL
	}
	var raw []byte
	var err error
	if strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "http://") {
		req, rerr := http.NewRequestWithContext(ctx, "GET", src, nil)
		if rerr != nil {
			return rerr
		}
		resp, derr := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if derr != nil {
			return derr
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("protection list: %s", resp.Status)
		}
		raw, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	} else {
		raw, err = os.ReadFile(src)
	}
	if err != nil {
		return err
	}
	if err := g.Load(raw); err != nil {
		return err
	}
	g.mu.Lock()
	g.fetched = time.Now()
	g.mu.Unlock()
	return nil
}

// Status summarizes the list in use.
func (g *Guard) Status() (version, entries int, updated string, fetched time.Time) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.list.Version, len(g.list.Entries), g.list.Updated, g.fetched
}

var urlRe = regexp.MustCompile(`(?i)\b(?:https?://)?([a-z0-9-]+(?:\.[a-z0-9-]+)+)(?:[:/][^\s"']*)?`)
var emailRe = regexp.MustCompile(`(?i)[a-z0-9._%+-]+@([a-z0-9.-]+\.[a-z]{2,})`)

// hosts finds the domains an action reaches: its scope, URLs and email
// addresses in its arguments.
func hosts(scope string, args any) []string {
	b, _ := json.Marshal(args)
	text := string(b)
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		h = strings.ToLower(strings.TrimSuffix(h, "."))
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	if scope != "" {
		add(scope)
	}
	for _, m := range emailRe.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	for _, m := range urlRe.FindAllStringSubmatch(text, -1) {
		if u, err := url.Parse("https://" + m[1]); err == nil {
			add(u.Hostname())
		}
	}
	sort.Strings(out)
	return out
}

func domainMatch(host, domain string) bool {
	domain = strings.ToLower(domain)
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// Check returns the entry an action runs into, if any.
func (g *Guard) Check(capability, scope string, args any) (Entry, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if len(g.list.Entries) == 0 {
		return Entry{}, false
	}
	hs := hosts(scope, args)
	b, _ := json.Marshal(args)
	for _, e := range g.list.Entries {
		switch e.Kind {
		case "domain":
			for _, h := range hs {
				if domainMatch(h, e.Value) {
					return e, true
				}
			}
		case "pattern":
			if (e.Capability == "" || e.Capability == capability) && g.patterns[e.ID].Match(b) {
				return e, true
			}
		}
	}
	return Entry{}, false
}

// Skill reports whether a skill's text is on the list.
func (g *Guard) Skill(text []byte) (Entry, bool) {
	sum := sha256.Sum256(text)
	h := hex.EncodeToString(sum[:])
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, e := range g.list.Entries {
		if e.Kind == "skill" && e.Value == h {
			return e, true
		}
	}
	return Entry{}, false
}

// Suggestion is what someone proposes for the list: only the indicator and
// why, never the message, email or page it came from.
func Suggestion(kind, value, reason string) (Entry, error) {
	e := Entry{ID: kind + "-" + shortHash(value), Kind: kind, Value: strings.TrimSpace(strings.ToLower(value)), Reason: strings.TrimSpace(reason), Reports: 1, Added: time.Now().UTC().Format("2006-01-02")}
	if kind == "pattern" {
		e.Value = strings.TrimSpace(value)
	}
	return e, e.valid()
}

func shortHash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:4]) }

// Package gallery shares routines safely. An entry is a routine with its
// code, manifest and tests, identified by the hash of that content and
// signed by its author's Ed25519 key. The index is a static file in a
// public git repository; nothing here needs a server.
//
// The index's list of authors is signed by a gallery root key built into
// Pimpo, so whoever serves the index cannot add an author or change one's
// key. Installing then checks, in order: the author is listed, the
// signature covers exactly this content, the entry is not revoked, its
// tests pass, and an audit run shows it calls only what its manifest
// declares.
package gallery

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/routine"
)

// DefaultIndex is the community index.
const DefaultIndex = "https://raw.githubusercontent.com/denerFernandes/pimpo-gallery/main/index.json"

type Entry struct {
	ID      string          `json:"id"`
	Author  string          `json:"author"`
	Routine routine.Routine `json:"routine"`
	// Hash is the SHA-256 of the routine's canonical JSON: the same code,
	// manifest and tests always give the same hash.
	Hash      string    `json:"hash"`
	Signature string    `json:"signature"`
	Published time.Time `json:"published"`
}

type Author struct {
	Name string `json:"name"`
	// Key is the base64 Ed25519 public key.
	Key string `json:"key"`
	URL string `json:"url,omitempty"`
}

// RootKeys are the gallery maintainers' public keys (base64 Ed25519); an
// index's authors must be signed with one of them. More than one allows a
// new key to ship before the old one retires.
var RootKeys = []string{"ccseMghOpOnxNHoAbJ/Er7gbD9dT7543muZgchevaQY="}

type Index struct {
	Authors map[string]Author `json:"authors"`
	// AuthorsSignature is a root key's signature of the authors list
	// (authorsSigned).
	AuthorsSignature string  `json:"authors_signature,omitempty"`
	Entries          []Entry `json:"entries"`
	// Revoked lists hashes removed after a confirmed report.
	Revoked []string `json:"revoked,omitempty"`
}

// Hash is the content address of a routine.
func Hash(r routine.Routine) string {
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Keygen makes an author key pair, base64 encoded.
func Keygen() (public, private string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
}

// Sign prepares an entry for the index.
func Sign(id, author string, r routine.Routine, private string) (Entry, error) {
	key, err := base64.StdEncoding.DecodeString(private)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return Entry{}, errors.New("not an Ed25519 private key")
	}
	e := Entry{ID: id, Author: author, Routine: r, Hash: Hash(r), Published: time.Now().UTC().Truncate(time.Second)}
	e.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key), []byte(e.signed())))
	return e, nil
}

// signed is what the signature covers: which entry, by whom, and exactly
// which content.
func (e Entry) signed() string { return signedAs("pimpo-gallery-v1", e) }

// legacySigned is what entries signed before the rename covered.
func (e Entry) legacySigned() string { return signedAs("zodim-gallery-v1", e) }

func signedAs(label string, e Entry) string {
	return label + "\n" + e.ID + "\n" + e.Author + "\n" + e.Hash
}

// authorsSigned is what the root signature covers: every author's id,
// name, key and address, in id order.
func authorsSigned(authors map[string]Author) []byte {
	ids := make([]string, 0, len(authors))
	for id := range authors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString("pimpo-gallery-authors-v1\n")
	for _, id := range ids {
		a := authors[id]
		line, _ := json.Marshal([]string{id, a.Name, a.Key, a.URL})
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// SignAuthors signs the index's authors with a root private key.
func SignAuthors(ix Index, private string) (Index, error) {
	key, err := base64.StdEncoding.DecodeString(private)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return ix, errors.New("not an Ed25519 private key")
	}
	ix.AuthorsSignature = base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key), authorsSigned(ix.Authors)))
	return ix, nil
}

// CheckAuthors returns an error unless the authors are signed with one of
// keys.
func (ix Index) CheckAuthors(keys []string) error {
	if ix.AuthorsSignature == "" {
		return errors.New("the gallery's authors are not signed by a gallery maintainer")
	}
	sig, err := base64.StdEncoding.DecodeString(ix.AuthorsSignature)
	if err == nil && len(sig) == ed25519.SignatureSize {
		payload := authorsSigned(ix.Authors)
		for _, k := range keys {
			pub, err := base64.StdEncoding.DecodeString(k)
			if err == nil && len(pub) == ed25519.PublicKeySize && ed25519.Verify(ed25519.PublicKey(pub), payload, sig) {
				return nil
			}
		}
	}
	return errors.New("the gallery's authors are not signed by a gallery root key; the index may have been changed")
}

// Report is what verification found out about one entry.
type Report struct {
	Verified bool     `json:"verified"`
	Problems []string `json:"problems,omitempty"`
	// Uses is what the audit saw the routine call.
	Uses []string `json:"uses"`
	// Sends is true when the routine can reach anyone but the owner: it
	// sends to other people, or it reads the owner's data and can reach
	// hosts outside (Outside), where that data could go. Installing such a
	// routine takes the owner's confirmation.
	Sends   bool     `json:"sends"`
	Outside []string `json:"outside,omitempty"`
	// Risk is the highest risk among the declared capabilities.
	Risk string `json:"risk"`
}

// Verify checks an entry against the index it came from.
func (ix Index) Verify(ctx context.Context, e Entry) Report {
	r := Report{Uses: []string{}}
	fail := func(format string, a ...any) { r.Problems = append(r.Problems, fmt.Sprintf(format, a...)) }
	if got := Hash(e.Routine); got != e.Hash {
		fail("the content does not match its hash")
	}
	for _, h := range ix.Revoked {
		if h == e.Hash {
			fail("removed from the gallery after a confirmed report")
		}
	}
	author, ok := ix.Authors[e.Author]
	if !ok {
		fail("unknown author %q", e.Author)
	} else if key, err := base64.StdEncoding.DecodeString(author.Key); err != nil || len(key) != ed25519.PublicKeySize {
		fail("the author's key is malformed")
	} else if sig, err := base64.StdEncoding.DecodeString(e.Signature); err != nil || !(ed25519.Verify(ed25519.PublicKey(key), []byte(e.signed()), sig) || ed25519.Verify(ed25519.PublicKey(key), []byte(e.legacySigned()), sig)) {
		fail("the signature does not match")
	}
	if len(e.Routine.Manifest.Uses) > 0 {
		// Another owner's routine ids mean nothing here.
		fail("it runs other routines (%s), which a gallery routine cannot", strings.Join(e.Routine.Manifest.Uses, ", "))
	}
	for _, t := range e.Routine.Tests {
		if out := routine.Check(ctx, e.Routine, t.Name, t.Scenario); !out.Passed {
			fail("test %q fails: %s", t.Name, strings.Join(out.Problems, "; "))
		}
	}
	uses, problems := routine.Audit(ctx, e.Routine)
	r.Uses = append(r.Uses, uses...)
	r.Problems = append(r.Problems, problems...)
	top := capability.Read
	for _, entry := range e.Routine.Manifest.Capabilities {
		spec, _, err := capability.Parse(entry)
		if err != nil {
			continue
		}
		if spec.Risk > top {
			top = spec.Risk
		}
	}
	r.Sends, r.Outside = Sends(e.Routine)
	r.Risk = top.String()
	sort.Strings(r.Problems)
	r.Verified = len(r.Problems) == 0
	return r
}

// publicReads read nothing of the owner's.
var publicReads = map[string]bool{"web.search": true, "rss.read": true, "code.run": true}

// Sends reports whether a routine can reach anyone but the owner, and the
// outside hosts it can reach: it sends to other people, or it reads the
// owner's data (email, calendar, notes...) and can reach a host outside.
func Sends(r routine.Routine) (bool, []string) {
	sends, private := false, false
	var outside []string
	for _, entry := range r.Manifest.Capabilities {
		spec, scope, err := capability.Parse(entry)
		if err != nil {
			continue
		}
		switch {
		case spec.Risk == capability.Irreversible && strings.Contains(spec.Name, "send"):
			sends = true
		case spec.Scoped:
			outside = append(outside, scope)
		case spec.Risk == capability.Read && !publicReads[spec.Name]:
			private = true
		}
	}
	sort.Strings(outside)
	return sends || (private && len(outside) > 0), outside
}

// Load reads an index from an https URL or a local file. Plain http is
// refused, except to this machine (tests, a local mirror): anyone on the
// way could change the entries and the authors' keys that come with them.
func Load(ctx context.Context, src string) (Index, error) {
	var raw []byte
	var err error
	if strings.HasPrefix(src, "http://") {
		u, perr := url.Parse(src)
		if perr != nil || !loopback(u.Hostname()) {
			return Index{}, errors.New("the gallery address must start with https://")
		}
	}
	if strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "http://") {
		req, rerr := http.NewRequestWithContext(ctx, "GET", src, nil)
		if rerr != nil {
			return Index{}, rerr
		}
		resp, derr := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if derr != nil {
			return Index{}, derr
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return Index{}, fmt.Errorf("gallery index: %s", resp.Status)
		}
		raw, err = io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	} else {
		raw, err = os.ReadFile(src)
	}
	if err != nil {
		return Index{}, err
	}
	return Parse(raw)
}

// Parse reads an index and refuses it unless its authors are signed by a
// root key.
func Parse(raw []byte) (Index, error) {
	var ix Index
	if err := json.Unmarshal(raw, &ix); err != nil {
		return Index{}, fmt.Errorf("gallery index: %w", err)
	}
	if err := ix.CheckAuthors(RootKeys); err != nil {
		return Index{}, err
	}
	return ix, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Find returns an entry by id.
func (ix Index) Find(id string) (Entry, bool) {
	for _, e := range ix.Entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// PublicKey derives the public key from a base64 private key.
func PublicKey(private string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(private)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return "", errors.New("not an Ed25519 private key")
	}
	return base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey)), nil
}

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	for _, c := range [][3]any{
		{"v0.7.0", "v0.6.0", true}, {"v0.6.0", "v0.6.0-beta.2", true}, {"v0.6.0-beta.2", "v0.6.0-beta.1", true},
		{"v0.6.0-beta.10", "v0.6.0-beta.9", true}, {"v0.6.0-rc.1", "v0.6.0-beta.3", true}, {"v0.6.0", "vdev", true},
		{"v0.6.0", "v0.6.0", false}, {"v0.5.9", "v0.6.0", false}, {"v1.0.0", "v0.99.0", true},
	} {
		if got := newerVersion(c[0].(string), c[1].(string)); got != c[2].(bool) {
			t.Errorf("%s newer than %s: %v", c[0], c[1], got)
		}
	}
}

func TestUpdateAndRollBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the release in this test is a tar.gz")
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho new\n")
	tw.WriteHeader(&tar.Header{Name: "pimpo", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write(body)
	tw.Close()
	gz.Close()
	sum := sha256.Sum256(archive.Bytes())
	good := hex.EncodeToString(sum[:])
	sums := good
	// A throwaway release key stands in for the real one.
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	oldKeys := releaseKeys
	releaseKeys = []string{base64.StdEncoding.EncodeToString(pub)}
	defer func() { releaseKeys = oldKeys }()
	sign := func(key ed25519.PrivateKey) string {
		s, _ := signBytes(base64.StdEncoding.EncodeToString(key), []byte(fmt.Sprintf("%s  %s\n", sums, archiveName())))
		return s
	}
	sig := sign(priv)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			fmt.Fprint(w, `[{"tag_name":"v9.0.0-beta.1","prerelease":true},{"tag_name":"v9.0.0"},{"tag_name":"v8.0.0"},{"tag_name":"v10.0.0","draft":true}]`)
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			fmt.Fprintf(w, "%s  %s\n", sums, archiveName())
		case strings.HasSuffix(r.URL.Path, "/checksums.txt.sig"):
			if sig == "" {
				w.WriteHeader(404)
				return
			}
			fmt.Fprintln(w, sig)
		case strings.HasSuffix(r.URL.Path, "/"+archiveName()):
			w.Write(archive.Bytes())
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	oldAPI, oldBase := releasesAPI, releasesBase
	releasesAPI, releasesBase = srv.URL+"/releases", srv.URL+"/download"
	defer func() { releasesAPI, releasesBase = oldAPI, oldBase }()

	if tag, err := latestTag(t.Context(), false); err != nil || tag != "v9.0.0" {
		t.Fatalf("latest %q %v", tag, err)
	}
	exe := filepath.Join(t.TempDir(), "pimpo")
	os.WriteFile(exe, []byte("old"), 0o755)
	bin, err := fetchRelease(t.Context(), "v9.0.0")
	if err != nil || !bytes.Equal(bin, body) {
		t.Fatalf("%q %v", bin, err)
	}
	if err := swapIn(exe, bin); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); !bytes.Equal(b, body) {
		t.Fatal("not replaced")
	}
	if b, _ := os.ReadFile(exe + ".previous"); string(b) != "old" {
		t.Fatal("old one not kept")
	}
	var out bytes.Buffer
	if err := rollBack(exe, &out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatal("not rolled back")
	}
	sums = strings.Repeat("0", 64)
	sig = sign(priv)
	if _, err := fetchRelease(t.Context(), "v9.0.0"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a bad checksum was accepted: %v", err)
	}
}

// Whoever can change the files of a release can change checksums.txt too,
// so it is trusted only with a signature by a release key.
func TestUpdateNeedsSignedChecksums(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	body := []byte("evil")
	tw.WriteHeader(&tar.Header{Name: "pimpo", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write(body)
	tw.Close()
	gz.Close()
	sum := sha256.Sum256(archive.Bytes())
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + archiveName() + "\n")
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	oldKeys := releaseKeys
	releaseKeys = []string{"not a key", base64.StdEncoding.EncodeToString(pub)}
	defer func() { releaseKeys = oldKeys }()
	sig := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			w.Write(sums)
		case strings.HasSuffix(r.URL.Path, "/checksums.txt.sig") && sig != "":
			fmt.Fprint(w, sig)
		case strings.HasSuffix(r.URL.Path, "/"+archiveName()):
			w.Write(archive.Bytes())
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	oldBase := releasesBase
	releasesBase = srv.URL + "/download"
	defer func() { releasesBase = oldBase }()

	if _, err := fetchRelease(t.Context(), "v9.0.0"); err == nil || !strings.Contains(err.Error(), "no signature") {
		t.Fatalf("installed without a signature: %v", err)
	}
	sig, _ = signBytes(base64.StdEncoding.EncodeToString(other), sums)
	if _, err := fetchRelease(t.Context(), "v9.0.0"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("installed with another key's signature: %v", err)
	}
	sig = "garbage"
	if _, err := fetchRelease(t.Context(), "v9.0.0"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("installed with a malformed signature: %v", err)
	}
}

func TestReleaseSign(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	file := filepath.Join(dir, "checksums.txt")
	os.WriteFile(file, []byte("abc  pimpo_linux_amd64.tar.gz\n"), 0o644)
	keyFile := filepath.Join(dir, "release.key")
	os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600)
	keys := []string{base64.StdEncoding.EncodeToString(pub)}
	var out bytes.Buffer
	if err := releaseCmd([]string{"sign", "--key", keyFile, file}, &out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	if sig, _ := os.ReadFile(file + ".sig"); !signedByOneOf(keys, data, string(sig)) {
		t.Fatal("the signature does not verify")
	}
	t.Setenv("TEST_RELEASE_KEY", base64.StdEncoding.EncodeToString(priv))
	if err := releaseCmd([]string{"sign", "--key-env", "TEST_RELEASE_KEY", "--out", filepath.Join(dir, "x.sig"), file}, &out); err != nil {
		t.Fatal(err)
	}
	if sig, _ := os.ReadFile(filepath.Join(dir, "x.sig")); !signedByOneOf(keys, data, string(sig)) {
		t.Fatal("the signature from the environment does not verify")
	}
	if strings.Contains(out.String(), base64.StdEncoding.EncodeToString(priv)) {
		t.Fatal("printed the private key")
	}
	t.Setenv("TEST_RELEASE_KEY", "")
	if err := releaseCmd([]string{"sign", "--key-env", "TEST_RELEASE_KEY", file}, &out); err == nil {
		t.Fatal("signed with an empty key")
	}
	if err := releaseCmd([]string{"sign", "--key", keyFile, "--key-env", "X", file}, &out); err == nil {
		t.Fatal("took two keys")
	}
}

// The key built into the binary is the one the release workflow signs with.
func TestReleaseKeyIsBuiltIn(t *testing.T) {
	if len(releaseKeys) == 0 {
		t.Fatal("no release key")
	}
	for _, k := range releaseKeys {
		if b, err := base64.StdEncoding.DecodeString(k); err != nil || len(b) != ed25519.PublicKeySize {
			t.Fatalf("malformed release key %q", k)
		}
	}
}

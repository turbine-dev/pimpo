package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			fmt.Fprint(w, `[{"tag_name":"v9.0.0-beta.1","prerelease":true},{"tag_name":"v9.0.0"},{"tag_name":"v8.0.0"},{"tag_name":"v10.0.0","draft":true}]`)
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			fmt.Fprintf(w, "%s  %s\n", sums, archiveName())
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
	if _, err := fetchRelease(t.Context(), "v9.0.0"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a bad checksum was accepted: %v", err)
	}
}

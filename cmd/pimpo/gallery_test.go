package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/gallery"
)

// A new author waits for a maintainer: build leaves the authors unsigned
// and verify refuses the index until sign-authors, with a root key, signs
// them.
func TestGallerySignAuthors(t *testing.T) {
	rootPub, rootPriv, _ := gallery.Keygen()
	old := gallery.RootKeys
	gallery.RootKeys = []string{rootPub}
	defer func() { gallery.RootKeys = old }()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "routines"), 0o755)
	b, _ := os.ReadFile("../../gallery/routines/dolar-hoje.json")
	os.WriteFile(filepath.Join(dir, "routines", "dolar-hoje.json"), b, 0o644)
	keys := t.TempDir()
	var out strings.Builder
	if err := galleryCmd([]string{"keygen", "--out", filepath.Join(keys, "author.key")}, &out); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(keys, "root.key"), []byte(rootPriv+"\n"), 0o600)
	_, otherPriv, _ := gallery.Keygen()
	os.WriteFile(filepath.Join(keys, "other.key"), []byte(otherPriv+"\n"), 0o600)
	index := filepath.Join(dir, "index.json")

	if err := galleryCmd([]string{"build", "--key", filepath.Join(keys, "author.key"), "--author", "ana", "--name", "Ana", dir}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "sign-authors") {
		t.Fatalf("build did not say the authors wait for a maintainer:\n%s", out.String())
	}
	if err := galleryVerify(index, &out); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Fatalf("an unsigned index verified: %v", err)
	}
	if err := galleryCmd([]string{"sign-authors", "--key", filepath.Join(keys, "other.key"), dir}, &out); err == nil {
		t.Fatal("signed with a key that is not a root key")
	}
	out.Reset()
	if err := galleryCmd([]string{"sign-authors", "--key", filepath.Join(keys, "root.key"), dir}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), rootPriv) {
		t.Fatal("printed the root private key")
	}
	if err := galleryVerify(index, &out); err != nil {
		t.Fatal(err)
	}
	// Signing again an unchanged routine keeps the authors signed.
	out.Reset()
	if err := galleryCmd([]string{"build", "--key", filepath.Join(keys, "author.key"), "--author", "ana", dir}, &out); err != nil || strings.Contains(out.String(), "sign-authors") {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

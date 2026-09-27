package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denerFernandes/pimpo/internal/routine"
	"github.com/denerFernandes/pimpo/internal/runtime"
)

// A routine folder cannot make Pimpo read another file of the owner's
// through a link, nor load something huge.
func TestLinksAreNotFollowed(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "id_rsa")
	os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600)
	ok := routine.Routine{Name: "Ok", Code: "async function run(){}", Manifest: runtime.Manifest{Schedule: "0 7 * * *", Capabilities: []string{"telegram.send"}}}
	Write(root, "ok", ok)
	Write(root, "linked", ok)
	os.Remove(filepath.Join(root, Dir, "linked", "routine.js"))
	os.Symlink(secret, filepath.Join(root, Dir, "linked", "routine.js"))
	os.Symlink(filepath.Join(root, Dir, "ok"), filepath.Join(root, Dir, "alias"))
	Write(root, "huge", ok)
	os.WriteFile(filepath.Join(root, Dir, "huge", "routine.js"), []byte(strings.Repeat("x", 2<<20)), 0o644)

	found, broken := Read(root)
	if _, in := found["ok"]; !in || len(found) != 1 {
		t.Fatalf("found %v", found)
	}
	for _, id := range []string{"linked", "alias", "huge"} {
		if broken[id] == "" {
			t.Errorf("%s was read: %v", id, broken)
		}
	}
	for _, r := range found {
		if strings.Contains(r.Code, "PRIVATE") {
			t.Fatal("followed a link to a private file")
		}
	}
}

// Package repo keeps routines as files in a folder, usually a git
// repository the owner controls: one folder per routine with its code,
// manifest and tests, so routines can be reviewed, versioned and shared
// like any other code. Reading a folder never installs anything; the app
// checks each routine and the owner approves it.
package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/routine"
	"github.com/turbine-dev/pimpo/internal/runtime"
)

// Dir is where routines live inside the repository.
const Dir = "routines"

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// meta is routine.json: everything but the code and the tests.
type meta struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Manifest    runtime.Manifest `json:"manifest"`
}

// Write puts one routine in root/routines/<id>/.
func Write(root, id string, r routine.Routine) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("%q is not a routine id", id)
	}
	dir := filepath.Join(root, Dir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	m, _ := json.MarshalIndent(meta{r.Name, r.Description, r.Manifest}, "", "  ")
	tests := r.Tests
	if tests == nil {
		tests = []routine.Test{}
	}
	t, _ := json.MarshalIndent(tests, "", "  ")
	code := strings.TrimRight(r.Code, "\n") + "\n"
	for name, data := range map[string][]byte{"routine.json": append(m, '\n'), "tests.json": append(t, '\n'), "routine.js": []byte(code)} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Read loads every routine folder; a folder that cannot be read is
// reported by id instead of stopping the rest.
func Read(root string) (map[string]routine.Routine, map[string]string) {
	found, broken := map[string]routine.Routine{}, map[string]string{}
	entries, err := os.ReadDir(filepath.Join(root, Dir))
	if err != nil {
		return found, broken
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			broken[e.Name()] = "links are not followed"
			continue
		}
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		id := e.Name()
		if !validID.MatchString(id) {
			broken[id] = "folder names are lowercase letters, digits and dashes"
			continue
		}
		r, err := readOne(filepath.Join(root, Dir, id))
		if err != nil {
			broken[id] = err.Error()
			continue
		}
		found[id] = r
	}
	return found, broken
}

// readFile reads one file of a routine folder: a regular file, not a link
// (which could point at any file of the owner's), and of a sane size.
func readFile(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	if st.Size() > 1<<20 {
		return nil, fmt.Errorf("%s is larger than 1 MB", filepath.Base(path))
	}
	return os.ReadFile(path)
}

func readOne(dir string) (routine.Routine, error) {
	var r routine.Routine
	if st, err := os.Lstat(dir); err != nil || !st.IsDir() {
		return r, errors.New("not a folder")
	}
	raw, err := readFile(filepath.Join(dir, "routine.json"))
	if err != nil {
		return r, errors.New("routine.json is missing")
	}
	var m meta
	if err := json.Unmarshal(raw, &m); err != nil {
		return r, fmt.Errorf("routine.json: %v", err)
	}
	code, err := readFile(filepath.Join(dir, "routine.js"))
	if err != nil {
		return r, errors.New("routine.js is missing")
	}
	r = routine.Routine{Name: m.Name, Description: m.Description, Manifest: m.Manifest, Code: string(code)}
	if raw, err := readFile(filepath.Join(dir, "tests.json")); err == nil {
		if err := json.Unmarshal(raw, &r.Tests); err != nil {
			return r, fmt.Errorf("tests.json: %v", err)
		}
	}
	if strings.TrimSpace(r.Name) == "" {
		return r, errors.New("routine.json needs a name")
	}
	return r, nil
}

// Hash identifies a routine's content, to tell changed from unchanged.
func Hash(r routine.Routine) string {
	r.Code = strings.TrimRight(r.Code, "\n")
	if r.Tests == nil {
		r.Tests = []routine.Test{}
	}
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// Git runs a git command in root and returns its output.
func Git(ctx context.Context, root string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, errors.New(text)
	}
	return text, nil
}

// IsGit reports whether root is inside a git work tree.
func IsGit(ctx context.Context, root string) bool {
	out, err := Git(ctx, root, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// HasRemote reports whether the repository has somewhere to pull from.
func HasRemote(ctx context.Context, root string) bool {
	out, err := Git(ctx, root, "remote")
	return err == nil && strings.TrimSpace(out) != ""
}

// Commit records the routines folder, if anything changed, and returns the
// short commit id ("" when there was nothing to commit).
func Commit(ctx context.Context, root, message string) (string, error) {
	if _, err := Git(ctx, root, "add", "--", Dir); err != nil {
		return "", err
	}
	if _, err := Git(ctx, root, "diff", "--cached", "--quiet", "--", Dir); err == nil {
		return "", nil
	}
	// The owner's git identity signs the commit; without one, Pimpo's.
	if _, err := Git(ctx, root, "commit", "-m", message, "--", Dir); err != nil {
		if !strings.Contains(err.Error(), "tell me who you are") && !strings.Contains(err.Error(), "user.email") {
			return "", err
		}
		if _, err := Git(ctx, root, "-c", "user.name=Pimpo", "-c", "user.email=pimpo@localhost", "commit", "-m", message, "--", Dir); err != nil {
			return "", err
		}
	}
	return Git(ctx, root, "rev-parse", "--short", "HEAD")
}

// Head is the current short commit, or "".
func Head(ctx context.Context, root string) string {
	out, err := Git(ctx, root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// Sorted returns the ids of a map in order.
func Sorted[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

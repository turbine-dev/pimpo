// Package snapshot copies Pimpo's state (database and memory) aside before
// risky moments, such as an update, and restores it in one command.
package snapshot

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Snapshot struct {
	Name  string    `json:"name"`
	Label string    `json:"label"`
	When  time.Time `json:"when"`
	Bytes int64     `json:"bytes"`
}

// Keep is how many snapshots stay on disk.
const Keep = 10

func dir(home string) string { return filepath.Join(home, "snapshots") }

// Create writes a consistent copy of the database (VACUUM INTO works while
// Pimpo runs) and a copy of the memory folder.
func Create(db *sql.DB, home, label string) (Snapshot, error) {
	when := time.Now()
	name := when.UTC().Format("20060102-150405") + "-" + clean(label)
	target := filepath.Join(dir(home), name)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return Snapshot{}, err
	}
	if _, err := db.Exec(`VACUUM INTO ?`, filepath.Join(target, "pimpo.db")); err != nil {
		os.RemoveAll(target)
		return Snapshot{}, fmt.Errorf("copy database: %w", err)
	}
	if err := copyDir(filepath.Join(home, "memory"), filepath.Join(target, "memory")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		os.RemoveAll(target)
		return Snapshot{}, fmt.Errorf("copy memory: %w", err)
	}
	prune(home)
	return info(home, name)
}

func clean(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	var b strings.Builder
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "manual"
	}
	return strings.Trim(b.String(), "-")
}

func info(home, name string) (Snapshot, error) {
	st, err := os.Stat(filepath.Join(dir(home), name, "pimpo.db"))
	if err != nil {
		return Snapshot{}, err
	}
	parts := strings.SplitN(name, "-", 3)
	when, _ := time.Parse("20060102-150405", parts[0]+"-"+parts[1])
	label := ""
	if len(parts) == 3 {
		label = parts[2]
	}
	return Snapshot{Name: name, Label: label, When: when, Bytes: st.Size()}, nil
}

func List(home string) ([]Snapshot, error) {
	entries, err := os.ReadDir(dir(home))
	if errors.Is(err, fs.ErrNotExist) {
		return []Snapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Snapshot{}
	for _, e := range entries {
		if s, err := info(home, e.Name()); err == nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

func prune(home string) {
	all, _ := List(home)
	for i, s := range all {
		if i >= Keep {
			os.RemoveAll(filepath.Join(dir(home), s.Name))
		}
	}
}

// Restore puts a snapshot back. Pimpo must not be running. The current
// state is snapshotted first, so a restore can itself be undone.
func Restore(home, name string, current *sql.DB) error {
	src := filepath.Join(dir(home), name)
	if _, err := os.Stat(filepath.Join(src, "pimpo.db")); err != nil {
		return fmt.Errorf("snapshot %s not found", name)
	}
	if current != nil {
		if _, err := Create(current, home, "before-restore"); err != nil {
			return err
		}
		current.Close()
	}
	for _, f := range []string{"pimpo.db", "pimpo.db-wal", "pimpo.db-shm"} {
		os.Remove(filepath.Join(home, f))
	}
	if err := copyFile(filepath.Join(src, "pimpo.db"), filepath.Join(home, "pimpo.db")); err != nil {
		return err
	}
	os.RemoveAll(filepath.Join(home, "memory"))
	if err := copyDir(filepath.Join(src, "memory"), filepath.Join(home, "memory")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		return copyFile(p, target)
	})
}

const stagedFile = "restore-pending"

// Stage marks a snapshot to be restored on the next start, since the
// database cannot be swapped while Pimpo has it open.
func Stage(home, name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("snapshot %s not found", name)
	}
	if _, err := info(home, name); err != nil {
		return fmt.Errorf("snapshot %s not found", name)
	}
	return os.WriteFile(filepath.Join(home, stagedFile), []byte(name), 0o600)
}

// Staged returns the snapshot waiting to be restored, if any.
func Staged(home string) string {
	b, _ := os.ReadFile(filepath.Join(home, stagedFile))
	return strings.TrimSpace(string(b))
}

func Unstage(home string) { os.Remove(filepath.Join(home, stagedFile)) }

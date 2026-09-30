package snapshot

import (
	"archive/zip"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// A damaged database is never repaired in place or written over: it is
// moved, with its journal, into quarantine/<time>/ and Pimpo starts in
// recovery until the owner chooses what to do. Nothing here deletes it.

// Recovery is the state kept in recovery.json while Pimpo waits for the
// owner's choice.
type Recovery struct {
	// Folder is the quarantine folder's name, under quarantine/.
	Folder string    `json:"folder"`
	When   time.Time `json:"when"`
	Reason string    `json:"reason"`
	// Token opens the recovery page, printed at start and by `pimpo
	// token`, for when nothing can be read from the damaged file.
	Token string `json:"token"`
}

const recoveryFile = "recovery.json"

var dbFiles = []string{"pimpo.db", "pimpo.db-wal", "pimpo.db-shm"}

// Quarantine moves the database aside and puts Pimpo in recovery.
func Quarantine(home, reason string) (Recovery, error) {
	rec := Recovery{When: time.Now(), Reason: reason, Token: newToken()}
	rec.Folder = rec.When.UTC().Format("20060102-150405")
	target := filepath.Join(home, "quarantine", rec.Folder)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return Recovery{}, err
	}
	if err := moveDB(home, target); err != nil {
		return Recovery{}, err
	}
	os.WriteFile(filepath.Join(target, "reason.txt"), []byte(reason+"\n"), 0o600)
	b, _ := json.Marshal(rec)
	if err := os.WriteFile(filepath.Join(home, recoveryFile), b, 0o600); err != nil {
		return Recovery{}, err
	}
	return rec, nil
}

// moveDB moves the database files into dst without replacing any there:
// a name already taken gets a number.
func moveDB(home, dst string) error {
	for _, f := range dbFiles {
		src := filepath.Join(home, f)
		if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		to := filepath.Join(dst, f)
		for i := 2; exists(to); i++ {
			to = filepath.Join(dst, fmt.Sprintf("%d-%s", i, f))
		}
		if err := os.Rename(src, to); err != nil {
			return fmt.Errorf("set the damaged database aside: %w", err)
		}
	}
	return nil
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func newToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Recovering returns the recovery Pimpo is in, if any.
func Recovering(home string) (Recovery, bool) {
	b, err := os.ReadFile(filepath.Join(home, recoveryFile))
	if err != nil {
		return Recovery{}, false
	}
	var rec Recovery
	if json.Unmarshal(b, &rec) != nil || rec.Folder == "" || !validName(rec.Folder) {
		return Recovery{}, false
	}
	return rec, true
}

// Quarantined is the damaged database's path.
func (r Recovery) Quarantined(home string) string {
	return filepath.Join(home, "quarantine", r.Folder, "pimpo.db")
}

// Salvage reads, read-only and as far as the damaged file allows, who may
// sign in: the owner's link token and the hashes of the owner's devices.
// Nothing else is taken from it.
func (r Recovery) Salvage(home string) (token string, deviceHashes []string) {
	db, err := sql.Open("sqlite", "file:"+r.Quarantined(home)+"?mode=ro")
	if err != nil {
		return "", nil
	}
	defer db.Close()
	db.QueryRow(`SELECT value FROM kv WHERE key = 'session_token'`).Scan(&token)
	var raw string
	db.QueryRow(`SELECT value FROM kv WHERE key = 'devices'`).Scan(&raw)
	var devices []struct {
		Hash   string `json:"hash"`
		Person string `json:"person"`
		Invite bool   `json:"invite"`
	}
	json.Unmarshal([]byte(raw), &devices)
	for _, d := range devices {
		if d.Hash != "" && !d.Invite && (d.Person == "" || d.Person == "owner") {
			deviceHashes = append(deviceHashes, d.Hash)
		}
	}
	return token, deviceHashes
}

// RecoverFrom puts back a snapshot that passes its check and ends the
// recovery. Who may sign in is kept from the damaged file when it can be
// read, as a normal restore keeps it. Anything at pimpo.db meanwhile is
// moved into the quarantine folder too, not replaced.
func (r Recovery) RecoverFrom(home, name string) error {
	if err := Verify(home, name); err != nil {
		return err
	}
	access := r.salvageAccess(home)
	if err := moveDB(home, filepath.Join(home, "quarantine", r.Folder)); err != nil {
		return err
	}
	dst := filepath.Join(home, "pimpo.db")
	if err := copyFile(filepath.Join(dir(home), name, "pimpo.db"), dst); err != nil {
		return err
	}
	if access != nil {
		if err := writeAccess(dst, access); err != nil {
			return err
		}
	}
	return r.end(home)
}

func (r Recovery) salvageAccess(home string) map[string]*string {
	db, err := sql.Open("sqlite", "file:"+r.Quarantined(home)+"?mode=ro")
	if err != nil {
		return nil
	}
	defer db.Close()
	access, err := readAccess(db)
	if err != nil || len(access) == 0 {
		return nil
	}
	return access
}

// StartFresh ends the recovery with a new, empty database; the damaged one
// stays in quarantine.
func (r Recovery) StartFresh(home string) error {
	if err := moveDB(home, filepath.Join(home, "quarantine", r.Folder)); err != nil {
		return err
	}
	return r.end(home)
}

func (r Recovery) end(home string) error {
	Unstage(home)
	return os.Remove(filepath.Join(home, recoveryFile))
}

// Export writes the quarantine folder as a zip, for a closer look or a
// repair elsewhere.
func (r Recovery) Export(home string, w io.Writer) error {
	src := filepath.Join(home, "quarantine", r.Folder)
	z := zip.NewWriter(w)
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		f, err := os.Open(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		out, err := z.Create(e.Name())
		if err == nil {
			_, err = io.Copy(out, f)
		}
		f.Close()
		if err != nil {
			return err
		}
	}
	return z.Close()
}

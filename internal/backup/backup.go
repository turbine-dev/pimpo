// Package backup exports everything Pimpo keeps into one file and imports
// it on another machine: the database (routines, history, receipts,
// people), the memory with its history, installed connectors,
// and the vault's secrets. The whole file is encrypted and authenticated
// with a passphrase the owner chooses, because the vault's own key never
// leaves this machine.
package backup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"golang.org/x/crypto/scrypt"
	_ "modernc.org/sqlite"
)

// format is what an archive made now says. Its manifest lists a SHA-256
// for every file and the whole archive is sealed with the passphrase, so
// nothing in it can be changed or swapped without the passphrase.
const format = "pimpo-backup-2"

// format1 archives carry only their secrets sealed; the database, memory
// and connectors in them are not authenticated unless the archive was
// sealed as a whole (cloud backups were).
const format1 = "pimpo-backup-1"

// legacyFormats are what format 1 backups made under the earlier names say.
var legacyFormats = []string{"zodim-backup-1", "vigia-backup-1"}

func knownV1(f string) bool { return f == format1 || slices.Contains(legacyFormats, f) }

// MinPassphrase is the shortest passphrase a new backup accepts.
const MinPassphrase = 12

var errShort = fmt.Errorf("choose a passphrase of at least %d characters", MinPassphrase)

type Manifest struct {
	Format  string    `json:"format"`
	Version string    `json:"version"`
	Created time.Time `json:"created"`
	Secrets int       `json:"secrets"`
	// Files maps every other file in the archive to its SHA-256.
	Files map[string]string `json:"files,omitempty"`
}

// Secrets reads and writes the vault in plain text, only in memory.
type Secrets interface {
	Names(ctx context.Context) ([]string, error)
	Get(ctx context.Context, name string) (string, error)
	Set(ctx context.Context, name, value string) error
}

var (
	ErrPassphrase = errors.New("wrong passphrase, or the file was changed")
	// ErrUnsealed is an archive that is not sealed as a whole: anyone
	// could have changed its database, memory or connectors.
	ErrUnsealed = errors.New("this backup is not sealed as a whole, so it cannot be checked; import it with pimpo import --unsealed only if you are sure where it came from")
)

// sealedMagic starts every archive made now, followed by the scrypt cost
// it was sealed with.
const sealedMagic = "PIMPO-SEALED-2\n"

// legacyMagic starts archives sealed as a whole by earlier versions, at a
// fixed scrypt cost.
var legacyMagic = []string{"PIMPO-SEALED-1\n", "ZODIM-SEALED-1\n"}

// kdf is scrypt's cost: N = 2^logN.
type kdf struct{ logN, r, p byte }

// defaultCost is what new archives use (tests lower cost to run fast);
// legacyCost is what earlier ones used.
var (
	defaultCost = kdf{logN: 17, r: 8, p: 1}
	cost        = defaultCost
	legacyCost  = kdf{logN: 15, r: 8, p: 1}
)

// sane bounds a cost read from a file, so a crafted header cannot make
// Pimpo spend gigabytes of memory.
func (k kdf) sane() bool {
	return k.logN >= 14 && k.logN <= 18 && k.r >= 1 && k.r <= 8 && k.p >= 1 && k.p <= 4
}

func (k kdf) key(pass string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(pass), salt, 1<<k.logN, int(k.r), int(k.p), 32)
}

func sealWith(plain []byte, pass, aad string, k kdf) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := k.key(pass, salt)
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(append(salt, nonce...), gcm.Seal(nil, nonce, plain, []byte(aad))...), nil
}

func openWith(sealed []byte, pass, aad string, k kdf) ([]byte, error) {
	if len(sealed) < 16+12 {
		return nil, ErrPassphrase
	}
	key, err := k.key(pass, sealed[:16])
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, sealed[16:16+12], sealed[16+12:], []byte(aad))
	if err != nil {
		return nil, ErrPassphrase
	}
	return plain, nil
}

// seal encrypts a whole archive. The header, cost included, is
// authenticated too.
func seal(archive []byte, pass string) ([]byte, error) {
	head := append([]byte(sealedMagic), cost.logN, cost.r, cost.p)
	b, err := sealWith(archive, pass, string(head), cost)
	if err != nil {
		return nil, err
	}
	return append(head, b...), nil
}

// unseal decrypts an archive sealed as a whole and says which generation
// sealed it: 2 now, 1 before, 0 for an archive that was not sealed, whose
// reader comes back untouched.
func unseal(r io.Reader, pass string) (io.Reader, int, error) {
	br := bufio.NewReader(r)
	head, _ := br.Peek(len(sealedMagic) + 3)
	magic := string(head[:min(len(head), len(sealedMagic))])
	gen, k, aad := 0, legacyCost, magic
	switch {
	case magic == sealedMagic && len(head) == len(sealedMagic)+3:
		gen, aad = 2, string(head)
		k = kdf{head[len(magic)], head[len(magic)+1], head[len(magic)+2]}
		if !k.sane() {
			return nil, 2, errors.New("this backup's header is damaged")
		}
	case slices.Contains(legacyMagic, magic):
		gen = 1
	default:
		return br, 0, nil
	}
	all, err := io.ReadAll(io.LimitReader(br, 4<<30))
	if err != nil {
		return nil, gen, err
	}
	plain, err := openWith(all[len(aad):], pass, aad, k)
	if err != nil {
		return nil, gen, err
	}
	return bytes.NewReader(plain), gen, nil
}

type file struct {
	name string
	data []byte
}

// Export writes the archive to w, sealed as a whole with the passphrase.
func Export(ctx context.Context, db *sql.DB, home string, vault Secrets, passphrase, version string, w io.Writer) (Manifest, error) {
	if len(passphrase) < MinPassphrase {
		return Manifest{}, errShort
	}
	tmp, err := os.MkdirTemp("", "pimpo-export-")
	if err != nil {
		return Manifest{}, err
	}
	defer os.RemoveAll(tmp)
	dbCopy := filepath.Join(tmp, "pimpo.db")
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, dbCopy); err != nil {
		return Manifest{}, fmt.Errorf("copy database: %w", err)
	}
	// The copied vault rows are encrypted with this machine's key and
	// useless elsewhere; the secrets travel in the sealed archive instead.
	if c, err := sql.Open("sqlite", dbCopy); err == nil {
		c.Exec(`DELETE FROM secrets`)
		c.Exec(`VACUUM`)
		c.Close()
	}
	secrets := map[string]string{}
	names, err := vault.Names(ctx)
	if err != nil {
		return Manifest{}, err
	}
	for _, n := range names {
		v, err := vault.Get(ctx, n)
		if err != nil {
			return Manifest{}, err
		}
		secrets[n] = v
	}
	plain, _ := json.Marshal(secrets)
	raw, err := os.ReadFile(dbCopy)
	if err != nil {
		return Manifest{}, err
	}
	files := []file{{"pimpo.db", raw}, {"secrets.json", plain}}
	for _, dir := range []string{"memory", "connectors"} {
		root := filepath.Join(home, dir)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			rel, _ := filepath.Rel(home, p)
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			files = append(files, file{filepath.ToSlash(rel), b})
			return nil
		})
		if err != nil {
			return Manifest{}, err
		}
	}
	m := Manifest{Format: format, Version: version, Created: time.Now().UTC(), Secrets: len(secrets), Files: map[string]string{}}
	for _, f := range files {
		m.Files[f.name] = digest(f.data)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name string, data []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: m.Created}); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	mj, _ := json.MarshalIndent(m, "", " ")
	if err := add("manifest.json", mj); err != nil {
		return m, err
	}
	for _, f := range files {
		if err := add(f.name, f.data); err != nil {
			return m, err
		}
	}
	if err := tw.Close(); err != nil {
		return m, err
	}
	if err := gz.Close(); err != nil {
		return m, err
	}
	sealed, err := seal(buf.Bytes(), passphrase)
	if err != nil {
		return m, err
	}
	_, err = w.Write(sealed)
	return m, err
}

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func walk(r io.Reader, fn func(name string, data io.Reader) error) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.New("not a Pimpo backup")
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if err := fn(h.Name, io.LimitReader(tr, 2<<30)); err != nil {
			return err
		}
	}
}

// Unpack checks the passphrase and every file, then extracts the archive
// into dir, which must be empty or missing. It refuses an archive that is
// not sealed as a whole. It returns the secrets to put in the new
// machine's vault once the database is open there. On any error dir is
// removed, so nothing unchecked is left to place.
func Unpack(r io.Reader, dir, passphrase string) (Manifest, map[string]string, error) {
	return unpack(r, dir, passphrase, false)
}

// UnpackUnsealed is Unpack that also takes an archive not sealed as a
// whole, as the CLI and web exports made before format 2. Only the
// secrets in it are authenticated.
func UnpackUnsealed(r io.Reader, dir, passphrase string) (Manifest, map[string]string, error) {
	return unpack(r, dir, passphrase, true)
}

func unpack(r io.Reader, dir, passphrase string, allowUnsealed bool) (Manifest, map[string]string, error) {
	m, secrets, err := extract(r, dir, passphrase, allowUnsealed)
	if err != nil {
		os.RemoveAll(dir)
	}
	return m, secrets, err
}

func extract(r io.Reader, dir, passphrase string, allowUnsealed bool) (Manifest, map[string]string, error) {
	var m Manifest
	var sealedSecrets, plainSecrets []byte
	r, gen, err := unseal(r, passphrase)
	if err != nil {
		return m, nil, err
	}
	if gen == 0 && !allowUnsealed {
		return m, nil, ErrUnsealed
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return m, nil, err
	}
	written := map[string]string{}
	err = walk(r, func(name string, data io.Reader) error {
		clean := filepath.Clean(filepath.FromSlash(name))
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("unsafe path in backup: %s", name)
		}
		switch {
		case name == "manifest.json":
			return json.NewDecoder(data).Decode(&m)
		case name == "secrets.enc":
			var err error
			sealedSecrets, err = io.ReadAll(data)
			return err
		case name == "secrets.json":
			var err error
			plainSecrets, err = io.ReadAll(data)
			written[name] = digest(plainSecrets)
			return err
		case name == "zodim.db" || name == "vigia.db":
			clean = "pimpo.db"
			fallthrough
		case name == "pimpo.db", strings.HasPrefix(clean, "memory"+string(filepath.Separator)), strings.HasPrefix(clean, "connectors"+string(filepath.Separator)):
			p := filepath.Join(dir, clean)
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				return err
			}
			f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				return err
			}
			defer f.Close()
			h := sha256.New()
			if _, err := io.Copy(io.MultiWriter(f, h), data); err != nil {
				return err
			}
			written[name] = hex.EncodeToString(h.Sum(nil))
		}
		return nil
	})
	if err != nil {
		return m, nil, err
	}
	secrets := map[string]string{}
	switch {
	case gen == 2:
		if m.Format != format {
			return m, nil, errors.New("not a Pimpo backup")
		}
		if err := checkFiles(m.Files, written); err != nil {
			return m, nil, err
		}
		if err := json.Unmarshal(plainSecrets, &secrets); err != nil {
			return m, nil, err
		}
	case knownV1(m.Format):
		plain, err := openWith(sealedSecrets, passphrase, m.Format, legacyCost)
		if err != nil {
			return m, nil, err
		}
		if err := json.Unmarshal(plain, &secrets); err != nil {
			return m, nil, err
		}
	case m.Format == format:
		// A format 2 archive is always sealed; one that is not was
		// taken apart.
		return m, nil, ErrUnsealed
	default:
		return m, nil, errors.New("not a Pimpo backup")
	}
	if err := checkHistory(filepath.Join(dir, "pimpo.db")); err != nil {
		return m, nil, err
	}
	return m, secrets, nil
}

// checkFiles compares what was extracted with the manifest: the same
// files, each with its hash.
func checkFiles(want, got map[string]string) error {
	if _, ok := want["pimpo.db"]; !ok {
		return errors.New("this backup has no database")
	}
	for name, sum := range want {
		if got[name] != sum {
			return fmt.Errorf("the file %s in the backup was changed or is missing", name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			return fmt.Errorf("the file %s is not listed in the backup", name)
		}
	}
	return nil
}

// checkHistory refuses a database whose event log was edited.
func checkHistory(path string) error {
	if _, err := os.Stat(path); err != nil {
		return errors.New("this backup has no database")
	}
	ev, err := event.Open(path)
	if err != nil {
		return fmt.Errorf("the backup's database does not open: %w", err)
	}
	defer ev.Close()
	bad, err := ev.Verify(context.Background())
	if err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("the backup's history was changed (event %d does not match the one before it)", bad)
	}
	return nil
}

// Staged secrets wait in the import folder encrypted with this machine's
// vault key until they are in the new vault.
const (
	stagedSecrets = "secrets.sealed"
	// legacyStaged is plain text, as staged by earlier versions.
	legacyStaged = "secrets.json"
	stagedLabel  = "import-pending"
)

// Wrapper encrypts with this machine's vault key; *vault.Vault is one.
type Wrapper interface {
	Wrap(label string, plain []byte) ([]byte, error)
	Unwrap(label string, sealed []byte) ([]byte, error)
}

// SaveStaged keeps the secrets of an unpacked backup in stage, encrypted.
func SaveStaged(stage string, v Wrapper, secrets map[string]string) error {
	plain, _ := json.Marshal(secrets)
	sealed, err := v.Wrap(stagedLabel, plain)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage, stagedSecrets), sealed, 0o600)
}

// LoadStaged reads the secrets SaveStaged kept. The file stays until the
// whole stage is removed, after the secrets are in the vault.
func LoadStaged(stage string, v Wrapper) (map[string]string, error) {
	var plain []byte
	sealed, err := os.ReadFile(filepath.Join(stage, stagedSecrets))
	if err == nil {
		plain, err = v.Unwrap(stagedLabel, sealed)
	} else if errors.Is(err, fs.ErrNotExist) {
		plain, err = os.ReadFile(filepath.Join(stage, legacyStaged))
	}
	if err != nil {
		return nil, err
	}
	secrets := map[string]string{}
	return secrets, json.Unmarshal(plain, &secrets)
}

// Pending reports whether an import is staged in stage.
func Pending(stage string) bool {
	for _, f := range []string{stagedSecrets, legacyStaged} {
		if _, err := os.Stat(filepath.Join(stage, f)); err == nil {
			return true
		}
	}
	return false
}

// Place moves an unpacked backup into home, keeping what was there under
// home/before-import-<time> so nothing is lost.
func Place(unpacked, home string) (string, error) {
	keep := filepath.Join(home, "before-import-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(keep, 0o700); err != nil {
		return "", err
	}
	for _, name := range []string{"pimpo.db", "pimpo.db-wal", "pimpo.db-shm", "memory", "connectors"} {
		src := filepath.Join(home, name)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, filepath.Join(keep, name)); err != nil {
				return keep, err
			}
		}
	}
	for _, name := range []string{"pimpo.db", "pimpo.db-wal", "pimpo.db-shm", "memory", "connectors"} {
		src := filepath.Join(unpacked, name)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, filepath.Join(home, name)); err != nil {
				return keep, err
			}
		}
	}
	return keep, nil
}

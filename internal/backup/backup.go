// Package backup exports everything Pimpo keeps into one file and imports
// it on another machine: the database (routines, history, receipts,
// people), the memory with its history, installed connectors,
// and the vault's secrets. Secrets are re-encrypted with a passphrase the
// owner chooses, because the vault's own key never leaves this machine.
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
	"database/sql"
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

	"golang.org/x/crypto/scrypt"
	_ "modernc.org/sqlite"
)

const format = "pimpo-backup-1"

// legacyFormats are what backups made under the earlier names say.
var legacyFormats = []string{"zodim-backup-1", "vigia-backup-1"}

func known(f string) bool { return f == format || slices.Contains(legacyFormats, f) }

type Manifest struct {
	Format  string    `json:"format"`
	Version string    `json:"version"`
	Created time.Time `json:"created"`
	Secrets int       `json:"secrets"`
}

// Secrets reads and writes the vault in plain text, only in memory.
type Secrets interface {
	Names(ctx context.Context) ([]string, error)
	Get(ctx context.Context, name string) (string, error)
	Set(ctx context.Context, name, value string) error
}

var ErrPassphrase = errors.New("wrong passphrase, or the file was changed")

func seal(plain []byte, pass string) ([]byte, error) { return sealWith(plain, pass, format) }

// open decrypts secrets sealed under the backup's own format name, so
// backups made before a rename still open.
func open(sealed []byte, pass, format string) ([]byte, error) { return openWith(sealed, pass, format) }

// sealedMagic starts a backup encrypted as a whole, for storage the owner
// does not control: the database and memory, not only the secrets, are
// unreadable without the passphrase.
const sealedMagic = "PIMPO-SEALED-1\n"

// legacyMagic starts backups sealed before the rename.
var legacyMagic = []string{"ZODIM-SEALED-1\n"}

// Seal encrypts a whole exported archive.
func Seal(archive []byte, pass string) ([]byte, error) {
	if len(pass) < 8 {
		return nil, errors.New("choose a passphrase of at least 8 characters")
	}
	b, err := sealWith(archive, pass, sealedMagic)
	if err != nil {
		return nil, err
	}
	return append([]byte(sealedMagic), b...), nil
}

// unseal returns a reader over the archive, decrypting it when it was
// sealed as a whole.
func unseal(r io.Reader, pass string) (io.Reader, error) {
	br := bufio.NewReader(r)
	head, _ := br.Peek(len(sealedMagic))
	magic := string(head)
	if magic != sealedMagic && !slices.Contains(legacyMagic, magic) {
		return br, nil
	}
	all, err := io.ReadAll(io.LimitReader(br, 4<<30))
	if err != nil {
		return nil, err
	}
	plain, err := openWith(all[len(magic):], pass, magic)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(plain), nil
}

func sealWith(plain []byte, pass, aad string) ([]byte, error) {
	salt := make([]byte, 16)
	rand.Read(salt)
	key, err := scrypt.Key([]byte(pass), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return append(append(salt, nonce...), gcm.Seal(nil, nonce, plain, []byte(aad))...), nil
}

func openWith(sealed []byte, pass, aad string) ([]byte, error) {
	if len(sealed) < 16+12 {
		return nil, ErrPassphrase
	}
	key, err := scrypt.Key([]byte(pass), sealed[:16], 1<<15, 8, 1, 32)
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

// Export writes the archive to w.
func Export(ctx context.Context, db *sql.DB, home string, vault Secrets, passphrase, version string, w io.Writer) (Manifest, error) {
	if len(passphrase) < 8 {
		return Manifest{}, errors.New("choose a passphrase of at least 8 characters")
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
	// useless elsewhere; the secrets travel re-encrypted instead.
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
	sealed, err := seal(plain, passphrase)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{Format: format, Version: version, Created: time.Now().UTC(), Secrets: len(secrets)}

	gz := gzip.NewWriter(w)
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
	raw, err := os.ReadFile(dbCopy)
	if err != nil {
		return m, err
	}
	if err := add("pimpo.db", raw); err != nil {
		return m, err
	}
	if err := add("secrets.enc", sealed); err != nil {
		return m, err
	}
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
			return add(filepath.ToSlash(rel), b)
		})
		if err != nil {
			return m, err
		}
	}
	if err := tw.Close(); err != nil {
		return m, err
	}
	return m, gz.Close()
}

// Inspect reads the manifest without importing anything.
func Inspect(r io.Reader) (Manifest, error) {
	var m Manifest
	err := walk(r, func(name string, data io.Reader) error {
		if name == "manifest.json" {
			return json.NewDecoder(data).Decode(&m)
		}
		return nil
	})
	if err == nil && !known(m.Format) {
		err = errors.New("not a Pimpo backup")
	}
	return m, err
}

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

// Unpack checks the passphrase and extracts the archive into dir, which
// must be empty or missing. It returns the secrets to put in the new
// machine's vault once the database is open there.
func Unpack(r io.Reader, dir, passphrase string) (Manifest, map[string]string, error) {
	var m Manifest
	var sealed []byte
	r, err := unseal(r, passphrase)
	if err != nil {
		return m, nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return m, nil, err
	}
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
			sealed, err = io.ReadAll(data)
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
			_, err = io.Copy(f, data)
			return err
		}
		return nil
	})
	if err != nil {
		return m, nil, err
	}
	if !known(m.Format) {
		return m, nil, errors.New("not a Pimpo backup")
	}
	plain, err := open(sealed, passphrase, m.Format)
	if err != nil {
		return m, nil, err
	}
	secrets := map[string]string{}
	return m, secrets, json.Unmarshal(plain, &secrets)
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
	for _, name := range []string{"pimpo.db", "memory", "connectors"} {
		src := filepath.Join(unpacked, name)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, filepath.Join(home, name)); err != nil {
				return keep, err
			}
		}
	}
	return keep, nil
}

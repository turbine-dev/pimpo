// Package vault keeps secrets encrypted at rest. The key lives in the OS
// keychain when there is one, or in a 0600 file next to the data. Code
// outside the vault only ever holds a secret's name; the value is resolved
// at the moment a connector makes a request. A secret may also be a
// reference to an outside password manager (see outside.go).
package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("secret not found")

type Vault struct {
	db   *sql.DB
	aead cipher.AEAD
	out  *outside
}

const table = `CREATE TABLE IF NOT EXISTS secrets (name TEXT PRIMARY KEY, nonce BLOB NOT NULL, value BLOB NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')))`

// KeySource returns the 32-byte data key. It may create one only when
// create is true, which Open sets when the vault holds no secrets yet: a
// new key over existing secrets would lose them all.
type KeySource func(create bool) ([]byte, error)

// ErrKeyMissing means the vault holds secrets but their key is not here.
var ErrKeyMissing = errors.New("the vault has secrets but its key is not on this computer (keychain item or vault.key): restore it, or import a backup")

// OSKey stores the key in the OS keychain, falling back to a file in dir
// when no keychain is available (headless Linux, CI). A vault.key file,
// when present, wins: it is where the key went when the keychain failed.
func OSKey(dir string) KeySource {
	return func(create bool) ([]byte, error) {
		const service, user = "pimpo", "data-key"
		file := filepath.Join(dir, "vault.key")
		if _, err := os.Stat(file); err == nil {
			return FileKey(file)(false)
		}
		s, err := keyring.Get(service, user)
		if err == nil {
			return decodeKey(s)
		}
		if !errors.Is(err, keyring.ErrNotFound) {
			// The keychain did not answer (none here, locked, denied):
			// that does not mean the key is gone.
			if !create {
				return nil, fmt.Errorf("the OS keychain did not give the vault key: %w", err)
			}
			return FileKey(file)(true)
		}
		// Before the renames the key lived under "zodim", and "vigia"
		// before that; carry it over.
		for _, old := range []string{"zodim", "vigia"} {
			if s, err := keyring.Get(old, user); err == nil {
				keyring.Set(service, user, s)
				return decodeKey(s)
			}
		}
		if !create {
			return nil, ErrKeyMissing
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := keyring.Set(service, user, base64.StdEncoding.EncodeToString(key)); err == nil {
			return key, nil
		}
		return FileKey(file)(true)
	}
}

// FileKey keeps the key in a 0600 file. It never replaces a file that is
// there, even one it cannot read.
func FileKey(path string) KeySource {
	return func(create bool) ([]byte, error) {
		b, err := os.ReadFile(path)
		if err == nil {
			return decodeKey(string(b))
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		if !create {
			return nil, ErrKeyMissing
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		_, err = f.WriteString(base64.StdEncoding.EncodeToString(key))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		return key, nil
	}
}

func decodeKey(s string) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(k) != 32 {
		return nil, errors.New("the vault key is damaged")
	}
	return k, nil
}

func Open(db *sql.DB, key KeySource) (*Vault, error) {
	if _, err := db.Exec(table); err != nil {
		return nil, err
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM secrets`).Scan(&n); err != nil {
		return nil, err
	}
	k, err := key(n == 0)
	if err != nil {
		return nil, fmt.Errorf("vault key: %w", err)
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{db: db, aead: aead, out: newOutside()}, nil
}

// Wrap encrypts data that must wait on disk outside the database, such as
// the secrets of a staged import, with the vault key. The label binds it
// to its use.
func (v *Vault) Wrap(label string, plain []byte) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return v.aead.Seal(nonce, nonce, plain, []byte("wrap:"+label)), nil
}

// Unwrap opens what Wrap sealed under the same label.
func (v *Vault) Unwrap(label string, sealed []byte) ([]byte, error) {
	n := v.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("sealed data is too short")
	}
	plain, err := v.aead.Open(nil, sealed[:n], sealed[n:], []byte("wrap:"+label))
	if err != nil {
		return nil, errors.New("sealed data cannot be decrypted with this key")
	}
	return plain, nil
}

func (v *Vault) Set(ctx context.Context, name, value string) error {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := v.aead.Seal(nil, nonce, []byte(value), []byte(name))
	_, err := v.db.ExecContext(ctx, `INSERT INTO secrets (name, nonce, value) VALUES (?, ?, ?) ON CONFLICT(name) DO UPDATE SET nonce = excluded.nonce, value = excluded.value`, name, nonce, sealed)
	return err
}

// Get gives a secret at the moment it is used. A stored reference to an
// outside password manager is read there, with the credentials of the
// secret's owner (see ScopeOf).
func (v *Vault) Get(ctx context.Context, name string) (string, error) {
	s, err := v.stored(ctx, name)
	if err != nil || !IsReference(s) {
		return s, err
	}
	return v.Resolve(ctx, ScopeOf(name), s)
}

func (v *Vault) stored(ctx context.Context, name string) (string, error) {
	var nonce, sealed []byte
	err := v.db.QueryRowContext(ctx, `SELECT nonce, value FROM secrets WHERE name = ?`, name).Scan(&nonce, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	plain, err := v.aead.Open(nil, nonce, sealed, []byte(name))
	if err != nil {
		return "", fmt.Errorf("secret %s cannot be decrypted with this key", name)
	}
	return string(plain), nil
}

func (v *Vault) Delete(ctx context.Context, name string) error {
	_, err := v.db.ExecContext(ctx, `DELETE FROM secrets WHERE name = ?`, name)
	return err
}

// Names lists stored secrets without revealing values.
func (v *Vault) Names(ctx context.Context) ([]string, error) {
	rows, err := v.db.QueryContext(ctx, `SELECT name FROM secrets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		out = append(out, n)
	}
	sort.Strings(out)
	return out, rows.Err()
}

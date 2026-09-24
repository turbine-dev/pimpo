// Package vault keeps secrets encrypted at rest. The key lives in the OS
// keychain when there is one, or in a 0600 file next to the data. Code
// outside the vault only ever holds a secret's name; the value is resolved
// at the moment a connector makes a request.
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

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("secret not found")

type Vault struct {
	db   *sql.DB
	aead cipher.AEAD
}

const table = `CREATE TABLE IF NOT EXISTS secrets (name TEXT PRIMARY KEY, nonce BLOB NOT NULL, value BLOB NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')))`

// KeySource returns the 32-byte data key, creating it on first use.
type KeySource func() ([]byte, error)

// OSKey stores the key in the OS keychain, falling back to a file in dir
// when no keychain is available (headless Linux, CI).
func OSKey(dir string) KeySource {
	return func() ([]byte, error) {
		const service, user = "vigia", "data-key"
		if s, err := keyring.Get(service, user); err == nil {
			return base64.StdEncoding.DecodeString(s)
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := keyring.Set(service, user, base64.StdEncoding.EncodeToString(key)); err == nil {
			return key, nil
		}
		return FileKey(filepath.Join(dir, "vault.key"))()
	}
}

func FileKey(path string) KeySource {
	return func() ([]byte, error) {
		if b, err := os.ReadFile(path); err == nil {
			return base64.StdEncoding.DecodeString(string(b))
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
			return nil, err
		}
		return key, nil
	}
}

func Open(db *sql.DB, key KeySource) (*Vault, error) {
	if _, err := db.Exec(table); err != nil {
		return nil, err
	}
	k, err := key()
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
	return &Vault{db: db, aead: aead}, nil
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

func (v *Vault) Get(ctx context.Context, name string) (string, error) {
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

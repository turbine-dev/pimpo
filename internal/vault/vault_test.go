package vault

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/turbine-dev/pimpo/internal/event"
)

func TestSecretsAreEncryptedAndBoundToTheirName(t *testing.T) {
	dir := t.TempDir()
	store, err := event.Open(filepath.Join(dir, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	v, err := Open(store.DB(), FileKey(filepath.Join(dir, "vault.key")))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := v.Set(ctx, "telegram.token", "123:ABC"); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	store.DB().QueryRow(`SELECT value FROM secrets WHERE name = 'telegram.token'`).Scan(&raw)
	if bytes.Contains(raw, []byte("123:ABC")) {
		t.Fatal("secret stored in plaintext")
	}
	if got, err := v.Get(ctx, "telegram.token"); err != nil || got != "123:ABC" {
		t.Fatalf("get %q %v", got, err)
	}
	// A value copied under another name must not decrypt.
	store.DB().Exec(`INSERT INTO secrets (name, nonce, value) SELECT 'stolen', nonce, value FROM secrets WHERE name = 'telegram.token'`)
	if _, err := v.Get(ctx, "stolen"); err == nil {
		t.Fatal("a secret decrypted under a different name")
	}
	if _, err := v.Get(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("missing: %v", err)
	}
	names, _ := v.Names(ctx)
	if len(names) != 2 {
		t.Fatalf("names %v", names)
	}
	// Reopening with the same key file reads existing secrets.
	v2, _ := Open(store.DB(), FileKey(filepath.Join(dir, "vault.key")))
	if got, _ := v2.Get(ctx, "telegram.token"); got != "123:ABC" {
		t.Fatal("reopened vault cannot read")
	}
}

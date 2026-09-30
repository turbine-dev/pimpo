package vault

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/zalando/go-keyring"
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

func openWith(t *testing.T, dir string, key KeySource) (*Vault, error) {
	t.Helper()
	store, err := event.Open(filepath.Join(dir, "pimpo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return Open(store.DB(), key)
}

// A key that cannot be read is an error, never a reason to make a new one
// over secrets that need the old one.
func TestFileKeyIsNeverReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.key")
	v, err := openWith(t, dir, FileKey(path))
	if err != nil {
		t.Fatal(err)
	}
	v.Set(context.Background(), "telegram.token", "123:ABC")
	before, _ := os.ReadFile(path)
	os.Chmod(path, 0o000)
	defer os.Chmod(path, 0o600)
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running as a user who reads any file")
	}
	if _, err := openWith(t, dir, FileKey(path)); err == nil {
		t.Fatal("opened without being able to read the key")
	}
	os.Chmod(path, 0o600)
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("the key file was replaced")
	}
	os.Remove(path)
	if _, err := openWith(t, dir, FileKey(path)); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("made a new key over existing secrets: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a new key file was written")
	}
}

func TestOSKeyOnlyCreatesForAnEmptyVault(t *testing.T) {
	ctx := context.Background()
	keyring.MockInit()
	dir := t.TempDir()
	v, err := openWith(t, dir, OSKey(dir))
	if err != nil {
		t.Fatal(err)
	}
	v.Set(ctx, "telegram.token", "123:ABC")
	first, err := keyring.Get("pimpo", "data-key")
	if err != nil {
		t.Fatal("the key is not in the keychain")
	}

	// A keychain that fails must not look like a missing key.
	keyring.MockInitWithError(errors.New("keychain locked"))
	if _, err := openWith(t, dir, OSKey(dir)); err == nil {
		t.Fatal("opened a vault with secrets while the keychain failed")
	}
	// An empty vault on a machine without a keychain uses a file.
	other := t.TempDir()
	if _, err := openWith(t, other, OSKey(other)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other, "vault.key")); err != nil {
		t.Fatal("no file key without a keychain")
	}

	// The key is gone from the keychain: secrets are kept, not orphaned.
	keyring.MockInit()
	if _, err := openWith(t, dir, OSKey(dir)); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("made a new key over existing secrets: %v", err)
	}
	if _, err := keyring.Get("pimpo", "data-key"); err == nil {
		t.Fatal("a new key was saved")
	}
	keyring.Set("pimpo", "data-key", first)
	v2, err := openWith(t, dir, OSKey(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := v2.Get(ctx, "telegram.token"); got != "123:ABC" {
		t.Fatal("the keychain key does not read the secrets")
	}

	// A vault.key file wins over the keychain.
	if _, err := openWith(t, other, OSKey(other)); err != nil {
		t.Fatalf("the file key was not used: %v", err)
	}
}

func TestWrapIsBoundToTheKeyAndLabel(t *testing.T) {
	dir := t.TempDir()
	v, _ := openWith(t, dir, FileKey(filepath.Join(dir, "k")))
	sealed, err := v.Wrap("import", []byte(`{"a":"b"}`))
	if err != nil || bytes.Contains(sealed, []byte(`"b"`)) {
		t.Fatalf("%v %q", err, sealed)
	}
	if plain, err := v.Unwrap("import", sealed); err != nil || string(plain) != `{"a":"b"}` {
		t.Fatalf("%q %v", plain, err)
	}
	if _, err := v.Unwrap("other", sealed); err == nil {
		t.Fatal("opened under another label")
	}
	other, _ := openWith(t, t.TempDir(), FileKey(filepath.Join(t.TempDir(), "k")))
	if _, err := other.Unwrap("import", sealed); err == nil {
		t.Fatal("opened with another key")
	}
}

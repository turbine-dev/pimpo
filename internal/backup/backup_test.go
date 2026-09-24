package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/vault"
)

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	ev, _ := event.Open(filepath.Join(home, "vigia.db"))
	defer ev.Close()
	v, _ := vault.Open(ev.DB(), vault.FileKey(filepath.Join(home, "key-a")))
	v.Set(ctx, "telegram.token", "123:secret")
	ev.Append(ctx, "routine.saved", "human:owner", map[string]string{"id": "bom-dia"})
	ev.Put(ctx, "mail.user", "eu@exemplo.com")
	os.MkdirAll(filepath.Join(home, "memory"), 0o700)
	os.WriteFile(filepath.Join(home, "memory", "facts.json"), []byte(`[{"text":"Ana é minha irmã"}]`), 0o600)
	os.MkdirAll(filepath.Join(home, "connectors", "tides"), 0o700)
	os.WriteFile(filepath.Join(home, "connectors", "tides", "connector.json"), []byte(`{}`), 0o600)

	var buf bytes.Buffer
	if _, err := Export(ctx, ev.DB(), home, v, "short", "1.0", &buf); err == nil {
		t.Fatal("accepted a short passphrase")
	}
	m, err := Export(ctx, ev.DB(), home, v, "correct horse", "1.0", &buf)
	if err != nil || m.Secrets != 1 {
		t.Fatalf("%+v %v", m, err)
	}
	if bytes.Contains(buf.Bytes(), []byte("123:secret")) {
		t.Fatal("a secret is readable in the backup")
	}
	if got, err := Inspect(bytes.NewReader(buf.Bytes())); err != nil || got.Version != "1.0" {
		t.Fatalf("inspect %+v %v", got, err)
	}
	if _, _, err := Unpack(bytes.NewReader(buf.Bytes()), t.TempDir(), "wrong passphrase"); !errors.Is(err, ErrPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}

	// A new machine, with its own vault key.
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "vigia.db"), []byte("old"), 0o600)
	unpacked := t.TempDir()
	_, secrets, err := Unpack(bytes.NewReader(buf.Bytes()), unpacked, "correct horse")
	if err != nil || secrets["telegram.token"] != "123:secret" {
		t.Fatalf("unpack %v %v", secrets, err)
	}
	keep, err := Place(unpacked, other)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(keep, "vigia.db")); string(b) != "old" {
		t.Fatal("the previous data was not kept")
	}
	ev2, err := event.Open(filepath.Join(other, "vigia.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ev2.Close()
	v2, _ := vault.Open(ev2.DB(), vault.FileKey(filepath.Join(other, "key-b")))
	for k, val := range secrets {
		v2.Set(ctx, k, val)
	}
	if tok, _ := v2.Get(ctx, "telegram.token"); tok != "123:secret" {
		t.Fatal("secret lost")
	}
	if u, _ := ev2.Get(ctx, "mail.user"); u != "eu@exemplo.com" {
		t.Fatal("settings lost")
	}
	if _, err := ev2.Verify(ctx); err != nil {
		t.Fatalf("event chain broken after import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(other, "connectors", "tides", "connector.json")); err != nil {
		t.Fatal("connectors lost")
	}
	if b, _ := os.ReadFile(filepath.Join(other, "memory", "facts.json")); !bytes.Contains(b, []byte("Ana")) {
		t.Fatal("memory lost")
	}
}

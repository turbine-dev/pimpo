package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/denerFernandes/zodim/internal/event"
	"github.com/denerFernandes/zodim/internal/vault"
)

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	ev, _ := event.Open(filepath.Join(home, "zodim.db"))
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
	os.WriteFile(filepath.Join(other, "zodim.db"), []byte("old"), 0o600)
	unpacked := t.TempDir()
	_, secrets, err := Unpack(bytes.NewReader(buf.Bytes()), unpacked, "correct horse")
	if err != nil || secrets["telegram.token"] != "123:secret" {
		t.Fatalf("unpack %v %v", secrets, err)
	}
	keep, err := Place(unpacked, other)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(keep, "zodim.db")); string(b) != "old" {
		t.Fatal("the previous data was not kept")
	}
	ev2, err := event.Open(filepath.Join(other, "zodim.db"))
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

// Backups made before the rename say vigia-backup-1 and carry vigia.db.
func TestImportsBackupsFromBeforeTheRename(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	ev, _ := event.Open(filepath.Join(home, "zodim.db"))
	defer ev.Close()
	v, _ := vault.Open(ev.DB(), vault.FileKey(filepath.Join(home, "key")))
	v.Set(ctx, "telegram.token", "123:secret")
	var buf bytes.Buffer
	if _, err := Export(ctx, ev.DB(), home, v, "correct horse", "0.5", &buf); err != nil {
		t.Fatal(err)
	}
	var old bytes.Buffer
	gw := gzip.NewWriter(&old)
	tw := tar.NewWriter(gw)
	walk(bytes.NewReader(buf.Bytes()), func(name string, data io.Reader) error {
		b, _ := io.ReadAll(data)
		switch name {
		case "zodim.db":
			name = "vigia.db"
		case "manifest.json":
			b = bytes.Replace(b, []byte(format), []byte(legacyFormat), 1)
		}
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b)), Typeflag: tar.TypeReg})
		tw.Write(b)
		return nil
	})
	tw.Close()
	gw.Close()
	if m, err := Inspect(bytes.NewReader(old.Bytes())); err != nil || m.Format != legacyFormat {
		t.Fatalf("inspect %+v %v", m, err)
	}
	dir := t.TempDir()
	_, secrets, err := Unpack(bytes.NewReader(old.Bytes()), dir, "correct horse")
	if err != nil || secrets["telegram.token"] != "123:secret" {
		t.Fatalf("%v %v", secrets, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "zodim.db")); err != nil {
		t.Fatal("the old database name was not mapped")
	}
}

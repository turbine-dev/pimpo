package ocr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRead(t *testing.T) {
	dir := t.TempDir()
	// A stand-in that knows only English and prints what it "sees".
	os.WriteFile(filepath.Join(dir, "tesseract"), []byte("#!/bin/sh\n[ \"$4\" = eng ] || exit 1\ncat >/dev/null\nprintf 'Conta de luz\\n\\n  Vencimento   10/10  R$ 187,90\\n'\n"), 0o755)
	t.Setenv("PATH", dir)
	text, err := Tesseract{}.Read(context.Background(), []byte("jpeg"))
	if err != nil || text != "Conta de luz\nVencimento 10/10 R$ 187,90" {
		t.Fatalf("%q %v", text, err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (Tesseract{}).Read(context.Background(), nil); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("%v", err)
	}
}

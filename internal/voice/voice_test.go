package voice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeTools puts stand-ins for ffmpeg and whisper-cli first on PATH.
func fakeTools(t *testing.T, whisperOut string) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte("#!/bin/sh\nfor a; do last=$a; done\ncp \"$5\" \"$last\"\n"), 0o755)
	os.WriteFile(filepath.Join(dir, "whisper-cli"), []byte("#!/bin/sh\necho '"+whisperOut+"'\n"), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestTranscribe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in tool is a shell script")
	}
	fakeTools(t, "  Me lembra de pagar\n a luz amanhã ")
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	w := Whisper{Model: model}
	if err := w.Ready(); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("ready without a model: %v", err)
	}
	os.WriteFile(model, []byte("m"), 0o644)
	text, err := w.Transcribe(context.Background(), []byte("OggS..."))
	if err != nil || text != "Me lembra de pagar a luz amanhã" {
		t.Fatalf("%q %v", text, err)
	}
}

func TestNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := (Whisper{Model: "x"}).Transcribe(context.Background(), nil); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("%v", err)
	}
}

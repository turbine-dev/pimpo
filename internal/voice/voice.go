// Package voice turns voice notes into text on this machine, with
// whisper.cpp, so nothing the owner says leaves the house to be understood.
package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Whisper runs ffmpeg to get 16 kHz mono audio and whisper.cpp to read it.
type Whisper struct {
	// Model is a ggml model file, e.g. ggml-base.bin.
	Model string
	// Language is a whisper language code, or "auto".
	Language string
}

var ErrNotInstalled = errors.New("voice notes need whisper.cpp and ffmpeg (on a Mac: brew install whisper-cpp ffmpeg) and a model in the models folder")

func find(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

// Ready reports whether transcription can work here.
func (w Whisper) Ready() error {
	if find("ffmpeg") == "" || find("whisper-cli", "whisper-cpp") == "" {
		return ErrNotInstalled
	}
	if _, err := os.Stat(w.Model); err != nil {
		return fmt.Errorf("%w (expected %s)", ErrNotInstalled, w.Model)
	}
	return nil
}

func (w Whisper) Transcribe(ctx context.Context, audio []byte) (string, error) {
	if err := w.Ready(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp("", "vigia-voice-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	in, wav := filepath.Join(dir, "in.audio"), filepath.Join(dir, "in.wav")
	if err := os.WriteFile(in, audio, 0o600); err != nil {
		return "", err
	}
	if out, err := exec.CommandContext(ctx, find("ffmpeg"), "-nostdin", "-loglevel", "error", "-i", in, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav).CombinedOutput(); err != nil {
		return "", fmt.Errorf("could not read the audio: %s", strings.TrimSpace(string(out)))
	}
	lang := w.Language
	if lang == "" {
		lang = "auto"
	}
	out, err := exec.CommandContext(ctx, find("whisper-cli", "whisper-cpp"), "-m", w.Model, "-f", wav, "-l", lang, "-nt", "-np").Output()
	if err != nil {
		return "", fmt.Errorf("transcription failed: %w", err)
	}
	text := strings.Join(strings.Fields(string(out)), " ")
	if text == "" {
		return "", errors.New("I could not make out any words")
	}
	return text, nil
}

// Package speech reads text aloud into an audio file on this machine: the
// text never leaves the house. On a Mac it uses the system voices (say)
// and converts to AAC (afconvert); elsewhere espeak-ng and ffmpeg.
package speech

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MaxText bounds what one file reads, about 20 minutes of speech.
const MaxText = 20000

var ErrUnavailable = errors.New("reading aloud needs the macOS voices (say) or espeak-ng with ffmpeg")

// Voice is a system voice for a language.
type Voice struct {
	Name     string `json:"name"`
	Language string `json:"language"` // pt-BR, en-US…
}

// novelty voices are sound effects, not readers.
var novelty = map[string]bool{"Albert": true, "Bad News": true, "Bahh": true, "Bells": true, "Boing": true, "Bubbles": true, "Cellos": true,
	"Good News": true, "Jester": true, "Organ": true, "Superstar": true, "Trinoids": true, "Whisper": true, "Wobble": true, "Zarvox": true, "Fred": true, "Junior": true, "Ralph": true}

// preferred are the natural-sounding voices macOS ships, by language.
var preferred = map[string][]string{"pt-BR": {"Luciana"}, "pt-PT": {"Joana"}, "en-US": {"Samantha"}, "en-GB": {"Daniel"}, "es-ES": {"Mónica"}, "es-MX": {"Paulina"},
	"fr-FR": {"Thomas", "Jacques"}, "de-DE": {"Anna"}, "it-IT": {"Alice"}, "ja-JP": {"Kyoko"}, "ko-KR": {"Yuna"}, "zh-CN": {"Tingting"}, "ru-RU": {"Milena"}}

var sayLine = regexp.MustCompile(`^(.+?)\s+([a-z]{2,3}_[A-Za-z0-9]+)\s+#`)

// Voices lists the macOS voices, by language.
func Voices(ctx context.Context) []Voice {
	out, err := exec.CommandContext(ctx, "say", "-v", "?").Output()
	if err != nil {
		return nil
	}
	var vs []Voice
	for _, l := range strings.Split(string(out), "\n") {
		m := sayLine.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil || novelty[m[1]] {
			continue
		}
		vs = append(vs, Voice{Name: m[1], Language: strings.Replace(m[2], "_", "-", 1)})
	}
	return vs
}

// Pick chooses the voice for a language: a premium or enhanced one when
// installed, then the one macOS is known for, then any of that language,
// then any of the same language family (pt for pt-BR).
func Pick(voices []Voice, language string) (Voice, bool) {
	lang := strings.ReplaceAll(language, "_", "-")
	family, _, _ := strings.Cut(lang, "-")
	score := func(v Voice) int {
		s := 0
		switch {
		case strings.EqualFold(v.Language, lang):
			s = 100
		case strings.EqualFold(strings.SplitN(v.Language, "-", 2)[0], family):
			s = 50
		default:
			return -1
		}
		n := strings.ToLower(v.Name)
		if strings.Contains(n, "premium") {
			s += 30
		} else if strings.Contains(n, "enhanced") || strings.Contains(n, "melhorad") {
			s += 20
		}
		for _, p := range preferred[lang] {
			if strings.HasPrefix(v.Name, p) {
				s += 10
			}
		}
		if strings.Contains(v.Name, "(") && !strings.Contains(n, "premium") && !strings.Contains(n, "enhanced") {
			s -= 5 // Siri-era multilingual voices sound worse reading long text
		}
		return s
	}
	best, bestScore := Voice{}, -1
	for _, v := range voices {
		if s := score(v); s > bestScore {
			best, bestScore = v, s
		}
	}
	return best, bestScore >= 0
}

// Speak reads text in a language and returns an M4A (AAC) file and its
// length in seconds.
func Speak(ctx context.Context, text, language string) ([]byte, float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, 0, errors.New("nothing to read")
	}
	if r := []rune(text); len(r) > MaxText {
		text = string(r[:MaxText])
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp("", "pimpo-speech-")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	in, raw, out := filepath.Join(dir, "text.txt"), filepath.Join(dir, "speech.aiff"), filepath.Join(dir, "speech.m4a")
	os.WriteFile(in, []byte(text), 0o600)
	var stderr bytes.Buffer
	switch {
	case has("say") && has("afconvert"):
		v, ok := Pick(Voices(ctx), language)
		if !ok {
			return nil, 0, fmt.Errorf("no voice for %s is installed; add one in System Settings › Accessibility › Spoken Content", language)
		}
		cmd := exec.CommandContext(ctx, "say", "-v", v.Name, "-f", in, "-o", raw)
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, 0, fmt.Errorf("say: %v %s", err, strings.TrimSpace(stderr.String()))
		}
		cmd = exec.CommandContext(ctx, "afconvert", "-f", "m4af", "-d", "aac", "-b", "64000", raw, out)
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, 0, fmt.Errorf("afconvert: %v %s", err, strings.TrimSpace(stderr.String()))
		}
	case has("espeak-ng") && has("ffmpeg"):
		wav := filepath.Join(dir, "speech.wav")
		family, _, _ := strings.Cut(language, "-")
		if err := exec.CommandContext(ctx, "espeak-ng", "-v", strings.ToLower(family), "-f", in, "-w", wav).Run(); err != nil {
			return nil, 0, fmt.Errorf("espeak-ng: %w", err)
		}
		if err := exec.CommandContext(ctx, "ffmpeg", "-y", "-loglevel", "error", "-i", wav, "-c:a", "aac", "-b:a", "64k", out).Run(); err != nil {
			return nil, 0, fmt.Errorf("ffmpeg: %w", err)
		}
	default:
		return nil, 0, ErrUnavailable
	}
	b, err := os.ReadFile(out)
	if err != nil {
		return nil, 0, err
	}
	return b, duration(ctx, out), nil
}

// duration asks afinfo (or ffprobe) how long the file is; 0 when neither
// can tell.
func duration(ctx context.Context, path string) float64 {
	if has("afinfo") {
		out, _ := exec.CommandContext(ctx, "afinfo", path).Output()
		if m := regexp.MustCompile(`estimated duration: ([0-9.]+)`).FindSubmatch(out); m != nil {
			var s float64
			fmt.Sscanf(string(m[1]), "%f", &s)
			return s
		}
	}
	return 0
}

func has(name string) bool { _, err := exec.LookPath(name); return err == nil }

// Encode turns any audio file the system can read (a WAV from a local
// voice) into M4A (AAC), with its length in seconds.
func Encode(ctx context.Context, in string) ([]byte, float64, error) {
	out := strings.TrimSuffix(in, filepath.Ext(in)) + ".m4a"
	var stderr bytes.Buffer
	var cmd *exec.Cmd
	switch {
	case has("afconvert"):
		cmd = exec.CommandContext(ctx, "afconvert", "-f", "m4af", "-d", "aac", "-b", "64000", in, out)
	case has("ffmpeg"):
		cmd = exec.CommandContext(ctx, "ffmpeg", "-y", "-loglevel", "error", "-i", in, "-c:a", "aac", "-b:a", "64k", out)
	default:
		return nil, 0, errors.New("converting audio needs afconvert (macOS) or ffmpeg")
	}
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, 0, fmt.Errorf("converting audio: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	b, err := os.ReadFile(out)
	if err != nil {
		return nil, 0, err
	}
	return b, duration(ctx, out), nil
}

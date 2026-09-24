// Package ocr reads text from photos on this machine with Tesseract, so a
// photo of a handwritten quote or a receipt never leaves the house.
package ocr

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

var ErrNotInstalled = errors.New("reading photos needs Tesseract (on a Mac: brew install tesseract tesseract-lang)")

type Tesseract struct {
	// Languages, e.g. "por+eng"; unavailable ones fall back to English.
	Languages string
}

func (t Tesseract) Read(ctx context.Context, image []byte) (string, error) {
	bin, err := exec.LookPath("tesseract")
	if err != nil {
		return "", ErrNotInstalled
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	langs := t.Languages
	if langs == "" {
		langs = "por+eng"
	}
	run := func(l string) (string, error) {
		cmd := exec.CommandContext(ctx, bin, "stdin", "stdout", "-l", l)
		cmd.Stdin = bytes.NewReader(image)
		out, err := cmd.Output()
		return string(out), err
	}
	out, err := run(langs)
	if err != nil && langs != "eng" {
		out, err = run("eng")
	}
	if err != nil {
		return "", errors.New("could not read the photo")
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return "", errors.New("I found no text in the photo")
	}
	return strings.Join(lines, "\n"), nil
}

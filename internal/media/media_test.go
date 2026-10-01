package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// needFFmpeg skips without a working ffmpeg, except where CI says it must
// be there.
func needFFmpeg(t *testing.T) {
	t.Helper()
	if err := Available(context.Background()); err != nil {
		if os.Getenv("PIMPO_MEDIA_REQUIRED") != "" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
}

func TestAShortIsMadeAndPassesItsChecks(t *testing.T) {
	needFFmpeg(t)
	ctx := context.Background()
	dir := t.TempDir()
	img, voice := filepath.Join(dir, "shot.png"), filepath.Join(dir, "voice.m4a")
	for _, args := range [][]string{
		{"-y", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=1280x720", "-frames:v", "1", img},
		{"-y", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:a", "aac", "-af", "volume=-30dB", voice},
	} {
		if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	out := filepath.Join(dir, "short.mp4")
	secs, err := Render(ctx, Formats["short"], []Scene{{Image: img, Audio: voice, Text: "Open the dashboard"}, {Text: "Then press Run", Seconds: 1.5}}, out)
	if err != nil {
		t.Fatal(err)
	}
	if secs < 3.5 || secs > 4.5 {
		t.Fatalf("lasts %.2f s", secs)
	}
	r, err := Check(ctx, out, Formats["short"])
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK() || r.Width != 1080 || r.Height != 1920 || !r.Captions || r.LUFS == nil {
		t.Fatalf("report = %+v", r)
	}
	wrong, _ := Check(ctx, out, Formats["tutorial"])
	if wrong.OK() || !strings.Contains(strings.Join(wrong.Problems, " "), "not the tutorial's 1920x1080") {
		t.Fatalf("a short passed as a tutorial: %+v", wrong)
	}
	silent := filepath.Join(dir, "silent.mp4")
	exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=red:s=1080x1920:d=2", "-c:v", "libx264", "-pix_fmt", "yuv420p", silent).Run()
	if r, _ := Check(ctx, silent, Formats["short"]); r.OK() || !strings.Contains(strings.Join(r.Problems, " "), "no captions") || !strings.Contains(strings.Join(r.Problems, " "), "no sound") {
		t.Fatalf("a silent video without captions passed: %+v", r)
	}
}

func TestCaptionTimes(t *testing.T) {
	for secs, want := range map[float64]string{0: "00:00:00,000", 61.5: "00:01:01,500", 3725.25: "01:02:05,250"} {
		if got := stamp(secs); got != want {
			t.Errorf("%v = %s, want %s", secs, got, want)
		}
	}
	if _, err := Render(context.Background(), Formats["short"], nil, "x.mp4"); err == nil {
		t.Fatal("a video of no scenes")
	}
}

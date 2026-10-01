// Package media builds videos on this computer with ffmpeg: scenes of an
// image, a voice and a caption, in fixed formats, and checks a video
// without a model (length, aspect, loudness, captions) before a person is
// asked about it.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A Format is a fixed shape a video is made in.
type Format struct {
	Name       string  `json:"name"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	MaxSeconds float64 `json:"max_seconds"`
}

var Formats = map[string]Format{
	"tutorial": {Name: "tutorial", Width: 1920, Height: 1080, MaxSeconds: 20 * 60},
	"short":    {Name: "short", Width: 1080, Height: 1920, MaxSeconds: 60},
}

// Loudness targets: videos are made at TargetLUFS and pass a check
// within LUFSRange of it.
const (
	TargetLUFS   = -14.0
	LUFSRange    = 3.0
	sceneDefault = 4.0
	sceneMax     = 120.0
	maxScenes    = 60
	renderLimit  = 20 * time.Minute
)

// ErrNoFFmpeg says ffmpeg is missing or does not run.
var ErrNoFFmpeg = errors.New("ffmpeg is not installed or does not run here (macOS: brew install ffmpeg)")

// Available says whether ffmpeg and ffprobe run.
func Available(ctx context.Context) error {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if err := exec.CommandContext(ctx, bin, "-version").Run(); err != nil {
			return ErrNoFFmpeg
		}
	}
	return nil
}

// A Scene is one stretch of a video: an image (or a plain background), a
// voice (or silence for Seconds) and the caption shown while it plays.
type Scene struct {
	Image   string  `json:"image,omitempty"`
	Audio   string  `json:"audio,omitempty"`
	Text    string  `json:"text,omitempty"`
	Seconds float64 `json:"seconds,omitempty"`
}

// Render makes a video of the scenes in format f at out, its voice brought
// to the target loudness and its captions as a subtitle track.
func Render(ctx context.Context, f Format, scenes []Scene, out string) (float64, error) {
	if len(scenes) == 0 || len(scenes) > maxScenes {
		return 0, fmt.Errorf("a video has 1 to %d scenes", maxScenes)
	}
	if err := Available(ctx); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, renderLimit)
	defer cancel()
	tmp, err := os.MkdirTemp("", "pimpo-render-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)
	fit := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1,format=yuv420p", f.Width, f.Height, f.Width, f.Height)
	var list strings.Builder
	var srt strings.Builder
	at := 0.0
	for i, sc := range scenes {
		secs := sc.Seconds
		if sc.Audio != "" {
			if d, err := Duration(ctx, sc.Audio); err == nil && d > secs {
				secs = d + 0.3
			}
		}
		if secs <= 0 {
			secs = sceneDefault
		}
		secs = math.Min(secs, sceneMax)
		seg := filepath.Join(tmp, fmt.Sprintf("seg%03d.mp4", i))
		args := []string{"-y", "-loglevel", "error"}
		if sc.Image != "" {
			args = append(args, "-loop", "1", "-t", ff(secs), "-i", sc.Image)
		} else {
			args = append(args, "-f", "lavfi", "-t", ff(secs), "-i", fmt.Sprintf("color=c=0x111111:s=%dx%d:r=30", f.Width, f.Height))
		}
		if sc.Audio != "" {
			args = append(args, "-i", sc.Audio)
		} else {
			args = append(args, "-f", "lavfi", "-t", ff(secs), "-i", "anullsrc=r=48000:cl=stereo")
		}
		args = append(args, "-vf", fit, "-r", "30", "-c:v", "libx264", "-preset", "veryfast", "-tune", "stillimage", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-ar", "48000", "-ac", "2", "-t", ff(secs), seg)
		if err := run(ctx, "ffmpeg", args...); err != nil {
			return 0, fmt.Errorf("scene %d: %w", i+1, err)
		}
		fmt.Fprintf(&list, "file '%s'\n", seg)
		if t := strings.TrimSpace(sc.Text); t != "" {
			fmt.Fprintf(&srt, "%d\n%s --> %s\n%s\n\n", i+1, stamp(at), stamp(at+secs), t)
		}
		at += secs
	}
	listFile, srtFile := filepath.Join(tmp, "list.txt"), filepath.Join(tmp, "captions.srt")
	os.WriteFile(listFile, []byte(list.String()), 0o600)
	joined := filepath.Join(tmp, "joined.mp4")
	if err := run(ctx, "ffmpeg", "-y", "-loglevel", "error", "-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", joined); err != nil {
		return 0, err
	}
	args := []string{"-y", "-loglevel", "error", "-i", joined}
	if srt.Len() > 0 {
		os.WriteFile(srtFile, []byte(srt.String()), 0o600)
		args = append(args, "-i", srtFile, "-map", "0:v", "-map", "0:a", "-map", "1:s", "-c:s", "mov_text", "-metadata:s:s:0", "language=und")
	}
	args = append(args, "-c:v", "copy", "-af", fmt.Sprintf("loudnorm=I=%.0f:TP=-1.5:LRA=11", TargetLUFS), "-c:a", "aac", "-ar", "48000", "-movflags", "+faststart", out)
	if err := run(ctx, "ffmpeg", args...); err != nil {
		return 0, err
	}
	return at, nil
}

// A Report is what a check found.
type Report struct {
	Format   string   `json:"format"`
	Seconds  float64  `json:"seconds"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	LUFS     *float64 `json:"lufs,omitempty"`
	Captions bool     `json:"captions"`
	Problems []string `json:"problems"`
}

// OK says whether nothing was found wrong.
func (r Report) OK() bool { return len(r.Problems) == 0 }

// Check measures a video against its format.
func Check(ctx context.Context, path string, f Format) (Report, error) {
	r := Report{Format: f.Name, Problems: []string{}}
	if err := Available(ctx); err != nil {
		return r, err
	}
	var probe struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	out, err := output(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type,width,height:format=duration", "-of", "json", path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return r, err
	}
	r.Seconds, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	audio := false
	for _, s := range probe.Streams {
		switch s.CodecType {
		case "video":
			r.Width, r.Height = s.Width, s.Height
		case "audio":
			audio = true
		case "subtitle":
			r.Captions = true
		}
	}
	if r.Width == 0 {
		r.Problems = append(r.Problems, "it has no picture")
	} else if math.Abs(float64(r.Width)/float64(r.Height)-float64(f.Width)/float64(f.Height)) > 0.01 {
		r.Problems = append(r.Problems, fmt.Sprintf("it is %dx%d, not the %s's %dx%d", r.Width, r.Height, f.Name, f.Width, f.Height))
	}
	if r.Seconds < 1 {
		r.Problems = append(r.Problems, "it is shorter than a second")
	} else if r.Seconds > f.MaxSeconds {
		r.Problems = append(r.Problems, fmt.Sprintf("it lasts %.0f s, over the %s's %.0f s", r.Seconds, f.Name, f.MaxSeconds))
	}
	if !r.Captions {
		r.Problems = append(r.Problems, "it has no captions")
	}
	if audio {
		if l, err := loudness(ctx, path); err == nil {
			r.LUFS = &l
			if math.Abs(l-TargetLUFS) > LUFSRange && l > -70 {
				r.Problems = append(r.Problems, fmt.Sprintf("its loudness is %.1f LUFS, not about %.0f", l, TargetLUFS))
			}
		}
	} else {
		r.Problems = append(r.Problems, "it has no sound")
	}
	return r, nil
}

var integrated = regexp.MustCompile(`I:\s+(-?[0-9.]+|-inf) LUFS`)

// loudness is a file's integrated loudness, by ffmpeg's EBU R128 meter.
func loudness(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostats", "-i", path, "-map", "0:a:0", "-af", "ebur128=framelog=quiet", "-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("measuring loudness: %w", err)
	}
	all := integrated.FindAllStringSubmatch(stderr.String(), -1)
	if len(all) == 0 {
		return 0, errors.New("no loudness measured")
	}
	v := all[len(all)-1][1]
	if v == "-inf" {
		return -70, nil
	}
	return strconv.ParseFloat(v, 64)
}

// Duration is how long an audio or video file lasts.
func Duration(ctx context.Context, path string) (float64, error) {
	out, err := output(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

func ff(secs float64) string { return strconv.FormatFloat(secs, 'f', 3, 64) }

// stamp is a time in SubRip's form.
func stamp(secs float64) string {
	ms := int(math.Round(secs * 1000))
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func run(ctx context.Context, bin string, args ...string) error {
	_, err := output(ctx, bin, args...)
	return err
}

func output(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		return nil, fmt.Errorf("%s: %v: %s", bin, err, msg)
	}
	return stdout.Bytes(), nil
}

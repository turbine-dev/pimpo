// Package sandbox runs short programs in an isolated container: no
// network, a read-only root, no Linux capabilities, an unprivileged user,
// no environment, and limits on time, memory, CPU, processes and output.
// See docs/rfcs/0001-code-sandbox.md.
package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Job is one program to run.
type Job struct {
	Language string            `json:"language"`
	Code     string            `json:"code"`
	Files    map[string]string `json:"files,omitempty"`
	Stdin    string            `json:"stdin,omitempty"`
}

// Result is what came out.
type Result struct {
	Stdout   string            `json:"stdout"`
	Stderr   string            `json:"stderr"`
	ExitCode int               `json:"exit_code"`
	Files    map[string]string `json:"files"`
	Seconds  float64           `json:"seconds"`
	TimedOut bool              `json:"timed_out,omitempty"`
}

// Limits of every run.
const (
	Timeout    = 60 * time.Second
	maxOutput  = 64 << 10
	maxFiles   = 5 << 20
	maxCode    = 256 << 10
	Memory     = "512m"
	CPUs       = "1"
	Processes  = "128"
	sandboxUID = "65534:65534"
)

type runtimeSpec struct {
	image string
	file  string
	cmd   []string
}

// languages are the programs a job may be; images are official ones.
var languages = map[string]runtimeSpec{
	"python":     {image: "python:3.12-alpine", file: "main.py", cmd: []string{"python", "-I", "main.py"}},
	"javascript": {image: "node:22-alpine", file: "main.js", cmd: []string{"node", "main.js"}},
	"shell":      {image: "alpine:3.20", file: "main.sh", cmd: []string{"sh", "main.sh"}},
}

// Languages lists what a job may be written in.
func Languages() []string { return []string{"python", "javascript", "shell"} }

// Docker runs jobs with Docker's command line.
type Docker struct {
	// Bin is the docker (or compatible) command; empty finds "docker".
	Bin string
}

func (d Docker) bin() (string, error) {
	if d.Bin != "" {
		return d.Bin, nil
	}
	p, err := exec.LookPath("docker")
	if err != nil {
		return "", errors.New("the code sandbox needs Docker, which is not installed or not on the PATH")
	}
	return p, nil
}

// Available says whether Docker answers.
func (d Docker) Available(ctx context.Context) error {
	bin, err := d.bin()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		return fmt.Errorf("Docker is installed but not running: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// args are the docker run arguments for a job in dir: the isolation lives
// here, and the tests check every flag.
func args(name, dir string, rt runtimeSpec) []string {
	a := []string{"run", "--rm", "--name", name,
		"--network", "none",
		"--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--user", sandboxUID,
		"--memory", Memory, "--memory-swap", Memory, "--cpus", CPUs, "--pids-limit", Processes,
		"--ipc", "none",
		"-v", dir + ":/work", "-w", "/work",
		"-i", rt.image}
	return append(a, rt.cmd...)
}

// Run runs a job and returns its output. A job that runs past Timeout is
// killed and reported as timed out.
func (d Docker) Run(ctx context.Context, j Job) (Result, error) {
	rt, ok := languages[j.Language]
	if !ok {
		return Result{}, fmt.Errorf("language must be one of %s", strings.Join(Languages(), ", "))
	}
	if strings.TrimSpace(j.Code) == "" || len(j.Code) > maxCode {
		return Result{}, errors.New("code must be between 1 byte and 256 KB")
	}
	bin, err := d.bin()
	if err != nil {
		return Result{}, err
	}
	dir, err := os.MkdirTemp("", "pimpo-sandbox-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)
	total := len(j.Code)
	for name, body := range j.Files {
		clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
		if clean == "." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) || clean == rt.file || strings.HasPrefix(clean, "out/") {
			return Result{}, fmt.Errorf("file name %q is not allowed", name)
		}
		total += len(body)
		if total > maxFiles {
			return Result{}, errors.New("the files are larger than 5 MB")
		}
		p := filepath.Join(dir, filepath.FromSlash(clean))
		os.MkdirAll(filepath.Dir(p), 0o777)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return Result{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, rt.file), []byte(j.Code), 0o644); err != nil {
		return Result{}, err
	}
	// The container's user is unprivileged, so its folders must be open.
	os.MkdirAll(filepath.Join(dir, "out"), 0o777)
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			os.Chmod(p, 0o777)
		}
		return nil
	})

	if err := d.pull(ctx, bin, rt.image); err != nil {
		return Result{}, err
	}
	b := make([]byte, 6)
	rand.Read(b)
	name := "pimpo-sandbox-" + hex.EncodeToString(b)
	run, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(run, bin, args(name, dir, rt)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	cmd.Stdin = strings.NewReader(j.Stdin)
	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = maxOutput, maxOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	runErr := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String(), Seconds: time.Since(start).Seconds(), Files: map[string]string{}}
	if run.Err() == context.DeadlineExceeded {
		exec.Command(bin, "kill", name).Run()
		res.TimedOut, res.ExitCode = true, -1
		res.Stderr = strings.TrimSpace(res.Stderr + "\n(stopped after 60 seconds)")
		return res, nil
	}
	if ee := (*exec.ExitError)(nil); errors.As(runErr, &ee) {
		res.ExitCode = ee.ExitCode()
		if res.ExitCode == 125 || res.ExitCode == 126 || res.ExitCode == 127 && strings.Contains(res.Stderr, "docker") {
			return res, fmt.Errorf("the sandbox could not start: %s", strings.TrimSpace(res.Stderr))
		}
	} else if runErr != nil {
		return res, runErr
	}
	outTotal := 0
	filepath.Walk(filepath.Join(dir, "out"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Join(dir, "out"), p)
		body, err := os.ReadFile(p)
		if err != nil || outTotal+len(body) > maxFiles {
			return nil
		}
		outTotal += len(body)
		if utf8.Valid(body) {
			res.Files[filepath.ToSlash(rel)] = string(body)
		} else {
			res.Files[filepath.ToSlash(rel)+";base64"] = base64.StdEncoding.EncodeToString(body)
		}
		return nil
	})
	return res, nil
}

// pull fetches an image the first time it is needed.
func (d Docker) pull(ctx context.Context, bin, image string) error {
	if exec.CommandContext(ctx, bin, "image", "inspect", image).Run() == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "pull", "-q", image).CombinedOutput(); err != nil {
		return fmt.Errorf("could not get the %s image: %s", image, strings.TrimSpace(string(out)))
	}
	return nil
}

type limitedBuffer struct {
	bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room < len(p) {
		if room > 0 {
			b.Buffer.Write(p[:room])
		}
		b.truncated = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func (b *limitedBuffer) String() string {
	if b.truncated {
		return b.Buffer.String() + "\n(output cut at 64 KB)"
	}
	return b.Buffer.String()
}

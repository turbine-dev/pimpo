package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// `pimpo update` replaces this binary with the latest release (or the one
// named), after checking it against the release's signed checksums, and keeps the
// one it replaces next to it as pimpo.previous; `pimpo update --rollback`
// puts that one back. The data is not touched here: the new version takes
// a snapshot the first time it starts, and `pimpo restore` goes back to it.

// releasesAPI and releasesBase can be replaced in tests.
var (
	releasesAPI  = "https://api.github.com/repos/turbine-dev/pimpo/releases"
	releasesBase = "https://github.com/turbine-dev/pimpo/releases/download"
)

func updateCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	want := fs.String("version", "", "install this version (vX.Y.Z) instead of the latest")
	check := fs.Bool("check", false, "only say whether there is a newer version")
	beta := fs.Bool("beta", false, "consider beta versions too")
	rollback := fs.Bool("rollback", false, "put back the binary the last update replaced")
	if err := fs.Parse(args); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	if *rollback {
		return rollBack(exe, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	tag := *want
	if tag == "" {
		if tag, err = latestTag(ctx, *beta); err != nil {
			return err
		}
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	current := "v" + strings.TrimPrefix(version, "v")
	if *want == "" && !newerVersion(tag, current) {
		fmt.Fprintf(out, "Pimpo %s is the latest.\n", strings.TrimPrefix(current, "v"))
		return nil
	}
	if *check {
		fmt.Fprintf(out, "Pimpo %s is available (you have %s). Run: pimpo update\n", strings.TrimPrefix(tag, "v"), strings.TrimPrefix(current, "v"))
		return nil
	}
	fmt.Fprintf(out, "Downloading Pimpo %s…\n", strings.TrimPrefix(tag, "v"))
	bin, err := fetchRelease(ctx, tag)
	if err != nil {
		return err
	}
	if err := swapIn(exe, bin); err != nil {
		return err
	}
	fmt.Fprintf(out, "Updated to %s. The one it replaced is %s.previous (pimpo update --rollback puts it back).\nRestart Pimpo to use it; its first start keeps a snapshot of the data.\n", strings.TrimPrefix(tag, "v"), filepath.Base(exe))
	return nil
}

func latestTag(ctx context.Context, beta bool) (string, error) {
	var releases []struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := getJSON(ctx, releasesAPI+"?per_page=30", &releases); err != nil {
		return "", err
	}
	best := ""
	for _, r := range releases {
		if r.Draft || (r.Prerelease && !beta) || !strings.HasPrefix(r.Tag, "v") {
			continue
		}
		if best == "" || newerVersion(r.Tag, best) {
			best = r.Tag
		}
	}
	if best == "" {
		return "", errors.New("no release found")
	}
	return best, nil
}

// archiveName is the release file for this system, as the installer names it.
func archiveName() string {
	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "armv7"
	}
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("pimpo_windows_%s.zip", arch)
	}
	return fmt.Sprintf("pimpo_%s_%s.tar.gz", runtime.GOOS, arch)
}

// fetchRelease downloads this system's archive, checks the release's
// checksums against the signature made with a release key built into this
// binary, checks the archive against them and returns the binary inside.
func fetchRelease(ctx context.Context, tag string) ([]byte, error) {
	name := archiveName()
	sums, err := get(ctx, releasesBase+"/"+tag+"/checksums.txt", 1<<20)
	if err != nil {
		return nil, fmt.Errorf("could not read the checksums of %s: %w", tag, err)
	}
	// The checksums come from the same place as the archive, so on their
	// own they only catch a broken download: the signature is what says
	// the maintainers made this release.
	sig, err := get(ctx, releasesBase+"/"+tag+"/checksums.txt.sig", 4<<10)
	if err != nil {
		return nil, fmt.Errorf("%s has no signature for its checksums (%v); not installing. Install it by hand from the releases page if you trust it", tag, err)
	}
	if !signedByOneOf(releaseKeys, sums, string(sig)) {
		return nil, fmt.Errorf("the signature of %s's checksums does not match Pimpo's release key; not installing", tag)
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name {
			want = f[0]
		}
	}
	if want == "" {
		return nil, fmt.Errorf("%s has no %s", tag, name)
	}
	archive, err := get(ctx, releasesBase+"/"+tag+"/"+name, 200<<20)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("the checksum of %s does not match; not installing", name)
	}
	bin := "pimpo"
	if runtime.GOOS == "windows" {
		bin = "pimpo.exe"
	}
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == bin {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 300<<20))
			}
		}
		return nil, fmt.Errorf("%s has no %s", name, bin)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("%s has no %s", name, bin)
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == bin {
			return io.ReadAll(io.LimitReader(tr, 300<<20))
		}
	}
}

// swapIn writes the new binary next to the running one and renames it into
// place, keeping the old one as .previous.
func swapIn(exe string, bin []byte) error {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".pimpo-update-")
	if err != nil {
		return fmt.Errorf("cannot write next to %s: %w", exe, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	prev := exe + ".previous"
	os.Remove(prev)
	if err := os.Rename(exe, prev); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		os.Rename(prev, exe)
		return err
	}
	return nil
}

func rollBack(exe string, out io.Writer) error {
	prev := exe + ".previous"
	if _, err := os.Stat(prev); err != nil {
		return errors.New("there is no earlier binary to go back to")
	}
	newer := exe + ".newer"
	os.Remove(newer)
	if err := os.Rename(exe, newer); err != nil {
		return err
	}
	if err := os.Rename(prev, exe); err != nil {
		os.Rename(newer, exe)
		return err
	}
	os.Remove(newer)
	fmt.Fprintln(out, "Put back the earlier binary. To bring the data back as it was before the update too, run `pimpo snapshots` and `pimpo restore NAME` (the snapshot is named before-VERSION).")
	return nil
}

func get(ctx context.Context, url string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Pimpo/"+version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s answered %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s is too large", url)
	}
	return b, err
}

func getJSON(ctx context.Context, url string, v any) error {
	b, err := get(ctx, url, 5<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// newerVersion compares vX.Y.Z[-pre.N]: a release is newer than its
// pre-releases; "dev" and unparsable versions are older than anything.
func newerVersion(a, b string) bool {
	pa, pb := parseVersion(a), parseVersion(b)
	for i := range 3 {
		if pa.n[i] != pb.n[i] {
			return pa.n[i] > pb.n[i]
		}
	}
	switch {
	case pa.pre == "" && pb.pre != "":
		return true
	case pa.pre != "" && pb.pre == "":
		return false
	}
	return comparePre(pa.pre, pb.pre) > 0
}

type semver struct {
	n   [3]int
	pre string
}

func parseVersion(v string) semver {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ := strings.Cut(v, "-")
	var s semver
	s.pre = pre
	for i, p := range strings.SplitN(core, ".", 3) {
		n, err := strconv.Atoi(p)
		if err != nil {
			return semver{n: [3]int{-1, -1, -1}}
		}
		s.n[i] = n
	}
	return s
}

func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, errX := strconv.Atoi(as[i])
		y, errY := strconv.Atoi(bs[i])
		switch {
		case errX == nil && errY == nil && x != y:
			if x > y {
				return 1
			}
			return -1
		case (errX != nil || errY != nil) && as[i] != bs[i]:
			return strings.Compare(as[i], bs[i])
		}
	}
	return len(as) - len(bs)
}

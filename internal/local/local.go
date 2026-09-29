// Package local downloads models that run on this computer: voices for
// reading aloud (with the sherpa-onnx engine they need) and, through
// Ollama, language models. Every file comes from a pinned address with a
// pinned SHA-256, is checked before it is unpacked, and unpacks only
// inside its own folder. Downloads run in the background and report
// their progress, so the app can show them.
package local

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Item is something that can be downloaded.
type Item struct {
	ID string `json:"id"`
	// Kind is engine or voice.
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	About string `json:"about,omitempty"`
	// Languages are those a voice reads (pt-BR, en-US…).
	Languages []string `json:"languages,omitempty"`
	Size      int64    `json:"size"`
	URL       string   `json:"url"`
	SHA256    string   `json:"sha256"`
	// Folder is the top folder inside the archive.
	Folder string `json:"folder"`
	// Voice is how the engine reads with it.
	Voice *VoiceSpec `json:"voice,omitempty"`
	// Files are loose files to fetch instead of an archive (URL, Folder).
	Files []File `json:"files,omitempty"`
	// Transcriber is how the engine turns speech into text with it.
	Transcriber *TranscriberSpec `json:"transcriber,omitempty"`
	// Quality ranks voices of a language: higher reads better.
	Quality int `json:"quality,omitempty"`
}

// File is one file of an item, with its own checksum.
type File struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// TranscriberSpec is how sherpa-onnx loads a speech-to-text model.
type TranscriberSpec struct {
	Type    string `json:"type"` // whisper
	Encoder string `json:"encoder"`
	Decoder string `json:"decoder"`
	Tokens  string `json:"tokens"`
}

// VoiceSpec is how sherpa-onnx loads a voice.
type VoiceSpec struct {
	Type string `json:"type"` // piper or kokoro
	// Model is the model file inside the folder.
	Model string `json:"model"`
	// Speakers are the speaker ids per language, for multi-speaker voices.
	Speakers map[string]int `json:"speakers,omitempty"`
	// Langs are Kokoro's language names per language.
	Langs map[string]string `json:"langs,omitempty"`
}

//go:embed catalog.json
var embedded []byte

// Catalog is what can be downloaded: the voice engine per machine, the
// voices, and language models to suggest for Ollama. It comes from
// catalog.json, built in, or from the owner's own in the models folder.
type Catalog struct {
	About        string          `json:"about,omitempty"`
	Engines      map[string]Item `json:"engines"`
	Voices       []Item          `json:"voices"`
	Transcribers []Item          `json:"transcribers"`
	Suggestions  []Suggestion    `json:"suggestions"`
}

var (
	engines      map[string]Item
	Voices       []Item
	Transcribers []Item
	Suggestions  []Suggestion
)

func init() {
	if err := Use(embedded); err != nil {
		panic("local: built-in catalog.json: " + err.Error())
	}
}

// Use loads a catalog, after checking every entry is complete.
func Use(raw []byte) error {
	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	sized := func(items []Item) {
		for i := range items {
			if len(items[i].Files) > 0 {
				items[i].Size = 0
				for _, f := range items[i].Files {
					items[i].Size += f.Size
				}
			}
		}
	}
	sized(c.Voices)
	sized(c.Transcribers)
	check := func(it Item) error {
		if len(it.Files) > 0 {
			for _, f := range it.Files {
				if f.Name == "" || strings.ContainsAny(f.Name, "/\\") || strings.HasPrefix(f.Name, ".") || f.Size <= 0 || !sha.MatchString(f.SHA256) || !strings.HasPrefix(f.URL, "https://") {
					return fmt.Errorf("entry %q: every file needs a plain name, an https url, size and a sha256", it.ID)
				}
			}
		} else if it.ID == "" || it.URL == "" || it.Folder == "" || it.Size <= 0 || !sha.MatchString(it.SHA256) || !strings.HasPrefix(it.URL, "https://") {
			return fmt.Errorf("entry %q needs id, an https url, folder, size and a sha256", it.ID)
		}
		if strings.ContainsAny(it.ID, "/\\.") {
			return fmt.Errorf("entry id %q may not contain / \\ or .", it.ID)
		}
		return nil
	}
	for _, e := range c.Engines {
		if err := check(e); err != nil {
			return err
		}
	}
	for _, v := range c.Voices {
		if err := check(v); err != nil {
			return err
		}
		if v.Voice == nil || (v.Voice.Type != "piper" && v.Voice.Type != "kokoro") || v.Voice.Model == "" || len(v.Languages) == 0 {
			return fmt.Errorf("voice %q needs languages and a piper or kokoro voice with its model", v.ID)
		}
	}
	for _, tr := range c.Transcribers {
		if err := check(tr); err != nil {
			return err
		}
		if tr.Transcriber == nil || tr.Transcriber.Type != "whisper" || len(tr.Files) == 0 {
			return fmt.Errorf("transcriber %q needs whisper files", tr.ID)
		}
	}
	engines, Voices, Transcribers, Suggestions = c.Engines, c.Voices, c.Transcribers, c.Suggestions
	return nil
}

var sha = regexp.MustCompile(`^[0-9a-f]{64}$`)

// UseFile loads the owner's catalog when there is one.
func UseFile(path string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return Use(raw)
}

// Engine is the voice engine build for this machine, if there is one.
func Engine() (Item, bool) {
	e, ok := engines[runtime.GOOS+"/"+runtime.GOARCH]
	return e, ok
}

// Job is one download.
type Job struct {
	ID string `json:"id"`
	// Item is the catalog id, or ollama:<model>.
	Item  string `json:"item"`
	Name  string `json:"name"`
	State string `json:"state"` // downloading, verifying, unpacking, done, failed, cancelled
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	// Detail is what Ollama says it is doing.
	Detail  string    `json:"detail,omitempty"`
	Error   string    `json:"error,omitempty"`
	Started time.Time `json:"started"`
	cancel  context.CancelFunc
}

// Manager keeps the downloaded models in Dir.
type Manager struct {
	Dir  string
	HTTP *http.Client
	// OllamaURL is where Ollama answers.
	OllamaURL func() string
	// Changed hears when something was installed or removed.
	Changed func()

	mu   sync.Mutex
	jobs map[string]*Job
	seq  int
}

func (m *Manager) client() *http.Client {
	if m.HTTP != nil {
		return m.HTTP
	}
	return &http.Client{}
}

// Jobs lists the downloads of this session, newest first.
func (m *Manager) Jobs() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, *j)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Started.After(out[k].Started) })
	return out
}

func (m *Manager) update(id string, f func(*Job)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobs[id]; ok {
		f(j)
	}
}

// running is the unfinished job for an item, if any.
func (m *Manager) running(item string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Item == item && (j.State == "downloading" || j.State == "verifying" || j.State == "unpacking") {
			return *j, true
		}
	}
	return Job{}, false
}

func (m *Manager) start(item, name string, total int64, work func(ctx context.Context, id string) error) Job {
	if j, ok := m.running(item); ok {
		return j
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	if m.jobs == nil {
		m.jobs = map[string]*Job{}
	}
	m.seq++
	j := &Job{ID: fmt.Sprintf("dl-%d", m.seq), Item: item, Name: name, State: "downloading", Total: total, Started: time.Now(), cancel: cancel}
	m.jobs[j.ID] = j
	first := *j
	m.mu.Unlock()
	go func() {
		defer cancel()
		err := work(ctx, j.ID)
		if err == nil && m.Changed != nil {
			// Whoever waits for "done" finds the change already known.
			m.Changed()
		}
		m.update(j.ID, func(j *Job) {
			switch {
			case errors.Is(ctx.Err(), context.Canceled):
				j.State, j.Error = "cancelled", ""
			case err != nil:
				j.State, j.Error = "failed", err.Error()
			default:
				j.State = "done"
				j.Done = j.Total
			}
		})
	}()
	return first
}

// Cancel stops a download.
func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if ok && j.cancel != nil {
		j.cancel()
	}
	return ok
}

// Find is a catalog item by id, the engine included.
func Find(id string) (Item, bool) {
	if e, ok := Engine(); ok && id == e.ID {
		return e, true
	}
	for _, v := range append(append([]Item{}, Voices...), Transcribers...) {
		if v.ID == id {
			return v, true
		}
	}
	return Item{}, false
}

func (m *Manager) folder(it Item) string { return filepath.Join(m.Dir, it.ID) }

// Installed says whether an item is unpacked and complete.
func (m *Manager) Installed(it Item) bool {
	_, err := os.Stat(filepath.Join(m.folder(it), ".complete"))
	return err == nil
}

// Install downloads an item (and the engine a voice needs first).
func (m *Manager) Install(id string) (Job, error) {
	it, ok := Find(id)
	if !ok {
		return Job{}, fmt.Errorf("%q is not in the catalog", id)
	}
	if m.Installed(it) {
		return Job{}, fmt.Errorf("%s is already installed", it.Name)
	}
	need := it.Size * 3
	var eng Item
	if it.Kind == "voice" || it.Kind == "transcriber" {
		e, ok := Engine()
		if !ok {
			return Job{}, errors.New("local voices and transcription need a Mac; this machine has no engine build")
		}
		if !m.Installed(e) {
			eng = e
			need += e.Size * 3
		}
	}
	if free := FreeBytes(m.Dir); free >= 0 && free < need+2<<30 {
		return Job{}, fmt.Errorf("not enough disk: %s needs about %s and %s are free", it.Name, human(need), human(free))
	}
	total := it.Size + eng.Size
	return m.start(it.ID, it.Name, total, func(ctx context.Context, job string) error {
		var base int64
		if eng.ID != "" {
			if err := m.fetch(ctx, job, eng, 0); err != nil {
				return fmt.Errorf("%s: %w", eng.Name, err)
			}
			base = eng.Size
		}
		return m.fetch(ctx, job, it, base)
	}), nil
}

// fetch downloads, checks and unpacks one item; base is what the job had
// already counted.
func (m *Manager) fetch(ctx context.Context, job string, it Item, base int64) error {
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return err
	}
	if len(it.Files) > 0 {
		return m.fetchFiles(ctx, job, it, base)
	}
	part := filepath.Join(m.Dir, it.ID+".part")
	defer os.Remove(part)
	m.update(job, func(j *Job) { j.State, j.Done = "downloading", base })
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, it.URL, nil)
	if err != nil {
		return err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download answered %d", resp.StatusCode)
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	buf := make([]byte, 256<<10)
	var n int64
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			f.Write(buf[:k])
			h.Write(buf[:k])
			n += int64(k)
			if n > it.Size+1<<20 {
				f.Close()
				return errors.New("the file is larger than it should be")
			}
			done := base + n
			m.update(job, func(j *Job) { j.Done = done })
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return fmt.Errorf("download interrupted: %w", rerr)
		}
	}
	f.Close()
	m.update(job, func(j *Job) { j.State = "verifying" })
	if got := hex.EncodeToString(h.Sum(nil)); got != it.SHA256 {
		return fmt.Errorf("the file does not match its published checksum (got %s…)", got[:12])
	}
	m.update(job, func(j *Job) { j.State = "unpacking" })
	dest := m.folder(it)
	tmp := dest + ".unpacking"
	os.RemoveAll(tmp)
	if err := untarBz2(part, tmp, it.Folder); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, ".complete"), []byte(it.SHA256), 0o600)
}

// untarBz2 unpacks the archive's top folder into dest; nothing may land
// outside it and links may only point inside it.
func untarBz2(archive, dest, top string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(bzip2.NewReader(bufio.NewReaderSize(f, 1<<20)))
	root, _ := filepath.Abs(dest)
	inside := func(p string) bool { return p == root || strings.HasPrefix(p, root+string(os.PathSeparator)) }
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("unreadable archive: %w", err)
		}
		name := filepath.Clean(hdr.Name)
		if rel, ok := strings.CutPrefix(name, top+string(os.PathSeparator)); ok {
			name = rel
		} else if name == top || name == "." {
			continue
		}
		target := filepath.Join(root, name)
		if !inside(target) {
			return fmt.Errorf("the archive tries to write outside its folder: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			mode := os.FileMode(0o644)
			if hdr.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, 2<<30)); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			link := filepath.Join(filepath.Dir(target), hdr.Linkname)
			if filepath.IsAbs(hdr.Linkname) || !inside(filepath.Clean(link)) {
				return fmt.Errorf("the archive links outside its folder: %s", hdr.Name)
			}
			os.MkdirAll(filepath.Dir(target), 0o755)
			os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
}

// Remove deletes an installed item.
func (m *Manager) Remove(id string) error {
	it, ok := Find(id)
	if !ok {
		return fmt.Errorf("%q is not in the catalog", id)
	}
	if err := os.RemoveAll(m.folder(it)); err != nil {
		return err
	}
	if m.Changed != nil {
		m.Changed()
	}
	return nil
}

func human(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", n>>10)
}

var modelName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,100}(:[a-zA-Z0-9._-]{1,60})?$`)

// Pull asks Ollama to download a model, following its progress.
func (m *Manager) Pull(model string) (Job, error) {
	model = strings.TrimSpace(model)
	if !modelName.MatchString(model) {
		return Job{}, fmt.Errorf("%q is not a model name (like qwen3:4b)", model)
	}
	base := ""
	if m.OllamaURL != nil {
		base = strings.TrimRight(m.OllamaURL(), "/")
	}
	if base == "" {
		return Job{}, errors.New("Ollama is not running")
	}
	return m.start("ollama:"+model, model, 0, func(ctx context.Context, job string) error {
		body, _ := json.Marshal(map[string]any{"model": model, "stream": true})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/pull", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := m.client().Do(req)
		if err != nil {
			return fmt.Errorf("Ollama is not answering: %w", err)
		}
		defer resp.Body.Close()
		// Ollama downloads a model in layers; the progress is the sum.
		layers := map[string][2]int64{}
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var e struct {
				Status    string `json:"status"`
				Digest    string `json:"digest"`
				Total     int64  `json:"total"`
				Completed int64  `json:"completed"`
				Error     string `json:"error"`
			}
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				continue
			}
			if e.Error != "" {
				return errors.New(e.Error)
			}
			if e.Digest != "" && e.Total > 0 {
				layers[e.Digest] = [2]int64{e.Completed, e.Total}
			}
			var done, total int64
			for _, l := range layers {
				done += l[0]
				total += l[1]
			}
			m.update(job, func(j *Job) {
				j.Detail, j.Done, j.Total = e.Status, done, total
				if strings.HasPrefix(e.Status, "verifying") {
					j.State = "verifying"
				}
			})
			if e.Status == "success" {
				return nil
			}
		}
		if err := sc.Err(); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("Ollama answered %d", resp.StatusCode)
		}
		return errors.New("Ollama stopped before the model was complete")
	}), nil
}

// DeleteOllama removes a model from Ollama.
func (m *Manager) DeleteOllama(ctx context.Context, model string) error {
	base := strings.TrimRight(m.OllamaURL(), "/")
	body, _ := json.Marshal(map[string]string{"model": model})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client().Do(req)
	if err != nil {
		return fmt.Errorf("Ollama is not answering: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Ollama answered %d", resp.StatusCode)
	}
	return nil
}

// Suggestion is a language model to download with Ollama.
type Suggestion struct {
	Model string `json:"model"`
	About string `json:"about"`
	Size  int64  `json:"size"`
	// MinRAM is the memory it needs to run well.
	MinRAM int64 `json:"min_ram"`
}

// VoiceFor is the best installed voice for a language, then for its
// family (pt for pt-BR).
func (m *Manager) VoiceFor(lang string) (Item, bool) {
	e, ok := Engine()
	if !ok || !m.Installed(e) {
		return Item{}, false
	}
	family, _, _ := strings.Cut(lang, "-")
	var best Item
	score := -1
	for _, v := range Voices {
		if !m.Installed(v) {
			continue
		}
		for _, l := range v.Languages {
			s := -1
			switch {
			case strings.EqualFold(l, lang):
				s = 100 + v.Quality
			case strings.EqualFold(strings.SplitN(l, "-", 2)[0], family):
				s = 50 + v.Quality
			}
			if s > score {
				best, score = v, s
			}
		}
	}
	return best, score >= 0
}

// Synthesize reads text with a voice into a WAV file.
func (m *Manager) Synthesize(ctx context.Context, v Item, lang, text, wav string) error {
	e, _ := Engine()
	bin := filepath.Join(m.folder(e), "bin", "sherpa-onnx-offline-tts")
	dir := m.folder(v)
	args := []string{"--num-threads=4", "--output-filename=" + wav}
	switch v.Voice.Type {
	case "piper":
		args = append(args, "--vits-model="+filepath.Join(dir, v.Voice.Model), "--vits-tokens="+filepath.Join(dir, "tokens.txt"), "--vits-data-dir="+filepath.Join(dir, "espeak-ng-data"))
	case "kokoro":
		use := lang
		if _, ok := v.Voice.Speakers[use]; !ok {
			family, _, _ := strings.Cut(lang, "-")
			for l := range v.Voice.Speakers {
				if strings.HasPrefix(l, family) {
					use = l
					break
				}
			}
		}
		args = append(args, "--kokoro-model="+filepath.Join(dir, v.Voice.Model), "--kokoro-voices="+filepath.Join(dir, "voices.bin"),
			"--kokoro-tokens="+filepath.Join(dir, "tokens.txt"), "--kokoro-data-dir="+filepath.Join(dir, "espeak-ng-data"),
			"--kokoro-lexicon="+filepath.Join(dir, "lexicon-us-en.txt"), "--kokoro-lang="+v.Voice.Langs[use], fmt.Sprintf("--sid=%d", v.Voice.Speakers[use]))
	default:
		return fmt.Errorf("unknown voice type %q", v.Voice.Type)
	}
	cmd := execCommand(ctx, bin, append(args, text)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v %s", v.Name, err, lastLine(stderr.String()))
	}
	if st, err := os.Stat(wav); err != nil || st.Size() < 1000 {
		return fmt.Errorf("%s wrote no audio", v.Name)
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

var execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// fetchFiles downloads an item made of loose files, each checked, into its
// folder.
func (m *Manager) fetchFiles(ctx context.Context, job string, it Item, base int64) error {
	dest := m.folder(it)
	tmp := dest + ".unpacking"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	for _, f := range it.Files {
		if err := m.download(ctx, job, f.URL, filepath.Join(tmp, f.Name), f.Size, f.SHA256, base); err != nil {
			os.RemoveAll(tmp)
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		base += f.Size
	}
	os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, ".complete"), []byte(it.ID), 0o600)
}

// download fetches one file to path, checking its size and checksum.
func (m *Manager) download(ctx context.Context, job, url, path string, size int64, want string, base int64) error {
	m.update(job, func(j *Job) { j.State = "downloading" })
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download answered %d", resp.StatusCode)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 256<<10)
	var n int64
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			f.Write(buf[:k])
			h.Write(buf[:k])
			n += int64(k)
			if n > size+1<<20 {
				return errors.New("the file is larger than it should be")
			}
			done := base + n
			m.update(job, func(j *Job) { j.Done = done })
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("download interrupted: %w", rerr)
		}
	}
	m.update(job, func(j *Job) { j.State = "verifying" })
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("the file does not match its published checksum (got %s…)", got[:12])
	}
	return nil
}

// Transcriber is the best installed speech-to-text model.
func (m *Manager) Transcriber() (Item, bool) {
	e, ok := Engine()
	if !ok || !m.Installed(e) {
		return Item{}, false
	}
	var best Item
	found := false
	for _, t := range Transcribers {
		if m.Installed(t) && (!found || t.Quality > best.Quality) {
			best, found = t, true
		}
	}
	return best, found
}

// Transcribe turns a 16 kHz mono WAV into text; lang is a language code
// (pt, en) or "" to detect it.
func (m *Manager) Transcribe(ctx context.Context, t Item, wav, lang string) (string, error) {
	e, _ := Engine()
	bin := filepath.Join(m.folder(e), "bin", "sherpa-onnx-offline")
	dir := m.folder(t)
	sp := t.Transcriber
	args := []string{"--whisper-encoder=" + filepath.Join(dir, sp.Encoder), "--whisper-decoder=" + filepath.Join(dir, sp.Decoder),
		"--tokens=" + filepath.Join(dir, sp.Tokens), "--whisper-task=transcribe", "--num-threads=4"}
	if lang != "" {
		args = append(args, "--whisper-language="+lang)
	}
	out, err := execCommand(ctx, bin, append(args, wav)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %v %s", t.Name, err, lastLine(string(out)))
	}
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "{") {
			continue
		}
		var r struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(l), &r) == nil {
			if text := strings.TrimSpace(r.Text); text != "" {
				return text, nil
			}
		}
	}
	return "", errors.New("no words were made out")
}

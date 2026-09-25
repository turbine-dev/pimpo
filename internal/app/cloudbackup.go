package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/denerFernandes/zodim/internal/backup"
	"github.com/denerFernandes/zodim/internal/cloud"
	"github.com/denerFernandes/zodim/internal/explore"
	"github.com/denerFernandes/zodim/internal/oauth"
	"github.com/denerFernandes/zodim/internal/server"
)

// Automatic backups to storage the owner already has. Each one is the
// full export, encrypted as a whole with the owner's passphrase before it
// leaves this machine.

type cloudConfig struct {
	Kind     string `json:"kind"` // "", "s3" or "drive"
	Every    string `json:"every"`
	Keep     int    `json:"keep"`
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region,omitempty"`
	Bucket   string `json:"bucket,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
}

type cloudRun struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Name  string    `json:"name,omitempty"`
	Size  int       `json:"size,omitempty"`
	Error string    `json:"error,omitempty"`
}

const (
	cloudKey     = "backup.cloud"
	cloudLastKey = "backup.cloud.last"
	cloudOKKey   = "backup.cloud.ok"
)

var cloudName = regexp.MustCompile(`^zodim-\d{8}-\d{6}\.zodim$`)

// cloudStore lets tests point S3 and Drive at fakes.
var cloudHTTP *http.Client
var driveAPI, driveUpload string

func (a *App) cloudConfig(ctx context.Context) cloudConfig {
	c := cloudConfig{Every: "daily", Keep: 7}
	if raw, _ := a.Events.Get(ctx, cloudKey); raw != "" {
		json.Unmarshal([]byte(raw), &c)
	}
	return c
}

func (a *App) vaultValue(ctx context.Context, name string) string {
	v, _ := a.secret(ctx, name)
	return v
}

func (a *App) cloudStore(ctx context.Context, c cloudConfig) (cloud.Store, error) {
	switch c.Kind {
	case "s3":
		return &cloud.S3{Endpoint: c.Endpoint, Region: c.Region, Bucket: c.Bucket, Prefix: c.Prefix,
			AccessKey: a.vaultValue(ctx, "backup.s3.access"), SecretKey: a.vaultValue(ctx, "backup.s3.secret"), HTTP: cloudHTTP}, nil
	case "drive":
		if a.vaultValue(ctx, "google.refresh") == "" {
			return nil, errors.New("connect Google in Connections first")
		}
		if !a.Google.Granted(ctx, oauth.DriveScope) {
			return nil, errors.New("Zodim may not use your Drive yet: reconnect Google in Connections and allow Drive")
		}
		return &cloud.Drive{Token: a.Google.Token, API: driveAPI, Upload: driveUpload, HTTP: cloudHTTP}, nil
	}
	return nil, errors.New("cloud backups are off")
}

func (c cloudConfig) interval() time.Duration {
	if c.Every == "weekly" {
		return 7 * 24 * time.Hour
	}
	return 24 * time.Hour
}

// backupToCloud exports, seals, uploads and keeps only the newest copies.
func (a *App) backupToCloud(ctx context.Context) cloudRun {
	run := cloudRun{At: time.Now()}
	err := func() error {
		c := a.cloudConfig(ctx)
		store, err := a.cloudStore(ctx, c)
		if err != nil {
			return err
		}
		pass := a.vaultValue(ctx, "backup.passphrase")
		var buf bytes.Buffer
		if _, err := backup.Export(ctx, a.Events.DB(), a.Home, a.Vault, pass, a.Version, &buf); err != nil {
			return err
		}
		sealed, err := backup.Seal(buf.Bytes(), pass)
		if err != nil {
			return err
		}
		run.Name, run.Size = "zodim-"+run.At.UTC().Format("20060102-150405")+".zodim", len(sealed)
		if err := store.Put(ctx, run.Name, sealed); err != nil {
			return err
		}
		list, err := store.List(ctx)
		if err != nil {
			return nil
		}
		var ours []string
		for _, o := range list {
			if cloudName.MatchString(o.Name) {
				ours = append(ours, o.Name)
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(ours)))
		for i, n := range ours {
			if i >= max(c.Keep, 1) {
				store.Delete(ctx, n)
			}
		}
		return nil
	}()
	run.OK = err == nil
	if err != nil {
		run.Error = err.Error()
	}
	b, _ := json.Marshal(run)
	a.Events.Put(ctx, cloudLastKey, string(b))
	if run.OK {
		a.Events.Put(ctx, cloudOKKey, run.At.Format(time.RFC3339))
		a.Events.Append(ctx, "backup.uploaded", "system", map[string]any{"name": run.Name, "size": run.Size})
	} else {
		a.Events.Append(ctx, "backup.failed", "system", map[string]string{"error": run.Error})
	}
	return run
}

func (a *App) lastCloudRun(ctx context.Context) (cloudRun, bool) {
	var r cloudRun
	raw, _ := a.Events.Get(ctx, cloudLastKey)
	return r, raw != "" && json.Unmarshal([]byte(raw), &r) == nil
}

// cloudDue reports whether a backup should run now: one interval after the
// last success, and after a failure at most once an hour.
func (a *App) cloudDue(ctx context.Context, now time.Time) bool {
	c := a.cloudConfig(ctx)
	if c.Kind == "" {
		return false
	}
	if last, ok := a.lastCloudRun(ctx); ok && !last.OK && now.Sub(last.At) < time.Hour {
		return false
	}
	raw, _ := a.Events.Get(ctx, cloudOKKey)
	ok, err := time.Parse(time.RFC3339, raw)
	return err != nil || now.Sub(ok) >= c.interval()
}

func (a *App) cloudLoop(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if a.cloudDue(ctx, time.Now()) {
			before, _ := a.lastCloudRun(ctx)
			if run := a.backupToCloud(ctx); !run.OK && (before.OK || before.At.IsZero()) {
				a.Channel.Notify(ctx, explore.Notice{Text: "O backup na nuvem falhou\n" + run.Error})
			}
		}
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) cloudRoutes() {
	a.Server.Handle("GET /api/backup/cloud", a.getCloud)
	a.Server.Handle("PUT /api/backup/cloud", a.putCloud)
	a.Server.Handle("DELETE /api/backup/cloud", a.offCloud)
	a.Server.Handle("POST /api/backup/cloud/run", a.runCloud)
	a.Server.Handle("GET /api/backup/cloud/files", a.cloudFiles)
	a.Server.Handle("POST /api/backup/cloud/restore", a.restoreCloud)
}

func (a *App) cloudView(ctx context.Context) map[string]any {
	c := a.cloudConfig(ctx)
	out := map[string]any{
		"config":         c,
		"has_keys":       a.vaultValue(ctx, "backup.s3.access") != "" && a.vaultValue(ctx, "backup.s3.secret") != "",
		"has_passphrase": a.vaultValue(ctx, "backup.passphrase") != "",
		"google":         map[string]bool{"connected": a.vaultValue(ctx, "google.refresh") != "", "drive": a.Google.Granted(ctx, oauth.DriveScope)},
	}
	if last, ok := a.lastCloudRun(ctx); ok {
		out["last"] = last
	}
	if raw, _ := a.Events.Get(ctx, cloudOKKey); raw != "" && c.Kind != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			out["next"] = t.Add(c.interval())
		}
	}
	return out
}

func (a *App) getCloud(w http.ResponseWriter, r *http.Request) {
	server.WriteJSON(w, 200, a.cloudView(r.Context()))
}

// putCloud saves the settings after checking the storage answers; keys
// and passphrase go to the vault and are never sent back.
func (a *App) putCloud(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		cloudConfig
		AccessKey  string `json:"access_key"`
		SecretKey  string `json:"secret_key"`
		Passphrase string `json:"passphrase"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	bad := func(msg string) { server.WriteError(w, server.StatusError{Status: 400, Msg: msg}) }
	c := req.cloudConfig
	if c.Kind != "s3" && c.Kind != "drive" {
		bad("choose Amazon S3 (or compatible) or Google Drive")
		return
	}
	if c.Every != "weekly" {
		c.Every = "daily"
	}
	c.Keep = min(max(c.Keep, 1), 90)
	if c.Kind == "s3" {
		c.Bucket, c.Endpoint, c.Region = strings.TrimSpace(c.Bucket), strings.TrimSpace(c.Endpoint), strings.TrimSpace(c.Region)
		if c.Bucket == "" {
			bad("the bucket name is missing")
			return
		}
		if (req.AccessKey == "" || req.SecretKey == "") && a.vaultValue(ctx, "backup.s3.access") == "" {
			bad("the access key and secret are missing")
			return
		}
	} else {
		c.Endpoint, c.Region, c.Bucket, c.Prefix = "", "", "", ""
	}
	if req.Passphrase != "" && len(req.Passphrase) < 8 {
		bad("choose a passphrase of at least 8 characters")
		return
	}
	if req.Passphrase == "" && a.vaultValue(ctx, "backup.passphrase") == "" {
		bad("choose a passphrase: without it nobody, not even you, can open the backups")
		return
	}
	// Check with the new keys before keeping anything.
	store, err := a.cloudStore(ctx, c)
	if s3, ok := store.(*cloud.S3); ok && req.AccessKey != "" {
		s3.AccessKey, s3.SecretKey = strings.TrimSpace(req.AccessKey), strings.TrimSpace(req.SecretKey)
	}
	if err == nil {
		_, err = store.List(ctx)
	}
	if err != nil {
		bad(err.Error())
		return
	}
	if req.AccessKey != "" && c.Kind == "s3" {
		a.Vault.Set(ctx, "backup.s3.access", strings.TrimSpace(req.AccessKey))
		a.Vault.Set(ctx, "backup.s3.secret", strings.TrimSpace(req.SecretKey))
	}
	if req.Passphrase != "" {
		a.Vault.Set(ctx, "backup.passphrase", req.Passphrase)
	}
	b, _ := json.Marshal(c)
	a.Events.Put(ctx, cloudKey, string(b))
	a.Events.Append(ctx, "backup.cloud.changed", "human:owner", map[string]string{"kind": c.Kind, "every": c.Every})
	server.WriteJSON(w, 200, a.cloudView(ctx))
}

func (a *App) offCloud(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := a.cloudConfig(ctx)
	c.Kind = ""
	b, _ := json.Marshal(c)
	a.Events.Put(ctx, cloudKey, string(b))
	a.Events.Append(ctx, "backup.cloud.changed", "human:owner", map[string]string{"kind": "off"})
	server.WriteJSON(w, 200, a.cloudView(ctx))
}

func (a *App) runCloud(w http.ResponseWriter, r *http.Request) {
	run := a.backupToCloud(r.Context())
	if !run.OK {
		server.WriteError(w, server.StatusError{Status: 502, Msg: run.Error})
		return
	}
	server.WriteJSON(w, 200, run)
}

func (a *App) cloudFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store, err := a.cloudStore(ctx, a.cloudConfig(ctx))
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	list, err := store.List(ctx)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 502, Msg: err.Error()})
		return
	}
	out := []cloud.Object{}
	for _, o := range list {
		if cloudName.MatchString(o.Name) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	server.WriteJSON(w, 200, out)
}

// restoreCloud downloads one backup and stages it like an imported file.
// The passphrase saved here is tried when none is given, which covers
// going back on the same machine.
func (a *App) restoreCloud(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Name       string `json:"name"`
		Passphrase string `json:"passphrase"`
	}
	if err := server.Decode(r, &req); err != nil {
		server.WriteError(w, err)
		return
	}
	if !cloudName.MatchString(req.Name) {
		server.WriteError(w, server.StatusError{Status: 400, Msg: "not a Zodim backup"})
		return
	}
	store, err := a.cloudStore(ctx, a.cloudConfig(ctx))
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	data, err := store.Get(ctx, req.Name)
	if err != nil {
		server.WriteError(w, server.StatusError{Status: 502, Msg: err.Error()})
		return
	}
	pass := req.Passphrase
	if pass == "" {
		pass = a.vaultValue(ctx, "backup.passphrase")
	}
	a.stageImport(w, r, bytes.NewReader(data), pass)
}

// stageImport checks and stages a backup; it takes effect when Zodim
// restarts, since the database cannot be swapped while it is open.
func (a *App) stageImport(w http.ResponseWriter, r *http.Request, file io.Reader, pass string) {
	if a.Home == "" {
		server.WriteError(w, server.StatusError{Status: 503, Msg: "import is not available here"})
		return
	}
	stage := filepath.Join(a.Home, "import-pending")
	os.RemoveAll(stage)
	m, secrets, err := backup.Unpack(file, stage, pass)
	if err != nil {
		os.RemoveAll(stage)
		server.WriteError(w, server.StatusError{Status: 400, Msg: err.Error()})
		return
	}
	b, _ := json.Marshal(secrets)
	if err := os.WriteFile(filepath.Join(stage, "secrets.json"), b, 0o600); err != nil {
		server.WriteError(w, err)
		return
	}
	a.Events.Append(r.Context(), "backup.staged", "human:owner", map[string]any{"created": m.Created, "secrets": m.Secrets})
	server.WriteJSON(w, 200, map[string]any{"created": m.Created, "version": m.Version, "secrets": m.Secrets, "restart": true})
}

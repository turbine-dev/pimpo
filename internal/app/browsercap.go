package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/turbine-dev/pimpo/internal/browser"
	"github.com/turbine-dev/pimpo/internal/connector"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/netguard"
	"github.com/turbine-dev/pimpo/internal/server"
)

// browserCap drives Pimpo's own browser (docs/rfcs/0002-browser.md), only
// after the owner turned it on in Laboratório. browser.open is scoped by
// host like web.read; the other actions work on pages of the hosts this
// run has opened, and any page that leads elsewhere is refused.
type browserCap struct{ a *App }

func (browserCap) Capabilities() []string {
	return []string{"browser.open", "browser.read", "browser.follow", "browser.type", "browser.choose", "browser.click"}
}

var runHosts = struct {
	sync.Mutex
	m map[string]map[string]bool
}{m: map[string]map[string]bool{}}

func allowOnRun(run, h string) {
	runHosts.Lock()
	defer runHosts.Unlock()
	if runHosts.m[run] == nil {
		if len(runHosts.m) > 500 {
			clear(runHosts.m)
		}
		runHosts.m[run] = map[string]bool{}
	}
	runHosts.m[run][strings.ToLower(h)] = true
}

func runAllows(run string) browser.Allowed {
	return func(h string) bool {
		runHosts.Lock()
		defer runHosts.Unlock()
		return runHosts.m[run][strings.ToLower(h)]
	}
}

func (a *App) theBrowser() *browser.Browser {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.browser == nil {
		a.browser = &browser.Browser{Profile: filepath.Join(a.Home, "browser", "profile")}
	}
	return a.browser
}

func (c browserCap) Call(ctx context.Context, name, scope string, args any) (any, error) {
	if !c.a.chose(ctx, "browser") {
		return nil, errors.New("the browser is off: the owner can turn it on in Ajustes › Laboratório (it needs Chrome)")
	}
	if c.a.Home == "" {
		return nil, errors.New("the browser needs a data folder")
	}
	var in struct {
		URL   string `json:"url"`
		Ref   int    `json:"ref"`
		Text  string `json:"text"`
		Value string `json:"value"`
	}
	if err := connector.Args(args, &in); err != nil {
		return nil, err
	}
	run := host.SourceOf(ctx)
	if run == "" {
		run = "chat"
	}
	b := c.a.theBrowser()
	allowed := runAllows(run)
	switch name {
	case "browser.open":
		u, err := netguard.ParseURL(in.URL)
		if err != nil {
			return nil, errors.New("give the page's full http(s) address, without a user name")
		}
		// The policy checked this host (the manifest's scope, or the
		// owner's answer while exploring); only then may the run reach it.
		if !netguard.SameHost(u, scope) {
			return nil, errors.New(u.Hostname() + " is outside the sites this may reach")
		}
		allowOnRun(run, u.Hostname())
		return b.Open(ctx, run, in.URL, allowed)
	}
	runHosts.Lock()
	opened := len(runHosts.m[run]) > 0
	runHosts.Unlock()
	if !opened {
		return nil, errors.New("open a page with browser.open first")
	}
	switch name {
	case "browser.read":
		return b.Read(ctx, run, allowed)
	case "browser.follow":
		return b.Follow(ctx, run, in.Ref, allowed)
	case "browser.type":
		return b.Type(ctx, run, in.Ref, in.Text, allowed)
	case "browser.choose":
		return b.Choose(ctx, run, in.Ref, in.Value, allowed)
	case "browser.click":
		return b.Click(ctx, run, in.Ref, allowed)
	}
	return nil, errors.New("unknown browser action " + name)
}

// chromePath finds Chrome (or Chromium) for the window the owner signs in
// with.
func chromePath() string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Chromium.app/Contents/MacOS/Chromium"}
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			candidates = append(candidates, filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
		}
	default:
		for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
			if p, err := exec.LookPath(n); err == nil {
				return p
			}
		}
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (a *App) browserRoutes() {
	// login opens a visible window of Pimpo's browser profile, for the
	// owner to sign in to sites by hand; routines then use those sessions.
	a.Server.Handle("POST /api/browser/login", func(w http.ResponseWriter, r *http.Request) {
		if !ownerOnly(w, r) {
			return
		}
		if !a.chose(r.Context(), "browser") {
			server.WriteError(w, server.StatusError{Status: 409, Msg: "turn the browser on in Laboratório first"})
			return
		}
		var req struct {
			URL string `json:"url"`
		}
		server.Decode(r, &req)
		start := "about:blank"
		if u, err := url.Parse(req.URL); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			start = u.String()
		}
		chrome := chromePath()
		if chrome == "" {
			server.WriteError(w, server.StatusError{Status: 404, Msg: "Chrome is not installed"})
			return
		}
		// The headless browser holds the profile; it restarts on the next call.
		a.theBrowser().Close()
		profile := filepath.Join(a.Home, "browser", "profile")
		os.MkdirAll(profile, 0o700)
		cmd := exec.Command(chrome, "--user-data-dir="+profile, "--no-first-run", "--new-window", start)
		if err := cmd.Start(); err != nil {
			server.WriteError(w, err)
			return
		}
		go cmd.Wait()
		a.Events.Append(r.Context(), "browser.login", actor(r.Context()), map[string]string{"url": start})
		server.WriteJSON(w, 200, map[string]string{"state": "opened"})
	})
}

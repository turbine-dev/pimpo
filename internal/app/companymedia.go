package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/media"
	"github.com/turbine-dev/pimpo/internal/netguard"
	"github.com/turbine-dev/pimpo/internal/people"
	"github.com/turbine-dev/pimpo/internal/server"
)

// A content member makes videos on this computer: a voice from the local
// voices, screenshots through its own browser, and scenes put together
// with ffmpeg in fixed formats. Each video is checked without a model
// before anyone is asked about it.

func init() {
	for _, s := range []capability.Spec{
		{Name: "media.voice", Risk: capability.Reversible, Signature: "media.voice({text, language})", Returns: "{media, seconds}; the text read by the voice the person chose, kept as a file of the company",
			Schema: `{"type":"object","properties":{"text":{"type":"string"},"language":{"type":"string"}},"required":["text"]}`},
		{Name: "media.capture", Risk: capability.Reversible, Signature: "media.capture({url, format})", Returns: "{media}; a screenshot of the page in your own browser, sized for the format (tutorial or short); only hosts the rules allow",
			Scoped: true, ScopeArg: "url", Schema: `{"type":"object","properties":{"url":{"type":"string"},"format":{"type":"string","enum":["tutorial","short"]}},"required":["url"]}`},
		{Name: "media.render", Risk: capability.Reversible, Signature: "media.render({format, title, scenes})",
			Returns: "{media, seconds, check}; a video in the format (tutorial 16:9 up to 20 minutes, short 9:16 up to 60 s) of scenes [{image: media, voice: media, text, seconds}], each caption shown while its scene plays; checked before anyone sees it",
			Schema:  `{"type":"object","properties":{"format":{"type":"string","enum":["tutorial","short"]},"title":{"type":"string"},"scenes":{"type":"array","items":{"type":"object","properties":{"image":{"type":"string"},"voice":{"type":"string"},"text":{"type":"string"},"seconds":{"type":"number"}}}}},"required":["format","scenes"]}`},
		{Name: "media.check", Risk: capability.Read, Signature: "media.check({media})", Returns: "{format, seconds, width, height, lufs, captions, problems}; length, aspect, loudness and captions against the video's format",
			Schema: `{"type":"object","properties":{"media":{"type":"string"}},"required":["media"]}`},
	} {
		capability.Register(s)
	}
}

type mediaCap struct{ a *App }

func (mediaCap) Capabilities() []string {
	return []string{"media.voice", "media.capture", "media.render", "media.check"}
}

func newMediaID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return "m_" + hex.EncodeToString(b)
}

// companyMediaDir is where a company's media files are kept.
func (a *App) companyMediaDir(co string) (string, error) {
	spaces, err := a.spaces()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(spaces.Root, co, "media")
	return dir, os.MkdirAll(dir, 0o700)
}

// mediaPath is a company's media file by id, only if the company has it.
func (a *App) mediaPath(ctx context.Context, co, id string, kinds ...string) (company.Media, string, error) {
	if !company.MediaID.MatchString(id) {
		return company.Media{}, "", fmt.Errorf("%q is not a media id", id)
	}
	m, err := a.Companies.MediaItem(ctx, co, id)
	if err != nil {
		return m, "", fmt.Errorf("the company has no media %s", id)
	}
	if len(kinds) > 0 && !containsAny(kinds, m.Kind) {
		return m, "", fmt.Errorf("%s is %s, not %s", id, m.Kind, strings.Join(kinds, " or "))
	}
	dir, err := a.companyMediaDir(co)
	return m, filepath.Join(dir, m.File), err
}

func containsAny(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (c mediaCap) Call(ctx context.Context, name, scope string, args any) (any, error) {
	a := c.a
	o, me, work, err := a.caller(ctx)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(args)
	dir, err := a.companyMediaDir(o.ID)
	if err != nil {
		return nil, err
	}
	keep := func(m company.Media, data []byte) (company.Media, error) {
		m.Company, m.Member, m.Work, m.Created = o.ID, me, work.ID, time.Now().UTC()
		if data != nil {
			if err := os.WriteFile(filepath.Join(dir, m.File), data, 0o600); err != nil {
				return m, err
			}
		}
		return m, a.Companies.SaveMedia(ctx, m)
	}
	switch name {
	case "media.voice":
		var in struct{ Text, Language string }
		json.Unmarshal(b, &in)
		if strings.TrimSpace(in.Text) == "" {
			return nil, errors.New("say what to read")
		}
		audio, secs, _, err := a.speak(people.With(ctx, o.Person), in.Text, in.Language)
		if err != nil {
			return nil, err
		}
		id := newMediaID()
		m, err := keep(company.Media{ID: id, Kind: company.MediaAudio, File: id + ".m4a", Title: clip(firstLine(in.Text), 120), Seconds: secs}, audio)
		if err != nil {
			return nil, err
		}
		return map[string]any{"media": m.ID, "seconds": secs}, nil
	case "media.capture":
		if !a.chose(ctx, "browser") {
			return nil, errors.New("the browser is off: the owner can turn it on in Settings › Labs (it needs Chrome)")
		}
		var in struct{ URL, Format string }
		json.Unmarshal(b, &in)
		f, ok := media.Formats[in.Format]
		if !ok {
			f = media.Formats["tutorial"]
		}
		u, err := netguard.ParseURL(in.URL)
		if err != nil {
			return nil, errors.New("give the page's full http(s) address, without a user name")
		}
		if !netguard.SameHost(u, scope) {
			return nil, errors.New(u.Hostname() + " is outside the sites this may reach")
		}
		run := host.SourceOf(ctx)
		allowOnRun(run, u.Hostname())
		png, err := a.memberBrowser(o.ID+"/"+me).Screenshot(ctx, run, in.URL, f.Width, f.Height, runAllows(run))
		if err != nil {
			return nil, err
		}
		id := newMediaID()
		m, err := keep(company.Media{ID: id, Kind: company.MediaImage, File: id + ".png", Title: u.Host + u.Path, Format: f.Name}, png)
		if err != nil {
			return nil, err
		}
		return map[string]any{"media": m.ID}, nil
	case "media.render":
		var in struct {
			Format string `json:"format"`
			Title  string `json:"title"`
			Scenes []struct {
				Image, Voice, Text string
				Seconds            float64
			} `json:"scenes"`
		}
		json.Unmarshal(b, &in)
		f, ok := media.Formats[in.Format]
		if !ok {
			return nil, errors.New("the format is tutorial or short")
		}
		var scenes []media.Scene
		for _, sc := range in.Scenes {
			s := media.Scene{Text: clip(sc.Text, 300), Seconds: sc.Seconds}
			if sc.Image != "" {
				if _, s.Image, err = a.mediaPath(ctx, o.ID, sc.Image, company.MediaImage); err != nil {
					return nil, err
				}
			}
			if sc.Voice != "" {
				if _, s.Audio, err = a.mediaPath(ctx, o.ID, sc.Voice, company.MediaAudio); err != nil {
					return nil, err
				}
			}
			scenes = append(scenes, s)
		}
		id := newMediaID()
		out := filepath.Join(dir, id+".mp4")
		secs, err := media.Render(ctx, f, scenes, out)
		if err != nil {
			os.Remove(out)
			return nil, err
		}
		report, err := media.Check(ctx, out, f)
		if err != nil {
			return nil, err
		}
		if _, err := keep(company.Media{ID: id, Kind: company.MediaVideo, File: id + ".mp4", Title: clip(in.Title, 200), Format: f.Name, Seconds: secs, Check: report}, nil); err != nil {
			return nil, err
		}
		a.Events.Append(ctx, "company.media.rendered", actorFor(o, me), map[string]any{"company": o.ID, "media": id, "format": f.Name, "ok": report.OK(), "person": o.Person})
		return map[string]any{"media": id, "seconds": secs, "check": report}, nil
	case "media.check":
		var in struct{ Media string }
		json.Unmarshal(b, &in)
		m, path, err := a.mediaPath(ctx, o.ID, in.Media, company.MediaVideo)
		if err != nil {
			return nil, err
		}
		return media.Check(ctx, path, media.Formats[m.Format])
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

var mediaTypes = map[string]string{company.MediaAudio: "audio/mp4", company.MediaImage: "image/png", company.MediaVideo: "video/mp4"}

func (a *App) companyMediaRoutes() {
	a.Server.Handle("GET /api/companies/{id}/media", a.companyRoute(company.View, func(w http.ResponseWriter, r *http.Request, o company.Org) (any, error) {
		return a.Companies.MediaList(r.Context(), o.ID)
	}))
	a.Server.Handle("GET /api/companies/{id}/media/{part}", a.serveMedia)
}

func (a *App) serveMedia(w http.ResponseWriter, r *http.Request) {
	if !a.companiesOn(w, r) {
		return
	}
	o, err := a.myCompany(r.Context(), r.PathValue("id"), company.View)
	if err != nil {
		server.WriteError(w, companyError(err))
		return
	}
	m, path, err := a.mediaPath(r.Context(), o.ID, r.PathValue("part"))
	if err != nil {
		server.WriteError(w, companyError(company.ErrNotFound))
		return
	}
	f, err := os.Open(path)
	if err != nil {
		server.WriteError(w, companyError(company.ErrNotFound))
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", mediaTypes[m.Kind])
	http.ServeContent(w, r, m.File, st.ModTime(), f)
}

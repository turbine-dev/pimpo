package app

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/media"
	"github.com/turbine-dev/pimpo/internal/workspace"
)

func TestAMemberMakesAVideoThatIsCheckedFirst(t *testing.T) {
	ta, co := clerk(t)
	ta.Workspaces = workspace.Spaces{Root: t.TempDir()}
	ctx := context.Background()
	mc := mediaCap{ta.App}
	clara := asMember(co+"/clara", "media.render")
	if _, err := mc.Call(clara, "media.render", "", map[string]any{"format": "square", "scenes": []map[string]any{{"text": "x"}}}); err == nil {
		t.Fatal("a format that does not exist")
	}
	if _, err := mc.Call(clara, "media.render", "", map[string]any{"format": "short", "scenes": []map[string]any{{"image": "m_000000000000"}}}); err == nil || !strings.Contains(err.Error(), "no media") {
		t.Fatalf("a scene of media the company does not have: %v", err)
	}
	if _, err := mc.Call(clara, "media.render", "", map[string]any{"format": "short", "scenes": []map[string]any{{"image": "../../etc/passwd"}}}); err == nil {
		t.Fatal("a path as media")
	}
	if _, err := mc.Call(asMember(co+"/clara", "media.capture"), "media.capture", "example.com", map[string]any{"url": "https://other.example/"}); err == nil {
		t.Fatal("a screenshot of a host the rules did not allow")
	}
	if _, err := mc.Call(context.Background(), "media.voice", "", map[string]any{"text": "hi"}); err == nil {
		t.Fatal("the person made media as a member")
	}

	if err := media.Available(ctx); err != nil {
		if os.Getenv("PIMPO_MEDIA_REQUIRED") != "" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	out, err := mc.Call(clara, "media.render", "", map[string]any{"format": "short", "title": "Run a routine", "scenes": []map[string]any{{"text": "Open Routines", "seconds": 1.5}, {"text": "Press Run", "seconds": 1.5}}})
	if err != nil {
		t.Fatal(err)
	}
	got := out.(map[string]any)
	report := got["check"].(media.Report)
	// Silence is all a scene without a voice has, so loudness cannot pass.
	if report.Width != 1080 || !report.Captions || got["seconds"].(float64) < 2.9 {
		t.Fatalf("render = %+v", got)
	}
	id := got["media"].(string)
	list, _ := ta.Companies.MediaList(ctx, co)
	if len(list) != 1 || list[0].ID != id || list[0].Member != "clara" || list[0].Kind != company.MediaVideo {
		t.Fatalf("media = %+v", list)
	}
	req, _ := http.NewRequest("GET", ta.srv.URL+"/api/companies/"+co+"/media/"+id, nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("serving: %v %+v", err, resp)
	}
	resp.Body.Close()
	if code, _ := ta.do(t, "GET", "/api/companies/"+co+"/media/m_ffffffffffff", nil); code != 404 {
		t.Fatalf("media that is not there: %d", code)
	}
}

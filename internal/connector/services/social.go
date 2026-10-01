package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/connector"
)

// YouTube and LinkedIn, for a company's own channel and page. Uploading
// (always private) and posting are done by Pimpo itself, which has the
// files and adds the AI disclosure; publishing and stats are here.

func init() {
	register(Kind{
		ID: "youtube", Title: "YouTube", Description: "Envia vídeos como privados, publica e lê estatísticas de um canal.",
		Help:   "Use a conta de marca do canal. Num projeto do Google Cloud com a YouTube Data API ligada, crie um cliente OAuth e obtenha um refresh token com os escopos youtube.upload e youtube.readonly (por exemplo no OAuth Playground, com o seu cliente).",
		Fields: []Field{{Name: "client_id", Label: "Client ID"}, {Name: "client_secret", Label: "Client secret", Secret: true}, {Name: "refresh_token", Label: "Refresh token", Secret: true}},
		Specs: []capability.Spec{
			{Name: "youtube.publish", Risk: capability.Irreversible, Signature: "youtube.publish({video})", Returns: "{ok, url}; makes a private video public for everyone",
				Schema: obj(`"video":{"type":"string","description":"the YouTube video id youtube.upload returned"}`, "video")},
			{Name: "youtube.stats", Risk: capability.Read, Signature: "youtube.stats({video})", Returns: "{views, likes, comments, privacy}",
				Schema: obj(`"video":{"type":"string"}`, "video")},
		},
		Call: callYouTube,
		Probe: func(ctx context.Context, cfg Config) error {
			_, err := YouTubeToken(ctx, cfg)
			return err
		},
	})
	register(Kind{
		ID: "linkedin", Title: "LinkedIn", Description: "Publica na página da empresa no LinkedIn.",
		Help:   "Crie um app em linkedin.com/developers com o produto Community Management API, gere um token com w_organization_social e informe a página como urn:li:organization:NÚMERO.",
		Fields: []Field{{Name: "token", Label: "Token", Secret: true}, {Name: "author", Label: "Página", Placeholder: "urn:li:organization:123456"}},
		Probe: func(ctx context.Context, cfg Config) error {
			v, err := need(ctx, cfg, "LinkedIn", "token")
			if err != nil {
				return err
			}
			return doJSON(ctx, "GET", base("linkedin", "https://api.linkedin.com")+"/v2/userinfo", map[string]string{"Authorization": "Bearer " + v[0]}, nil, nil)
		},
	})
}

var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{6,20}$`)

// YouTubeToken is an access token from the channel's refresh token.
func YouTubeToken(ctx context.Context, cfg Config) (string, error) {
	v, err := need(ctx, cfg, "YouTube", "client_id", "client_secret", "refresh_token")
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {v[0]}, "client_secret": {v[1]}, "refresh_token": {v[2]}}
	req, _ := http.NewRequestWithContext(ctx, "POST", base("google-oauth", "https://oauth2.googleapis.com")+"/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := do(req, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("YouTube gave no access token")
	}
	return out.AccessToken, nil
}

func callYouTube(ctx context.Context, cfg Config, name, _ string, args any) (any, error) {
	var a struct {
		Video string `json:"video"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	if !videoID.MatchString(a.Video) {
		return nil, errors.New("video is the id youtube.upload returned")
	}
	token, err := YouTubeToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	h := map[string]string{"Authorization": "Bearer " + token}
	api := base("youtube", "https://www.googleapis.com")
	var list struct {
		Items []struct {
			Status struct {
				PrivacyStatus           string `json:"privacyStatus"`
				SelfDeclaredMadeForKids bool   `json:"selfDeclaredMadeForKids"`
				ContainsSyntheticMedia  bool   `json:"containsSyntheticMedia"`
			} `json:"status"`
			Statistics struct {
				Views    string `json:"viewCount"`
				Likes    string `json:"likeCount"`
				Comments string `json:"commentCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	if err := doJSON(ctx, "GET", api+"/youtube/v3/videos?part=status,statistics&id="+a.Video, h, nil, &list); err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, errors.New("the channel has no such video")
	}
	it := list.Items[0]
	switch name {
	case "youtube.stats":
		return map[string]any{"views": it.Statistics.Views, "likes": it.Statistics.Likes, "comments": it.Statistics.Comments, "privacy": it.Status.PrivacyStatus}, nil
	case "youtube.publish":
		// The whole status is sent, as YouTube wants, keeping what it was.
		status := map[string]any{"privacyStatus": "public", "selfDeclaredMadeForKids": it.Status.SelfDeclaredMadeForKids, "containsSyntheticMedia": it.Status.ContainsSyntheticMedia}
		if err := doJSON(ctx, "PUT", api+"/youtube/v3/videos?part=status", h, map[string]any{"id": a.Video, "status": status}, nil); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "url": "https://youtu.be/" + a.Video}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

// A Video is what an upload says about itself.
type Video struct {
	Title, Description string
	Tags               []string
	ForKids            bool
	// Synthetic tells YouTube it was made with AI.
	Synthetic bool
}

// YouTubeUpload sends a file as a private video and gives its id.
func YouTubeUpload(ctx context.Context, cfg Config, path string, v Video) (string, error) {
	token, err := YouTubeToken(ctx, cfg)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, _ := f.Stat()
	meta := map[string]any{
		"snippet": map[string]any{"title": v.Title, "description": v.Description, "tags": nonNil(v.Tags), "categoryId": "28"},
		"status":  map[string]any{"privacyStatus": "private", "selfDeclaredMadeForKids": v.ForKids, "containsSyntheticMedia": v.Synthetic},
	}
	api := base("youtube", "https://www.googleapis.com")
	start, _ := http.NewRequestWithContext(ctx, "POST", api+"/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status", jsonBody(meta))
	start.Header.Set("Authorization", "Bearer "+token)
	start.Header.Set("Content-Type", "application/json; charset=UTF-8")
	start.Header.Set("X-Upload-Content-Type", "video/mp4")
	start.Header.Set("X-Upload-Content-Length", fmt.Sprint(st.Size()))
	resp, err := uploads.Do(start)
	if err != nil {
		return "", err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode >= 300 || loc == "" {
		return "", fmt.Errorf("YouTube refused the upload: %s", resp.Status)
	}
	put, _ := http.NewRequestWithContext(ctx, "PUT", loc, f)
	put.ContentLength = st.Size()
	put.Header.Set("Authorization", "Bearer "+token)
	put.Header.Set("Content-Type", "video/mp4")
	var out struct {
		ID string `json:"id"`
	}
	if err := doWith(uploads, put, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// LinkedInPost publishes text on the page and gives the post's id.
func LinkedInPost(ctx context.Context, cfg Config, text string) (string, error) {
	v, err := need(ctx, cfg, "LinkedIn", "token", "author")
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(v[1], "urn:li:organization:") && !strings.HasPrefix(v[1], "urn:li:person:") {
		return "", errors.New("the LinkedIn page is a urn:li:organization:… (or urn:li:person:…)")
	}
	body := map[string]any{"author": v[1], "commentary": text, "visibility": "PUBLIC", "lifecycleState": "PUBLISHED", "isReshareDisabledByAuthor": false,
		"distribution": map[string]any{"feedDistribution": "MAIN_FEED", "targetEntities": []any{}, "thirdPartyDistributionChannels": []any{}}}
	req, _ := http.NewRequestWithContext(ctx, "POST", base("linkedin", "https://api.linkedin.com")+"/rest/posts", jsonBody(body))
	req.Header.Set("Authorization", "Bearer "+v[0])
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LinkedIn-Version", "202409")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return "", &statusError{resp.StatusCode, fmt.Sprintf("LinkedIn answered %s: %s", resp.Status, strings.TrimSpace(string(raw)))}
	}
	return resp.Header.Get("x-restli-id"), nil
}

// uploads may take long: a video is sent whole.
var uploads = &http.Client{Timeout: 30 * time.Minute}

func do(req *http.Request, out any) error { return doWith(client, req, out) }

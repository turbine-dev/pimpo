package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/turbine-dev/pimpo/internal/capability"
	"github.com/turbine-dev/pimpo/internal/company"
	"github.com/turbine-dev/pimpo/internal/connector/services"
	"github.com/turbine-dev/pimpo/internal/explore"
	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/media"
	"github.com/turbine-dev/pimpo/internal/people"
)

// Publishing what a company makes: a video goes to YouTube as private,
// and only publishing makes it public; posts go to LinkedIn; anything for
// a network Pimpo does not reach is sent, ready, to the person's phone to
// publish by hand. What a company member publishes always says it was
// made with AI.

func init() {
	for _, s := range []capability.Spec{
		{Name: "youtube.upload", Risk: capability.Reversible, Signature: "youtube.upload({media, title, description, tags, made_for_kids})",
			Returns: "{video, url}; sends a company video that passed its checks to the channel, always private, marked as made with AI; youtube.publish makes it public",
			Schema:  `{"type":"object","properties":{"media":{"type":"string"},"title":{"type":"string"},"description":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}},"made_for_kids":{"type":"boolean"}},"required":["media","title"]}`},
		{Name: "linkedin.post", Risk: capability.Irreversible, Signature: "linkedin.post({text})", Returns: "{post}; publishes on the company's LinkedIn page for everyone",
			Schema: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`},
		{Name: "publish.assisted", Risk: capability.Notify, Signature: "publish.assisted({where, text, media})",
			Returns: "{ok}; sends a ready post (its text and the media to attach) to the person's phone, to publish by hand where Pimpo does not post, such as Instagram or TikTok",
			Schema:  `{"type":"object","properties":{"where":{"type":"string"},"text":{"type":"string"},"media":{"type":"string"}},"required":["where","text"]}`},
	} {
		capability.Register(s)
	}
}

type publishCap struct{ a *App }

func (publishCap) Capabilities() []string {
	return []string{"youtube.upload", "linkedin.post", "publish.assisted"}
}

func (c publishCap) Call(ctx context.Context, name, _ string, args any) (any, error) {
	a := c.a
	b, _ := json.Marshal(args)
	if name == "linkedin.post" && host.MemberOf(ctx) == "" {
		var in struct{ Text string }
		json.Unmarshal(b, &in)
		return a.linkedInPost(ctx, in.Text)
	}
	o, me, _, err := a.caller(ctx)
	if err != nil {
		return nil, err
	}
	disclose := o.Disclosing()
	switch name {
	case "youtube.upload":
		var in struct {
			Media, Title, Description string
			Tags                      []string
			MadeForKids               bool `json:"made_for_kids"`
		}
		json.Unmarshal(b, &in)
		m, path, err := a.mediaPath(ctx, o.ID, in.Media, company.MediaVideo)
		if err != nil {
			return nil, err
		}
		// What failed its checks is fixed first, not uploaded.
		report, err := media.Check(ctx, path, media.Formats[m.Format])
		if err != nil {
			return nil, err
		}
		if !report.OK() {
			return nil, fmt.Errorf("the video does not pass its checks yet: %s", strings.Join(report.Problems, "; "))
		}
		title := strings.TrimSpace(in.Title)
		if title == "" || len([]rune(title)) > 100 {
			return nil, errors.New("a title of up to 100 characters")
		}
		id, err := services.YouTubeUpload(ctx, a.catalogConfig("youtube"), path, services.Video{Title: title, Description: withDisclosure(in.Description, disclose), Tags: in.Tags, ForKids: in.MadeForKids, Synthetic: true})
		if err != nil {
			return nil, err
		}
		a.Events.Append(ctx, "company.media.uploaded", actorFor(o, me), map[string]any{"company": o.ID, "media": m.ID, "video": id, "person": o.Person})
		return map[string]any{"video": id, "url": "https://youtu.be/" + id, "privacy": "private"}, nil
	case "linkedin.post":
		var in struct{ Text string }
		json.Unmarshal(b, &in)
		return a.linkedInPost(ctx, withDisclosure(in.Text, disclose))
	case "publish.assisted":
		var in struct{ Where, Text, Media string }
		json.Unmarshal(b, &in)
		if strings.TrimSpace(in.Where) == "" || strings.TrimSpace(in.Text) == "" {
			return nil, errors.New("say where it goes and what it says")
		}
		text := fmt.Sprintf("%s · ready for %s:\n\n%s", o.Name, clip(in.Where, 60), withDisclosure(clip(in.Text, 3000), disclose))
		if in.Media != "" {
			m, _, err := a.mediaPath(ctx, o.ID, in.Media)
			if err != nil {
				return nil, err
			}
			text += fmt.Sprintf("\n\nAttach %s from the company's Media tab: %s/companies/%s?tab=media", m.ID, strings.TrimSuffix(a.Explore.BaseURL, "/"), o.ID)
		}
		text += "\n\nMark it as AI-generated where the network asks."
		a.Channel.Notify(people.With(ctx, o.Person), explore.Notice{Text: text, To: o.Person, Kind: "task"})
		a.Events.Append(ctx, "company.publish.assisted", actorFor(o, me), map[string]any{"company": o.ID, "where": in.Where, "person": o.Person})
		return map[string]bool{"ok": true}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}

func (a *App) linkedInPost(ctx context.Context, text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 3000 {
		return nil, errors.New("a post of up to 3000 characters")
	}
	id, err := services.LinkedInPost(ctx, a.catalogConfig("linkedin"), text)
	if err != nil {
		return nil, err
	}
	return map[string]string{"post": id}, nil
}

// withDisclosure ends a text with the company's AI disclosure.
func withDisclosure(text, disclosure string) string {
	text = strings.TrimSpace(text)
	if strings.Contains(text, disclosure) {
		return text
	}
	if text == "" {
		return disclosure
	}
	return text + "\n\n" + disclosure
}

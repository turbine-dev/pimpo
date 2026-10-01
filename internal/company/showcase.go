package company

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// A Showcase is a company's optional public page, with only what a person
// put there: shipped briefs, published videos and posts, links. It names
// no one.
type Showcase struct {
	On    bool           `json:"on,omitempty" yaml:"-"`
	Slug  string         `json:"slug,omitempty" yaml:"-"`
	Items []ShowcaseItem `json:"items,omitempty" yaml:"-"`
}

type ShowcaseItem struct {
	ID    string    `json:"id"`
	Kind  string    `json:"kind"`
	Title string    `json:"title"`
	URL   string    `json:"url,omitempty"`
	Note  string    `json:"note,omitempty"`
	Ref   string    `json:"ref,omitempty"`
	Added time.Time `json:"added"`
}

// Kinds of showcase item.
const (
	ShowBrief = "brief"
	ShowVideo = "video"
	ShowLink  = "link"
)

var slugRule = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,38}[a-z0-9])$`)

func (s Showcase) check() error {
	if s.On && !slugRule.MatchString(s.Slug) {
		return errors.New("the showcase's address is 3 to 40 lowercase letters, digits and dashes")
	}
	if len(s.Items) > 50 {
		return errors.New("at most 50 things on the showcase")
	}
	for _, it := range s.Items {
		if strings.TrimSpace(it.Title) == "" || len([]rune(it.Title)) > 200 || len([]rune(it.Note)) > 1000 {
			return errors.New("each thing on the showcase has a title of up to 200 characters")
		}
		if it.Kind != ShowBrief && it.Kind != ShowVideo && it.Kind != ShowLink {
			return errors.New("the showcase shows briefs, videos and links")
		}
		if it.URL != "" {
			if u, err := url.Parse(it.URL); err != nil || u.Scheme != "https" || u.Host == "" {
				return errors.New("a showcase link is an https address")
			}
		}
	}
	return nil
}

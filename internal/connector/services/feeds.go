package services

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/denerFernandes/vigia/internal/capability"
	"github.com/denerFernandes/vigia/internal/connector"
)

func init() {
	register(Kind{
		ID: "rss", Title: "RSS e Atom", Description: "Lê feeds de notícias, blogs e lançamentos.",
		Help: "Não precisa configurar nada: cada rotina declara os sites que pode ler.",
		Specs: []capability.Spec{{Name: "rss.read", Risk: capability.Read, Scoped: true, ScopeArg: "url",
			Signature: "rss.read({url, max})", Returns: "[{title, link, published, summary}] newest first",
			Schema: obj(`"url":{"type":"string","description":"feed URL"},"max":{"type":"integer"}`, "url")}},
		Call: readFeed,
	})
}

type feed struct {
	Items []struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		PubDate string `xml:"pubDate"`
		Desc    string `xml:"description"`
	} `xml:"channel>item"`
	Entries []struct {
		Title string `xml:"title"`
		Links []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
		Updated   string `xml:"updated"`
		Published string `xml:"published"`
		Summary   string `xml:"summary"`
		Content   string `xml:"content"`
	} `xml:"entry"`
}

var tags = regexp.MustCompile(`<[^>]+>`)

func plain(s string) string {
	s = html.UnescapeString(tags.ReplaceAllString(s, " "))
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 400 {
		s = string(r[:399]) + "…"
	}
	return s
}

func when(s string) string {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t.Format(time.RFC3339)
		}
	}
	return strings.TrimSpace(s)
}

func readFeed(ctx context.Context, _ Config, _, _ string, args any) (any, error) {
	var a struct {
		URL string `json:"url"`
		Max int    `json:"max"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(a.URL, "https://") && !strings.HasPrefix(a.URL, "http://") {
		return nil, fmt.Errorf("url must be http(s)")
	}
	if a.Max <= 0 || a.Max > 50 {
		a.Max = 20
	}
	req, err := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Vigia/1 (+https://github.com/denerFernandes/vigia)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("feed answered %s", resp.Status)
	}
	var f feed
	dec := xml.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	dec.Strict = false
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("not a feed: %w", err)
	}
	out := []map[string]any{}
	for _, it := range f.Items {
		out = append(out, map[string]any{"title": plain(it.Title), "link": strings.TrimSpace(it.Link), "published": when(it.PubDate), "summary": plain(it.Desc)})
	}
	for _, e := range f.Entries {
		link := ""
		for _, l := range e.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.Href
				break
			}
		}
		pub := e.Published
		if pub == "" {
			pub = e.Updated
		}
		sum := e.Summary
		if sum == "" {
			sum = e.Content
		}
		out = append(out, map[string]any{"title": plain(e.Title), "link": link, "published": when(pub), "summary": plain(sum)})
	}
	if len(out) > a.Max {
		out = out[:a.Max]
	}
	return out, nil
}

package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// MaxText is how much of a page's text a routine gets.
const MaxText = 30000

// read fetches a web page and gives its title, readable text, the
// structured data it publishes (JSON-LD, where shops put product prices),
// and its links. Scripts, styles and hidden parts are left out.
func (w *Web) read(ctx context.Context, raw, scope string) (any, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("%q is not an http(s) URL", raw)
	}
	if !strings.EqualFold(u.Hostname(), scope) {
		return nil, fmt.Errorf("%s is outside the allowed host %s", u.Hostname(), scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) Pimpo/1")
	resp, err := w.client(scope).Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s failed: %w", u.Hostname(), errors.Unwrap(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s answered %d", u.Hostname(), resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "text/") {
		return nil, fmt.Errorf("%s is not a web page (%s)", u.Hostname(), ct)
	}
	return ParsePage(io.LimitReader(resp.Body, 5<<20), resp.Request.URL)
}

var spaces = regexp.MustCompile(`[ \t\r\f\v\x{00a0}\x{202f}]+`)
var blankLines = regexp.MustCompile(`\n{2,}`)

// ParsePage reads an HTML document into {url, title, description, text,
// data, links}.
func ParsePage(r io.Reader, base *url.URL) (map[string]any, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("unreadable page: %w", err)
	}
	var title, description string
	var text strings.Builder
	data := []any{}
	links := []map[string]string{}
	seen := map[string]bool{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script":
				if attr(n, "type") == "application/ld+json" && n.FirstChild != nil && len(data) < 10 {
					var v any
					if json.Unmarshal([]byte(n.FirstChild.Data), &v) == nil {
						data = append(data, v)
					}
				}
				return
			case "style", "noscript", "template", "svg", "iframe", "head":
				if n.Data == "head" {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						walk(c)
					}
				}
				return
			case "title":
				if n.FirstChild != nil && title == "" {
					title = strings.TrimSpace(n.FirstChild.Data)
				}
				return
			case "meta":
				if (attr(n, "name") == "description" || attr(n, "property") == "og:description") && description == "" {
					description = strings.TrimSpace(attr(n, "content"))
				}
				return
			case "a":
				if href := attr(n, "href"); href != "" && len(links) < 100 {
					if ref, err := base.Parse(href); err == nil && (ref.Scheme == "http" || ref.Scheme == "https") && !seen[ref.String()] {
						seen[ref.String()] = true
						links = append(links, map[string]string{"text": strings.TrimSpace(nodeText(n)), "url": ref.String()})
					}
				}
			}
			if hasAttr(n, "hidden") || attr(n, "aria-hidden") == "true" || strings.Contains(strings.ReplaceAll(attr(n, "style"), " ", ""), "display:none") {
				return
			}
			if block[n.Data] {
				text.WriteString("\n")
			}
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && block[n.Data] {
			text.WriteString("\n")
		}
	}
	walk(doc)
	lines := strings.Split(text.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaces.ReplaceAllString(l, " "))
	}
	body := strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n"))
	truncated := false
	if r := []rune(body); len(r) > MaxText {
		body, truncated = string(r[:MaxText]), true
	}
	return map[string]any{"url": base.String(), "title": title, "description": description, "text": body, "truncated": truncated, "data": data, "links": links}, nil
}

var block = map[string]bool{"p": true, "div": true, "br": true, "li": true, "tr": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"section": true, "article": true, "header": true, "footer": true, "table": true, "ul": true, "ol": true, "dd": true, "dt": true, "blockquote": true, "pre": true, "main": true, "nav": true, "aside": true}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return spaces.ReplaceAllString(b.String(), " ")
}

package external

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The official MCP Registry lists public servers and how to run them. It
// says nothing about what their tools may do: the owner reviews that when
// adding one.

const DefaultRegistry = "https://registry.modelcontextprotocol.io"

// Input is something the owner fills in before adding a server: an env
// var, a request header, or a command-line argument.
type Input struct {
	Kind        string `json:"kind"` // env, header or arg
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
}

// Listing is one registry server in the form Zodim can add.
type Listing struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Repository  string   `json:"repository,omitempty"`
	Kind        string   `json:"kind"` // npm, pypi, remote or unsupported
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	URL         string   `json:"url,omitempty"`
	Inputs      []Input  `json:"inputs"`
}

type regVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Secret      bool   `json:"isSecret"`
	Required    bool   `json:"isRequired"`
	Default     string `json:"default"`
	Value       string `json:"value"`
	Type        string `json:"type"`
	ValueHint   string `json:"valueHint"`
}

type regServer struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Repository  struct {
		URL string `json:"url"`
	} `json:"repository"`
	Packages []struct {
		RegistryType string                `json:"registryType"`
		Identifier   string                `json:"identifier"`
		Version      string                `json:"version"`
		Transport    struct{ Type string } `json:"transport"`
		RuntimeArgs  []regVar              `json:"runtimeArguments"`
		PackageArgs  []regVar              `json:"packageArguments"`
		Env          []regVar              `json:"environmentVariables"`
	} `json:"packages"`
	Remotes []struct {
		Type    string   `json:"type"`
		URL     string   `json:"url"`
		Headers []regVar `json:"headers"`
	} `json:"remotes"`
}

var nonName = regexp.MustCompile(`[^a-z0-9]+`)

// SuggestName turns a registry name like io.github.acme/weather-mcp into
// a connector name (weather).
func SuggestName(regName string) string {
	last := regName[strings.LastIndex(regName, "/")+1:]
	n := nonName.ReplaceAllString(strings.ToLower(last), "")
	for _, cut := range []string{"mcpserver", "servermcp", "mcp", "server"} {
		if t := strings.TrimSuffix(strings.TrimPrefix(n, cut), cut); len(t) >= 2 {
			n = t
		}
	}
	if len(n) > 31 {
		n = n[:31]
	}
	if n == "" || n[0] < 'a' || n[0] > 'z' {
		n = "x" + n
	}
	return n
}

func args(vars []regVar, inputs *[]Input) []string {
	var out []string
	for _, a := range vars {
		switch {
		case a.Type == "named" && a.Value != "":
			out = append(out, flag(a.Name), a.Value)
		case a.Value != "":
			out = append(out, a.Value)
		case a.Required:
			hint := a.ValueHint
			if hint == "" {
				hint = a.Name
			}
			if a.Type == "named" {
				out = append(out, flag(a.Name))
			}
			out = append(out, "{"+hint+"}")
			*inputs = append(*inputs, Input{Kind: "arg", Name: hint, Description: a.Description, Required: true, Default: a.Default})
		}
	}
	return out
}

func flag(n string) string {
	if strings.HasPrefix(n, "-") {
		return n
	}
	return "--" + n
}

func listing(s regServer) Listing {
	l := Listing{ID: s.Name, Name: SuggestName(s.Name), Title: s.Title, Description: s.Description, Version: s.Version, Repository: s.Repository.URL, Kind: "unsupported", Inputs: []Input{}}
	if l.Title == "" {
		l.Title = s.Name[strings.LastIndex(s.Name, "/")+1:]
	}
	for _, p := range s.Packages {
		if p.Transport.Type != "stdio" || (p.RegistryType != "npm" && p.RegistryType != "pypi") || p.Identifier == "" {
			continue
		}
		var inputs []Input
		if p.RegistryType == "npm" {
			l.Command = "npx"
			rt := args(p.RuntimeArgs, &inputs)
			if len(rt) == 0 {
				rt = []string{"-y"}
			}
			l.Args = append(rt, p.Identifier+"@"+p.Version)
		} else {
			l.Command = "uvx"
			l.Args = append(args(p.RuntimeArgs, &inputs), p.Identifier+"=="+p.Version)
		}
		l.Args = append(l.Args, args(p.PackageArgs, &inputs)...)
		for _, e := range p.Env {
			inputs = append(inputs, Input{Kind: "env", Name: e.Name, Description: e.Description, Secret: e.Secret, Required: e.Required, Default: e.Default})
		}
		l.Kind, l.Inputs = p.RegistryType, inputs
		if l.Inputs == nil {
			l.Inputs = []Input{}
		}
		return l
	}
	for _, r := range s.Remotes {
		if r.Type != "streamable-http" || !strings.HasPrefix(r.URL, "https://") {
			continue
		}
		l.Kind, l.URL = "remote", r.URL
		for _, h := range r.Headers {
			l.Inputs = append(l.Inputs, Input{Kind: "header", Name: h.Name, Description: h.Description, Secret: h.Secret, Required: h.Required, Default: h.Value})
		}
		return l
	}
	return l
}

// Search asks the registry for servers matching q, latest versions only.
func Search(ctx context.Context, client *http.Client, base, q, cursor string) ([]Listing, string, error) {
	if base == "" {
		base = DefaultRegistry
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	v := url.Values{"limit": {"30"}, "version": {"latest"}}
	if q = strings.TrimSpace(q); q != "" {
		v.Set("search", q)
	}
	if cursor != "" {
		v.Set("cursor", cursor)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/v0/servers?"+v.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("the MCP registry is unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("the MCP registry answered %d", resp.StatusCode)
	}
	var page struct {
		Servers []struct {
			Server regServer `json:"server"`
		} `json:"servers"`
		Metadata struct {
			Next string `json:"nextCursor"`
		} `json:"metadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("unexpected answer from the MCP registry: %w", err)
	}
	out := []Listing{}
	for _, s := range page.Servers {
		out = append(out, listing(s.Server))
	}
	return out, page.Metadata.Next, nil
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/denerFernandes/pimpo/internal/connector/external"
)

// connectorCmd checks an external connector's contract before it is
// installed: `pimpo connector check DIR`. Secrets for the check come from
// the environment, under the names the connector declares.
func connectorCmd(args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "openapi" {
		return openapiCmd(args[1:], out)
	}
	if len(args) != 2 || args[0] != "check" {
		return errors.New("usage: pimpo connector check DIR   (then copy DIR into ~/.pimpo/connectors/)\n       pimpo connector openapi [--only op1,op2] [--header NAME] SPEC NAME DIR")
	}
	m, err := external.Load(args[1])
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %d capabilities, %d contract cases\n", m.Name, len(m.Capabilities), len(m.Contract))
	for _, c := range m.Capabilities {
		fmt.Fprintf(out, "  %-24s %-12s %s\n", c.Name, c.Risk, c.Signature)
	}
	problems := external.Check(context.Background(), m, func(_ context.Context, name string) (string, error) { return os.Getenv(name), nil })
	if len(problems) > 0 {
		return fmt.Errorf("contract failed:\n  %s", strings.Join(problems, "\n  "))
	}
	fmt.Fprintln(out, "contract ok")
	return nil
}

// openapiCmd writes a JSON connector from an OpenAPI description, a file
// or an https address: `pimpo connector openapi [--only a,b] [--header
// Authorization] SPEC NAME DIR`. Without --only it lists the operations.
func openapiCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("openapi", flag.ContinueOnError)
	only := fs.String("only", "", "operations to include, comma-separated (all GET operations with 'get')")
	header := fs.String("header", "", "a header that carries a key the description does not declare")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("usage: pimpo connector openapi [--only op1,op2|get] [--header NAME] SPEC [NAME DIR]")
	}
	spec := fs.Arg(0)
	var raw []byte
	var err error
	specURL := ""
	if strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "http://") {
		raw, err = external.FetchSpec(context.Background(), spec)
		specURL = spec
	} else {
		raw, err = os.ReadFile(spec)
	}
	if err != nil {
		return err
	}
	api, err := external.ParseOpenAPI(raw, specURL)
	if err != nil {
		return err
	}
	if *header != "" {
		if err := api.AddHeader(*header); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "%s — %s, %d operations\n", api.Title, api.Base, len(api.Operations))
	for _, k := range api.Keys {
		fmt.Fprintf(out, "  key %s: %s\n", k.Name, k.Description)
	}
	if *only == "" || fs.NArg() != 3 {
		for _, o := range api.Operations {
			fmt.Fprintf(out, "  %-40s %-6s %s  %s\n", o.ID, o.Method, o.Path, o.Summary)
		}
		for _, s := range api.Unsupported {
			fmt.Fprintf(out, "  %-40s (unsupported: %s)\n", s.ID, s.Why)
		}
		fmt.Fprintln(out, "choose with --only op1,op2 (or --only get) and give NAME DIR")
		return nil
	}
	chosen := map[string]string{}
	for _, o := range api.Operations {
		if *only == "get" && o.Method == "GET" {
			chosen[o.ID] = o.Risk
		}
	}
	if *only != "get" {
		risk := map[string]string{}
		for _, o := range api.Operations {
			risk[o.ID] = o.Risk
		}
		for _, id := range strings.Split(*only, ",") {
			id = strings.TrimSpace(id)
			if r, ok := risk[id]; ok {
				chosen[id] = r
			} else {
				chosen[id] = "irreversible" // Manifest reports it as unknown
			}
		}
	}
	m, err := api.Manifest(fs.Arg(1), "", specURL, chosen)
	if err != nil {
		return err
	}
	dir := fs.Arg(2)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "connector.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	if _, err := external.Load(dir); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s with %d capabilities; review each risk, then: pimpo connector check %s\n", filepath.Join(dir, "connector.json"), len(m.Capabilities), dir)
	return nil
}

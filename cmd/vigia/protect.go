package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/denerFernandes/vigia/internal/protect"
	"github.com/denerFernandes/vigia/protection"
)

const protectUsage = `usage:
  vigia protect suggest --domain DOMAIN | --pattern REGEX [--capability NAME] --reason "why"   (prints an entry to propose by pull request)
  vigia protect sign --key KEYFILE DIR       (maintainers: signs DIR/entries.json into DIR/list.json)
  vigia protect verify FILE`

func protectCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(protectUsage)
	}
	fs := flag.NewFlagSet("protect", flag.ContinueOnError)
	domain := fs.String("domain", "", "")
	pattern := fs.String("pattern", "", "")
	capName := fs.String("capability", "", "")
	reason := fs.String("reason", "", "")
	key := fs.String("key", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "suggest":
		kind, value := "domain", *domain
		if *pattern != "" {
			kind, value = "pattern", *pattern
		}
		e, err := protect.Suggestion(kind, value, *reason)
		if err != nil {
			return err
		}
		e.Capability = *capName
		b, _ := json.MarshalIndent(e, "", "  ")
		fmt.Fprintln(out, string(b))
		fmt.Fprintln(out, "\nPropose it by pull request to github.com/denerFernandes/vigia-protection. Only the domain or pattern and the reason go in: never the message it came from.")
		return nil
	case "sign":
		if *key == "" || fs.NArg() != 1 {
			return errors.New(protectUsage)
		}
		priv, err := os.ReadFile(*key)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(filepath.Join(fs.Arg(0), "entries.json"))
		if err != nil {
			return err
		}
		var l protect.List
		if err := json.Unmarshal(raw, &l); err != nil {
			return err
		}
		l, err = protect.Sign(l, strings.TrimSpace(string(priv)))
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(l, "", " ")
		if err := os.WriteFile(filepath.Join(fs.Arg(0), "list.json"), append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "signed version %d with %d entries\n", l.Version, len(l.Entries))
		return nil
	case "verify":
		if fs.NArg() != 1 {
			return errors.New(protectUsage)
		}
		raw, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			return err
		}
		g := &protect.Guard{Keys: protection.Keys}
		if err := g.Load(raw); err != nil {
			return err
		}
		v, n, _, _ := g.Status()
		fmt.Fprintf(out, "ok: version %d, %d entries\n", v, n)
		return nil
	}
	return errors.New(protectUsage)
}

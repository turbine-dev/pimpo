package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/turbine-dev/pimpo/internal/local"
)

// localCmd lists and installs the models of the local catalog from the
// terminal, with the same checks as Settings › Models › Download models.
func localCmd(args []string, out io.Writer) error {
	usage := errors.New("usage: pimpo local list | pimpo local install ID… | pimpo local remove ID")
	if len(args) == 0 {
		return usage
	}
	fs := flag.NewFlagSet("local", flag.ContinueOnError)
	dir := fs.String("data", "", "data directory (default ~/.pimpo)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	root := filepath.Join(dataDir(*dir), "local")
	if err := local.UseFile(filepath.Join(root, "catalog.json")); err != nil {
		return fmt.Errorf("your catalog.json: %w", err)
	}
	m := &local.Manager{Dir: root}
	switch args[0] {
	case "list":
		items := append(append([]local.Item{}, local.Voices...), local.Transcribers...)
		if e, ok := local.Engine(); ok {
			items = append([]local.Item{e}, items...)
		}
		for _, it := range items {
			mark := " "
			if m.Installed(it) {
				mark = "✓"
			}
			fmt.Fprintf(out, "%s %-28s %-12s %6d MB  %s %v\n", mark, it.ID, it.Kind, it.Size>>20, it.Name, it.Languages)
		}
		return nil
	case "remove":
		for _, id := range fs.Args() {
			if err := m.Remove(id); err != nil {
				return err
			}
			fmt.Fprintf(out, "removed %s\n", id)
		}
		return nil
	case "install":
		for _, id := range fs.Args() {
			j, err := m.Install(id)
			if err != nil {
				return err
			}
			for {
				time.Sleep(time.Second)
				var cur local.Job
				for _, x := range m.Jobs() {
					if x.ID == j.ID {
						cur = x
					}
				}
				fmt.Fprintf(out, "\r%-16s %-12s %4d / %d MB   ", cur.Name, cur.State, cur.Done>>20, cur.Total>>20)
				switch cur.State {
				case "done":
					fmt.Fprintln(out)
				case "failed", "cancelled":
					fmt.Fprintln(out)
					return fmt.Errorf("%s: %s", id, cur.Error)
				default:
					continue
				}
				break
			}
		}
		return nil
	}
	return usage
}

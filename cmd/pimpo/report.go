package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
	"github.com/turbine-dev/pimpo/internal/store"
	"github.com/turbine-dev/pimpo/internal/usage"
)

// reportCmd prints how the routines did in real use: `pimpo report
// [--days 21] [--json]`. It reads the data folder and may run while Pimpo
// does.
func reportCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	dir := fs.String("data", "", "data directory (default ~/.pimpo)")
	days := fs.Int("days", 21, "how many days back")
	asJSON := fs.Bool("json", false, "print JSON instead of Markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ev, err := event.Open(filepath.Join(dataDir(*dir), "pimpo.db"))
	if err != nil {
		return err
	}
	defer ev.Close()
	st, err := store.Open(ev.DB())
	if err != nil {
		return err
	}
	rep, err := usage.Build(context.Background(), ev, st, time.Now(), *days, time.Local)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Fprint(out, rep.Markdown(time.Local))
	return nil
}

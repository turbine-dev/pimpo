package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/denerFernandes/vigia/internal/app"
	"github.com/denerFernandes/vigia/internal/event"
	"github.com/denerFernandes/vigia/internal/migrate"
	"github.com/denerFernandes/vigia/internal/snapshot"
	"github.com/denerFernandes/vigia/internal/vault"
)

func migrateCmd(args []string, out io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: vigia migrate openclaw|hermes [--home DIR] [--apply] [--secrets] [--trust]")
	}
	from := args[0]
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fromHome := fs.String("home", "", "the other agent's data (default ~/.openclaw or ~/.hermes)")
	dir := fs.String("data", "", "Vigia data directory (default ~/.vigia)")
	apply := fs.Bool("apply", false, "import; without it, only show what would come over")
	secrets := fs.Bool("secrets", false, "also copy the Telegram token and mail password into the vault")
	trust := fs.Bool("trust", false, "treat imported memories and rules as your own words")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	p, err := migrate.Read(from, *fromHome)
	if err != nil {
		return err
	}
	report(out, p)
	if !*apply {
		fmt.Fprintln(out, "\nNothing imported yet. Run again with --apply (add --secrets to bring the tokens).")
		return nil
	}

	home := dataDir(*dir)
	if running(home) {
		return errors.New("Vigia is running; import from Settings › Import in the web app instead")
	}
	store, err := event.Open(filepath.Join(home, "vigia.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	snap, err := snapshot.Create(store.DB(), home, "before-import")
	if err != nil {
		return err
	}
	v, err := vault.Open(store.DB(), vault.OSKey(home))
	if err != nil {
		return err
	}
	ctx := context.Background()
	a, err := app.New(ctx, store, v, "", "")
	if err != nil {
		return err
	}
	if err := a.AttachMemory(filepath.Join(home, "memory")); err != nil {
		return err
	}
	n, err := a.Import(ctx, p, app.ImportOptions{Memories: true, Rules: true, Tasks: true, Secrets: *secrets, Trust: *trust}, "human:owner")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\nImported %d memories, %d rules and %d tasks.", n.Memories, n.Rules, n.Tasks)
	if n.Telegram || n.Mail {
		fmt.Fprint(out, " Tokens are in the vault.")
	}
	fmt.Fprintf(out, "\nThe tasks wait on the Routines page: explore each once and it becomes a routine.\nChanged your mind? `vigia restore %s`\n", snap.Name)
	return nil
}

func report(out io.Writer, p migrate.Plan) {
	fmt.Fprintf(out, "From %s (%s)\n\n", p.From, p.Home)
	fmt.Fprintf(out, "  %d memories, %d standing-rule files\n", len(p.Memories), len(p.Rules))
	fmt.Fprintf(out, "  %d scheduled tasks\n", len(p.Tasks))
	for _, t := range p.Tasks {
		state := ""
		if !t.Enabled {
			state = " (paused)"
		}
		fmt.Fprintf(out, "    - %s: %s%s\n", t.Name, t.Schedule, state)
	}
	if len(p.Skills) > 0 {
		verdicts := map[string]string{"works": "can become a routine", "partial": "partly", "no": "no"}
		fmt.Fprintf(out, "  %d skills\n", len(p.Skills))
		for _, s := range p.Skills {
			line := fmt.Sprintf("    - %s: %s", s.Name, verdicts[s.Verdict])
			if len(s.Missing) > 0 {
				line += " (" + strings.Join(s.Missing, "; ") + ")"
			}
			fmt.Fprintln(out, line)
		}
	}
	if p.Telegram.HasBot {
		fmt.Fprintln(out, "  Telegram bot found")
	}
	if p.Mail.Address != "" {
		fmt.Fprintf(out, "  Mail account %s\n", p.Mail.Address)
	}
	for _, w := range p.Warnings {
		fmt.Fprintln(out, "  ! "+w)
	}
}

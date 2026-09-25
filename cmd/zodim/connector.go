package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/denerFernandes/zodim/internal/connector/external"
)

// connectorCmd checks an external connector's contract before it is
// installed: `zodim connector check DIR`. Secrets for the check come from
// the environment, under the names the connector declares.
func connectorCmd(args []string, out io.Writer) error {
	if len(args) != 2 || args[0] != "check" {
		return errors.New("usage: zodim connector check DIR   (then copy DIR into ~/.zodim/connectors/)")
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

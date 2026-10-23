package cli

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
)

// maxStatusLineInput bounds what the harness's JSON may be: it carries no
// transcript, and a pipe that never ends must not hold the pane.
const maxStatusLineInput = 1 << 20

// statusLineInputWait is how long the harness's JSON may take to arrive; a
// byte limit alone would not stop a pipe that is held open and silent.
const statusLineInputWait = 3 * time.Second

// readStatusLineInput reads what the harness wrote, within the bound of bytes
// and of time; what came before the deadline is kept.
func readStatusLineInput(in *os.File, wait time.Duration) []byte {
	_ = in.SetReadDeadline(time.Now().Add(wait))
	input, _ := io.ReadAll(io.LimitReader(in, maxStatusLineInput))
	return input
}

// statuslineCommand is what a worker's harness runs as its status line
// (docs/v2/design-claude.md#the-cache-ttl): called by the harness on its own,
// never by an agent.
func statuslineCommand(catalog adapter.Catalog) *Command {
	return &Command{
		Name:    "statusline",
		Summary: "The status line a worker's harness runs: forwards what its input tells of the session to the run's host.",
		Hidden:  true,
		Examples: []string{
			"rewake statusline",
		},
		Notes: []string{
			"Reads the harness's JSON on stdin, prints the person's own status line if the workers' settings name one, and always exits 0: a status line must not break the pane.",
		},
		Access: viewForm,
		Handler: func(ctx *Context, _ Call) error {
			input := readStatusLineInput(os.Stdin, statusLineInputWait)
			run, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			for _, harness := range catalog.All() {
				if liner, ok := harness.(adapter.StatusLiner); ok {
					liner.StatusLine(run, input, ctx.Stdout)
				}
			}
			return nil
		},
	}
}

package claude

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
)

// The status line of every run with a mod (docs/claude.md#the-cache-ttl):
// Claude Code runs `rewake statusline` after each response and on a timer,
// with its own JSON on stdin. The command forwards what the input tells to
// the run's host over the mod socket and prints nothing: the run's statusLine
// shadows the person's own, and rewake's shell shows what that line would
// repeat (the owner's decision, October 31, 2026; docs/shell.md, "The
// strip"). What it reads, posts and installs is statusline_contract.go; this
// file runs it.

// postTimeout bounds the post: the pane waits for the command to end. A
// variable so a test can shorten it.
var postTimeout = 2 * time.Second

var _ adapter.StatusLiner = claudeHarness{}

// StatusLine posts what the input tells of the session to the run's host and
// prints nothing; a host that is not there costs only the post.
func (claudeHarness) StatusLine(ctx context.Context, input []byte, _ io.Writer) {
	seen, parsed := parseLineInput(input)
	if !parsed {
		return
	}
	if body := seen.body(); body != (modhost.StatusLineRequest{}) {
		post, cancel := context.WithTimeout(ctx, postTimeout)
		defer cancel()
		postLine(post, os.Getenv(modSocketEnv), body)
	}
}

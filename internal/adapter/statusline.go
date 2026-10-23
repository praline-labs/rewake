package adapter

import (
	"context"
	"io"
)

// StatusLiner is implemented by a harness that runs a command of rewake's as
// its status line and reads what it tells of the session from the command's
// input. StatusLine is given that input and writes the line the harness
// shows, nothing at all being a line. It runs inside the harness's own tree
// and must not fail the pane: whatever it cannot do it leaves undone.
type StatusLiner interface {
	StatusLine(ctx context.Context, input []byte, out io.Writer)
}

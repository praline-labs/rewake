package cli

import (
	"os"

	"github.com/iiiokojiadbi/rewake/internal/boottime"
	"github.com/iiiokojiadbi/rewake/internal/harness/claude/telemetry"
)

// handleObserve is what a Claude Code hook runs: it passes the few fields the
// wrapper's collector uses on to it, and returns. Like turn-ended it never
// fails loudly, and unlike it it never waits for anything — it sits in front
// of every prompt, and a wrapper that is gone or busy must cost the session
// nothing.
func handleObserve(_ *Context, call Call) error {
	socket := ""
	if len(call.Raw) > 0 {
		socket = call.Raw[0]
	}
	if event, ok := telemetry.DecodeHook(readPayload(os.Stdin)); ok {
		telemetry.Send(socket, event)
		if event.Kind == telemetry.UserPromptSubmit {
			// The start of a turn, for `rewake pending` (turnstart.go).
			telemetry.RecordTurnStart(socket, boottime.ProcessStarted)
		}
	}
	return nil
}

// handleStatusTap is the status-line command of a Claude Code session: it
// tells the wrapper what the status line was handed and then runs the
// person's own status line in its place (internal/harness/claude/telemetry).
// Ordinarily the process becomes the person's command and never returns here.
func handleStatusTap(_ *Context, call Call) error {
	socket, sources, caller := "", telemetry.AllSources, ""
	if len(call.Raw) > 0 {
		socket = call.Raw[0]
	}
	if len(call.Raw) > 1 {
		sources = call.Raw[1]
	}
	if len(call.Raw) > 2 {
		caller = call.Raw[2]
	}
	if code := telemetry.RunTap(socket, sources, caller); code != 0 {
		return &ExitCodeError{Code: code}
	}
	return nil
}

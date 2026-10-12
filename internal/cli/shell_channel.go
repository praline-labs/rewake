package cli

import (
	"errors"
	"os"

	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The shell's evidence for the mail channel
// (docs/mail-bridge-channel.md#the-shell-observation): a mail operation run
// in the shell that wrote, or that failed reaching the state, leaves one
// observation for its run's wrapper. A call through the tool never gets
// here: its evidence is its ticket.

// shellOperations are the commands whose outcome is the shell's evidence;
// the switches name the forms of them that write nothing to observe. A view
// is never observed (observeShell), so --owed and --awaited need no entry:
// --peek is a change, for its lock, and writes nothing.
var shellOperations = map[string][]string{
	"inbox":   {"peek"},
	"pending": nil,
	"send":    nil,
	"retry":   nil,
}

// observeShell writes what a shell call met, while the run's channel is not
// working through the tool. Only a change form is observed: a view writes
// nothing, its observation included.
func observeShell(class access, call Call, err error) {
	if call.Command == nil || class == accessView {
		return
	}
	readOnly, operation := shellOperations[call.Command.Name]
	if !operation {
		return
	}
	for _, flag := range readOnly {
		if call.Switch(flag) {
			return
		}
	}
	reached := state.Reached()
	// A send accepted but not delivered yet wrote its message: the shell
	// reached the state, whatever the delivery does after.
	var pending *PendingError
	accepted := err == nil || errors.As(err, &pending)
	var note receipt.ShellNote
	switch {
	case !accepted && reached.Failed != nil:
		note.Class = receipt.ShellClass(reached.Failed)
		note.Boot, note.Wall = reached.FailedAt.Boot, reached.FailedAt.Wall
	case accepted && reached.Wrote:
		note.OK = true
		note.Boot, note.Wall = reached.WroteAt.Boot, reached.WroteAt.Wall
	default:
		// A refusal, a wait, a hold, or nothing written: no evidence.
		return
	}
	name, epoch := os.Getenv(state.SessionEnv), os.Getenv(state.EpochEnv)
	dir, derr := state.Dir()
	if derr != nil || name == "" || epoch == "" {
		return
	}
	// A record that cannot be read is no proof the tool works: write.
	if record := session.Load(dir, name, epoch).Channel; record != nil && record.Working() {
		return
	}
	receipt.WriteShell(dir, name, epoch, note)
}

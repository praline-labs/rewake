package wrap

import (
	"context"
	"time"

	"github.com/praline-labs/rewake/internal/cutover"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

// lookForWriters lists the machine's processes once and answers the check a
// launch passes for the name it chooses (docs/protocol-cutover.md). A list
// that cannot be taken refuses the launch: no writer is then proven stopped.
// Replaceable, so the launch's tests do not depend on this machine's
// processes.
var lookForWriters = func() (func(dir, name string) error, error) {
	scanner, err := cutover.Here()
	if err != nil {
		return nil, err
	}
	found, err := scanner.Look()
	if err != nil {
		return nil, err
	}
	return func(dir, name string) error { return cutover.Check(found, dir, name) }, nil
}

// claimRun lists the machine's processes once, before the room lock: a pass
// over /proc takes a moment, and only its verdict for the chosen name is taken
// under the lock.
func claimRun(request Request, self int, selfStart uint64, boot, cwd string) (registry.Session, error) {
	writers, err := lookForWriters()
	if err != nil {
		return registry.Session{}, err
	}
	return claimName(request, self, selfStart, boot, cwd, writers)
}

// heldWait bounds the fifth step: a sender busy past it keeps the report
// held, and its next barrier takes it.
const heldWait = 2 * time.Second

func takeHeld(parent context.Context, dir string, session registry.Session) {
	ctx, cancel := context.WithTimeout(parent, heldWait)
	defer cancel()
	inbox.TakeHeld(ctx, dir, session.Name)
	if session.Role == role.Main.ID {
		noteEarlierRuns(dir)
	}
}

// noteEarlierRuns gives a main of this build, as it starts, the note a
// refusal would have sent it for every run of the earlier build still
// running: one main of the earlier build heard nothing until it was resumed.
// The note's id is the run's, so it is sent once whoever sends it.
func noteEarlierRuns(dir string) {
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return
	}
	for _, session := range sessions {
		if session.EarlierBuild() && registry.ObserveRun(session.Epoch()) != proc.IdentityEnded {
			_ = inbox.TellMainUpgraded(dir, session.Name, session.Epoch())
		}
	}
}

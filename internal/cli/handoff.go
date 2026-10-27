package cli

import (
	"fmt"
	"os"
	"syscall"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// execImage replaces this process with another image; a test sees the call
// instead.
var execImage = syscall.Exec

// handOff gives the call to the room's own copy of its build when the room
// runs another build than this one and keeps a copy of it
// (docs/rules/state.md#the-rooms-own-copy): a person's rewake upgraded under
// a running room, or main's shell started from the install path. The copy
// takes the same arguments, environment and pid, and checks its lease itself.
// It returns only when there is nothing to hand off or the exec failed, and
// the call then goes on as this build's: refused, or a view saying the
// state is another build's.
func (ctx *Context) handOff() {
	if len(ctx.argv) == 0 {
		return
	}
	room, err := state.Dir()
	if err != nil {
		return
	}
	path, theirs, ok := build.Handoff(room)
	if !ok {
		return
	}
	_, _ = fmt.Fprintln(ctx.Stderr, build.HandoffLine(room, theirs))
	err = execImage(path, append([]string{path}, ctx.argv...), os.Environ())
	_, _ = fmt.Fprintf(ctx.Stderr, "Rewake: the room's own copy %s could not be started (%v).\n", path, err)
}

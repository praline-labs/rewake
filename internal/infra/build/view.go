package build

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Foreign is the line a view of a room adds when the room's build is not
// this one (docs/v2/stage3-state.md#one-build-per-room), or "" when the room
// has no build yet or it is this build. A view takes no lease and refuses
// nothing: another build's state is shown as this build reads it, and said
// to be another's. A build that cannot be read, the room's or this
// process's, is unknown — never taken for the same one.
func Foreign(room string) string {
	name := filepath.Base(room)
	theirs, err := state.ReadBuildRecord(room)
	if errors.Is(err, state.ErrNoBuildRecord) {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("Rewake: the build of room %s is unknown (%v); what follows is how this build reads its state.", name, err)
	}
	own, err := ID()
	if err != nil {
		return fmt.Sprintf("Rewake: room %s belongs to build %s and the build of this rewake is unknown (%v); what follows is how this rewake reads its state.", name, Short(theirs), err)
	}
	if own == theirs {
		return ""
	}
	return fmt.Sprintf("Rewake: room %s belongs to build %s and this is build %s; what follows is how this build reads its state.", name, Short(theirs), Short(own))
}

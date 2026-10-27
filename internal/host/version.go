package host

import (
	"fmt"
	"os"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/infra/build"
)

// readVersion is the launch's read of its harness's version, through the
// adapter's gate; its error refuses the launch before the claim. The program
// read is the one the launch will start: the shipped copy it locates, unless
// --command names another, which is held to the minimum alone.
func readVersion(request Request, cwd string) (string, adapter.Start, error) {
	if shipped, ok := request.Harness.(adapter.Shipped); ok && request.Command == "" {
		start, err := shipped.Locate()
		if err != nil {
			return "", adapter.Start{}, err
		}
		version, err := launch.GateShipped(request.Harness, start, launch.ProbeEnv(), cwd)
		if err != nil && runsRoomCopy(request.Dir) {
			// The copy found its harness in the install it came from, which an
			// upgrade has replaced since (docs/rules/state.md#the-rooms-own-copy).
			err = fmt.Errorf("%w; this room runs the build of its launch, and rewake was upgraded since: restarting main moves the room to the new one", err)
		}
		return version, start, err
	}
	program := adapter.LaunchRequest{Command: request.Command}.Program(request.Harness.ID())
	version, err := launch.Gate(request.Harness, program, launch.ProbeEnv(), cwd)
	return version, adapter.Start{}, err
}

// runsRoomCopy says whether this process is room's own copy of its build.
func runsRoomCopy(room string) bool {
	self, err := os.Executable()
	return err == nil && self == build.Pinned(room)
}

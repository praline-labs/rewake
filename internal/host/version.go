package host

import (
	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/launch"
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
		return version, start, err
	}
	program := adapter.LaunchRequest{Command: request.Command}.Program(request.Harness.ID())
	version, err := launch.Gate(request.Harness, program, launch.ProbeEnv(), cwd)
	return version, adapter.Start{}, err
}

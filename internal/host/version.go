package host

import (
	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/launch"
)

// readVersion is the launch's read of its harness's version, through the
// adapter's gate; its error refuses the launch before the claim. The program
// read is the one the launch will start.
func readVersion(request Request, cwd string) (string, error) {
	program := adapter.LaunchRequest{Command: request.Command}.Program(request.Harness.ID())
	return launch.Gate(request.Harness, program, launch.ProbeEnv(), cwd)
}

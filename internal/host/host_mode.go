package host

import (
	"os"
	"os/exec"

	"github.com/praline-labs/rewake/internal/host/hostmode"
	"github.com/praline-labs/rewake/internal/host/spacehost"
)

// screen is what holds the person's terminal for a run: nothing in an
// ordinary run, the host in the host mode.
type screen struct{ host *hostmode.Host }

// Stop gives the person's terminal back; it may be called more than once.
func (s screen) Stop() {
	if s.host != nil {
		s.host.Stop()
	}
}

// startHarness starts the harness and says how its stops are followed.
//
// An ordinary run gives the harness the person's terminal and keeps it in the
// wrapper's process group, which is what makes the terminal treat it as the
// program it is: Ctrl+C, Ctrl+Z, window size and job control all work as they
// would without rewake in between. Giving it a group of its own and handing
// over the terminal was tried, and it broke exactly that — Ctrl+Z left the
// terminal with a stopped harness, and starting rewake in the background took
// the terminal away from the shell.
//
// The host mode holds the person's terminal itself, so the harness runs on
// the host's, in a session of its own, and a stop is continued at once.
func startHarness(request Request, command *exec.Cmd, epoch string) (screen, func(int), error) {
	if !request.Host {
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
		return screen{}, followJob, command.Start()
	}
	h, err := hostmode.Open(hostmode.Config{
		In: os.Stdin, Out: os.Stdout, Env: os.Environ(),
		Panel: spacehost.Panel{Dir: request.Dir, Main: epoch},
	})
	if err != nil {
		return screen{}, nil, err
	}
	if err := h.Prepare(command); err != nil {
		h.Stop()
		return screen{}, nil, err
	}
	if err := command.Start(); err != nil {
		h.Stop()
		return screen{}, nil, err
	}
	if err := h.Start(); err != nil {
		h.Stop()
		_ = command.Process.Kill()
		_ = command.Wait()
		return screen{}, nil, err
	}
	return screen{host: h}, continueHarness, nil
}

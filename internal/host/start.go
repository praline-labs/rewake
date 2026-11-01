package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/delivery"
	"github.com/praline-labs/rewake/internal/host/mainhost"
)

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
// A main in rewake's shell has a stop continued at once: its terminal is the
// shell's pane, and no shell behind it has an fg to bring a stopped job back.
// Its suspend is answered too (continueSuspends), since on that pane the
// kernel never carries it out.
func startHarness(request Request, command *exec.Cmd) (func(int), error) {
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	onStop := followJob
	var suspends chan os.Signal
	if request.Shell {
		onStop = continueHarness
		suspends = catchSuspends()
	}
	if err := command.Start(); err != nil {
		return onStop, err
	}
	if suspends != nil {
		go continueSuspends(suspends, command.Process.Pid)
	}
	return onStop, nil
}

// tellShellClosing writes the closing mark (mainhost.ClosingMark) on the
// wrapper's standard output, main's pane, when the harness says it has begun
// to end its process; the harness writes there too, and the kernel keeps one
// write whole between two of the harness's, so the mark lands between its
// writes.
func tellShellClosing() { _, _ = os.Stdout.WriteString(mainhost.ClosingMark) }

// runHarness starts the plan's program, or adopts the one an earlier image
// of this wrapper started (reexec.go).
func runHarness(request Request, plan adapter.LaunchPlan) (*os.Process, func(int), error) {
	if request.Handover != nil {
		return adoptHarness(request)
	}
	command := exec.Command(plan.Command, plan.Args...)
	command.Env = plan.Env
	onStop, err := startHarness(request, command)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, nil, fmt.Errorf("%s is not installed or not in PATH", plan.Command)
		}
		return nil, nil, err
	}
	return command.Process, onStop, nil
}

// serveHandingOver serves until ctx ends; a server stopped for an exec that
// failed serves again.
func serveHandingOver(ctx context.Context, server *delivery.Server, resume <-chan struct{}) {
	for {
		server.Serve(ctx)
		select {
		case <-ctx.Done():
			return
		case <-resume:
		}
	}
}

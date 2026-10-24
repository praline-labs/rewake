package host

import (
	"os"
	"os/exec"
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

// shellClosing is the private sequence main's wrapper writes on its terminal,
// the shell's pane, when the harness says it has begun to end its process:
// the shell's scanner reads it in main's stream and starts its closing screen,
// or keeps the one main's leave of its screen began (docs/shell.md). The OSC number is one no terminal or program
// is known to use; shell/src/scan.ts reads the same bytes.
const shellClosing = "\x1b]6262;rewake-closing\x07"

// tellShellClosing writes shellClosing on the wrapper's standard output,
// main's pane, which the harness writes too: the kernel keeps one write whole
// between two of the harness's, so the mark lands between its writes.
func tellShellClosing() { _, _ = os.Stdout.WriteString(shellClosing) }

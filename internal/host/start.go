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
func startHarness(request Request, command *exec.Cmd) (func(int), error) {
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	onStop := followJob
	if request.Shell {
		onStop = continueHarness
	}
	return onStop, command.Start()
}

// shellClosing is the private sequence main's wrapper writes on its terminal,
// the shell's pane, when the harness says it has begun to end its process:
// the shell's scanner takes it out of main's stream and starts its closing
// screen (docs/v2/shell.md). The OSC number is one no terminal or program
// is known to use; shell/src/scan.ts reads the same bytes.
const shellClosing = "\x1b]6262;rewake-closing\x07"

// tellShellClosing writes shellClosing on the wrapper's standard output,
// main's pane, which the harness writes too: the kernel keeps one write whole
// between two of the harness's, so the mark lands between its writes.
func tellShellClosing() { _, _ = os.Stdout.WriteString(shellClosing) }

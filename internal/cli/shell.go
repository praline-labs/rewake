package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

const (
	// shellEnv names the shell to run in place of the one beside rewake:
	// the person's own copy, run from its sources (docs/shell.md).
	shellEnv = "REWAKE_SHELL"
	// shellProgram is the shell's executable beside rewake's.
	shellProgram = "rewake-shell"
	// shellMainEnv tells the launch the shell starts that it is main's in
	// the shell, so it runs main rather than another shell. The launch takes
	// it out of its environment, and the harness never sees it.
	shellMainEnv = "REWAKE_SHELL_MAIN"
)

// enterShell starts the shell for main on a terminal (the owner's decision,
// October 19, 2026): the launch's own process becomes the shell, which runs
// this same command line again on a terminal of its own, and that run is
// main's. It returns only for that second run, saying it is in the shell, or
// for a launch the shell does not take: a worker, a harness whose main opens
// no spaces, or a launch whose standard input or output is not a terminal —
// a script, a pipe, a print mode.
//
// The shell is exec'd rather than run as a child, so no rewake process
// stays between the person's terminal and the shell: when the terminal goes
// the shell hears it, ends main, and nothing lives on unseen.
func enterShell(h adapter.Harness, id launchIdentity) (bool, error) {
	inside := os.Getenv(shellMainEnv) == "1"
	_ = os.Unsetenv(shellMainEnv)
	if !id.part.Spaces || !h.Offers().Spaces {
		return false, nil
	}
	if inside {
		return true, nil
	}
	if !terminal.IsTerminal(os.Stdin) || !terminal.IsTerminal(os.Stdout) {
		return false, nil
	}
	self, err := os.Executable()
	if err != nil {
		return false, failf("main runs in the shell, which runs this rewake again, and its own path is unknown: %v", err)
	}
	program, err := findShell(self)
	if err != nil {
		return false, &FailedError{Message: err.Error()}
	}
	// The room, for the shell's own reads of it.
	argv := append([]string{program, "--room", id.room, "--", self}, os.Args[1:]...)
	env := append(os.Environ(), shellMainEnv+"=1")
	return false, failf("could not start the shell %s: %v", program, syscall.Exec(program, argv, env))
}

// findShell is REWAKE_SHELL, looked up on PATH unless it holds a slash, or
// rewake-shell beside rewake.
func findShell(self string) (string, error) {
	if named := os.Getenv(shellEnv); named != "" {
		program, err := exec.LookPath(named)
		if err != nil {
			return "", errors.New("main runs in the shell, " + shellEnv + "=" + named + ", which is not an executable program: " + err.Error())
		}
		return filepath.Abs(program)
	}
	program := filepath.Join(filepath.Dir(self), shellProgram)
	if _, err := exec.LookPath(program); err != nil {
		return "", errors.New("main on a terminal runs in the shell, " + program + ", which is not there; install it beside rewake or set " + shellEnv + " to the shell's program")
	}
	return program, nil
}

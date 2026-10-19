package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/praline-labs/rewake/internal/infra/state"
)

const (
	// shellEnv names the shell to run in place of the one beside rewake:
	// the person's own copy, run from its sources (docs/v2/shell.md).
	shellEnv = "REWAKE_SHELL"
	// shellProgram is the shell's executable beside rewake's.
	shellProgram = "rewake-shell"
	// shellMainEnv tells the launch the shell starts that it is main's in
	// the shell, so it runs main rather than another shell. The launch takes
	// it out of its environment, and the harness never sees it.
	shellMainEnv = "REWAKE_SHELL_MAIN"
)

// enterShell starts the shell for a launch with --shell: the launch's own
// process becomes the shell, which runs this same command line again on a
// terminal of its own, and that run is main's. It returns only for that
// second run, saying it is in the shell, or for a launch without --shell.
//
// The shell is exec'd rather than run as a child, so no rewake process
// stays between the person's terminal and the shell: when the terminal goes
// the shell hears it, ends main, and nothing lives on unseen.
func enterShell(call Call) (bool, error) {
	if !call.Switch("shell") {
		return false, nil
	}
	if os.Getenv(shellMainEnv) == "1" {
		_ = os.Unsetenv(shellMainEnv)
		return true, nil
	}
	self, err := os.Executable()
	if err != nil {
		return false, failf("--shell runs this rewake again inside the shell, and its own path is unknown: %v", err)
	}
	program, err := findShell(self)
	if err != nil {
		return false, &UsageError{Command: call.Command, Message: err.Error()}
	}
	// The room, for the shell's own reads of it: the launch takes it from
	// the flag alone.
	argv := append([]string{program, "--room", call.Flag("room", state.DefaultRoom), "--", self}, os.Args[1:]...)
	env := append(os.Environ(), shellMainEnv+"=1")
	return false, failf("could not start the shell %s: %v", program, syscall.Exec(program, argv, env))
}

// findShell is REWAKE_SHELL, looked up on PATH unless it holds a slash, or
// rewake-shell beside rewake.
func findShell(self string) (string, error) {
	if named := os.Getenv(shellEnv); named != "" {
		program, err := exec.LookPath(named)
		if err != nil {
			return "", errors.New("--shell runs " + shellEnv + "=" + named + ", which is not an executable program: " + err.Error())
		}
		return filepath.Abs(program)
	}
	program := filepath.Join(filepath.Dir(self), shellProgram)
	if _, err := exec.LookPath(program); err != nil {
		return "", errors.New("--shell needs the shell, " + program + ", which is not there; set " + shellEnv + " to the shell's program")
	}
	return program, nil
}

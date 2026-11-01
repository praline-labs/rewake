package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/mainhost"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/state"
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
	// shellSocketEnv is where the shell listens for rewake screen
	// (docs/screen.md): the room's sock/shell.sock, whose folder main's
	// launch makes. Only the shell's environment carries it.
	shellSocketEnv = "REWAKE_SHELL_SOCK"
)

// enterShell runs main in the shell on a terminal (the owner's decisions,
// October 19 and November 1, 2026): the launch's own process becomes main's
// host in the pane (internal/host/mainhost), which runs this same command
// line again on a terminal of its own — that run is main's — and the shell
// as its child, which it can replace while main runs on. It returns only for
// that second run, saying it is in the shell, or for a launch the shell does
// not take: a worker, a harness whose main opens no spaces, or a launch whose
// standard input or output is not a terminal — a script, a pipe, a print
// mode. For the host it returns main's exit code as an ExitCodeError.
//
// The host stays the pane's process, so nothing of rewake's lives apart from
// the person's terminal: when the terminal goes the host hears it, ends main,
// and nothing lives on unseen.
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
	base, err := state.Base()
	if err != nil {
		return false, &UsageError{Message: err.Error()}
	}
	dir, err := state.Dir()
	if err != nil {
		return false, &UsageError{Message: err.Error()}
	}
	command := append([]string{self}, os.Args[1:]...)
	code := mainhost.Run(mainhost.Config{
		Shell: program,
		// The room, for the shell's own reads of it, and main's command line,
		// whose rewake the shell calls for them.
		ShellArgs: append([]string{"--room", id.room, "--"}, command...),
		ShellEnv:  []string{shellSocketEnv + "=" + state.SocketPath(dir, "shell", "")},
		Main:      command,
		MainEnv:   mainEnv(),
		Base:      base,
		Room:      id.room,
		Dir:       dir,
	})
	return false, &ExitCodeError{Code: code}
}

// mainEnv is what main's environment adds to the host's: the mark that it is
// main's run in the shell, and pi's switch to copy through the terminal.
func mainEnv() []string {
	env := []string{shellMainEnv + "=1"}
	// pi copies through the machine's clipboard where it finds one, which
	// never reaches the person's terminal from inside the shell (on WSL
	// without WT_SESSION, PowerShell); this makes it send OSC 52 as well,
	// which the shell's scanner carries out. Main's workers inherit it.
	if _, set := os.LookupEnv("PI_CLIPBOARD_OSC52"); !set {
		env = append(env, "PI_CLIPBOARD_OSC52=1")
	}
	return env
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

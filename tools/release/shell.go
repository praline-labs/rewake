package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter/pi"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// shellMarker is what the installed shell's main prints; the shell draws it
// in main's pane, and the gate looks for it in what the terminal received.
const shellMarker = "rewake-release-shell-check"

// shellHints is the start of the hints row only the shell draws, so a main
// that ran alone in the plain terminal, its marker shown all the same, is no
// pass.
const shellHints = "keys go to main"

// standInPi answers the version gate as the pinned pi and then is main: it
// prints the marker and ends a second later, since the shell ends with main
// and the output of a main gone before the shell's first frame is never drawn.
const standInPi = "#!/bin/sh\ncase \"$1\" in --version) echo %s; exit 0;; esac\necho " + shellMarker + "\nsleep 1\n"

// installedShell requires npm to have put the compiled shell beside the
// installed binary, where rewake looks for it (internal/cli/shell.go), and
// that shell to draw main's output when the installed command starts main on
// a terminal, as a person's launch does: the launch becomes main's host,
// which runs the shell on its channel (docs/shell.md#the-hosts-channel). The
// shell runs only under that host since 85ffc35. main is a stand-in pi named
// with --command, so no login and no model take part.
func (g *gate) installedShell(ctx context.Context, command, modules string, p pkg, scratch string) {
	ref := p.ref(g.version)
	for _, dir := range []string{filepath.Join(modules, packageName, "node_modules", p.alias()), filepath.Join(modules, p.alias())} {
		if _, err := os.Stat(filepath.Join(dir, "bin", "rewake")); err != nil {
			continue
		}
		shell := filepath.Join(dir, "bin", "rewake-shell")
		if drawn, err := launchOnTerminal(ctx, command, filepath.Join(scratch, "shell-launch")); err != nil {
			g.fail("%s: the installed command did not start main in the shell %s: %v", ref, shell, err)
		} else if !strings.Contains(drawn, shellMarker) || !strings.Contains(drawn, shellHints) {
			g.fail("%s: the installed shell %s did not draw main's output: the terminal showed %.300q", ref, shell, drawn)
		} else {
			g.pass("%s: the installed command starts main in the shell beside bin/rewake, which draws main's output", ref)
		}
		// The face is a second entrypoint of the compiled shell, named in the
		// bundle by where face/ and shell/ lie; a name that moved breaks only
		// here, so the gate starts it and waits for its answer.
		if r := g.run(ctx, runLimit, scratch, shell, "--face-check"); r.ok() {
			g.pass("%s: the installed shell's own face starts and answers", ref)
		} else {
			g.fail("%s: the installed shell's own face did not answer: %s", ref, r.why())
		}
		return
	}
	g.fail("%s: npm installed no bin/rewake under %s, beside which the shell would be", ref, modules)
}

// launchOnTerminal starts command --command <stand-in> pi in a room, a home
// and a state directory of its own, on a terminal of 120x40, and answers the
// text the terminal received, its escape sequences taken out, once every
// process on it has ended.
func launchOnTerminal(ctx context.Context, command, dir string) (string, error) {
	home, work := filepath.Join(dir, "home"), filepath.Join(dir, "work")
	for _, d := range []string{home, work} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
	}
	// The state directory lies in the system's temporary directory, not
	// under dir: a pi's socket lies in it, and a path past 107 bytes refuses
	// the launch.
	state, err := os.MkdirTemp("", "rw-gate-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(state) }()
	// The version rewake pins, so the launch adds no note of another.
	standIn := filepath.Join(dir, "pi")
	if err := os.WriteFile(standIn, fmt.Appendf(nil, standInPi, pi.PinnedVersion), 0o755); err != nil {
		return "", err
	}
	window, err := terminal.OpenPlain(terminal.Size{Cols: 120, Rows: 40})
	if err != nil {
		return "", err
	}
	defer func() { _ = window.Close() }()
	ctx, cancel := context.WithTimeout(ctx, runLimit)
	defer cancel()
	launch := exec.CommandContext(ctx, command, "--command", standIn, "pi")
	launch.Dir = work
	launch.Env = append(launchEnv(), "HOME="+home, "REWAKE_DIR="+state, "REWAKE_WORKTREES="+filepath.Join(dir, "worktrees"), "TERM=xterm-256color")
	if err := window.Prepare(launch); err != nil {
		return "", err
	}
	if err := launch.Start(); err != nil {
		return "", err
	}
	_ = window.ReleaseSide()
	var drawn bytes.Buffer
	read := make(chan struct{})
	go func() { _, _ = io.Copy(&drawn, window.Master); close(read) }()
	waited := launch.Wait()
	select {
	case <-read:
	case <-ctx.Done():
	}
	if ctx.Err() != nil {
		return "", errors.New("no end within " + runLimit.String())
	}
	if waited != nil {
		return "", fmt.Errorf("%w: %s", waited, plain(drawn.Bytes()))
	}
	return plain(drawn.Bytes()), nil
}

// launchEnv is the gate's environment without anything that names a
// session or a pane of the person's: a launch inside a session refuses, a
// REWAKE_SHELL would replace the installed shell, and a pane's variables
// would have the launch report itself into the person's terminal.
func launchEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "REWAKE_") || strings.HasPrefix(name, "HERDR_") || name == "HOME" || name == "TERM" {
			continue
		}
		env = append(env, entry)
	}
	return env
}

// escape is one control sequence of what a terminal is sent.
var escape = regexp.MustCompile(`\x1b\[[0-9;?<>=]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[()][0-9A-B]|\x1b[=>78]`)

// plain is the text of what a terminal was sent.
func plain(sent []byte) string { return escape.ReplaceAllString(string(sent), "") }

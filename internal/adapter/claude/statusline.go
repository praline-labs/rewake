package claude

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
)

// The status line of every run with a mod (docs/claude.md#the-cache-ttl):
// Claude Code runs `rewake statusline` after each response and on a timer,
// with its own JSON on stdin. The command forwards what the input tells to
// the run's host over the mod socket and prints the person's own status line,
// if their settings.json names one, in its place. What it reads, posts and
// installs is statusline_contract.go; this file runs it.

// The two deadlines are separate and the two jobs run side by side: the
// telemetry is optional and must not spend the person's line's time, and the
// pane waits for the line's text. Variables so a test can shorten them.
var (
	statusLineTimeout = 5 * time.Second
	postTimeout       = 2 * time.Second
)

// statusLineGuardEnv marks the environment of the person's own command: a
// `rewake statusline` that finds it is that command calling us again, and
// prints and posts nothing, so a person's line that is ours cannot recurse.
const statusLineGuardEnv = "REWAKE_STATUSLINE_PASSTHROUGH"

var _ adapter.StatusLiner = claudeHarness{}

// StatusLine posts what the input tells of the session to the run's host and
// prints the person's own line. Either failing leaves the other alone.
func (claudeHarness) StatusLine(ctx context.Context, input []byte, out io.Writer) {
	if os.Getenv(statusLineGuardEnv) != "" {
		return
	}
	seen, parsed := parseLineInput(input)
	posted := make(chan struct{})
	go func() {
		defer close(posted)
		if !parsed {
			return
		}
		if body := seen.body(); body != (modhost.StatusLineRequest{}) {
			post, cancel := context.WithTimeout(ctx, postTimeout)
			defer cancel()
			postLine(post, os.Getenv(modSocketEnv), body)
		}
	}()
	line, cancel := context.WithTimeout(ctx, statusLineTimeout)
	defer cancel()
	personal(line, input, seen.projectDir(), out)
	<-posted
}

// personal runs the person's own status line — the one their settings name
// for the session's project (ownStatusLine) — with the same input, and
// copies its output: the run's own statusLine shadows it, and the person's
// line should not be lost to the cache's.
func personal(ctx context.Context, input []byte, project string, out io.Writer) {
	line := ownStatusLine(os.Getenv(configDirEnv), project)
	if line.Type != "command" || line.Command == "" {
		return
	}
	// The command runs in a process group of its own with its output in a
	// pipe this process reads: at the deadline the whole group is killed and
	// the pipe closed, so nothing a child still holds keeps the harness's
	// output open, and only what came in time is printed.
	var printed bytes.Buffer
	command := exec.CommandContext(ctx, "sh", "-c", line.Command)
	command.Stdin = bytes.NewReader(input)
	command.Stdout = &printed
	command.Env = append(os.Environ(), statusLineGuardEnv+"=1")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.WaitDelay = 500 * time.Millisecond
	_ = command.Run()
	_, _ = out.Write(printed.Bytes())
}

package codex

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// noticeMark opens a notice in a Codex session. Codex shows it as an ordinary
// message, and in a long conversation a plain line is easy to scroll past; a
// colored circle is not. Green matches the circle Claude Code draws in front of
// the same notice, so rewake looks the same in both.
const noticeMark = "🟢"

// queue is the call to the Codex CLI, replaceable in tests.
//
// The deadline has to bind the wait as well as the process. CombinedOutput waits
// for the pipes to close, and a child that outlives the command keeps them open:
// measured, a call with a 100 ms deadline returned after two seconds because the
// grandchild was still holding them. WaitDelay is what stops that.
var queue = func(ctx context.Context, home, thread, text string) (string, error) {
	command := exec.CommandContext(ctx, "codex", "queue", "--thread", thread, "--message", text)
	command.Env = append(os.Environ(), "CODEX_HOME="+home)
	command.WaitDelay = 2 * time.Second
	// Its own process group, so the deadline reaches whatever it started. The
	// command may be a launcher, and killing only the launcher left a child that
	// went on to queue the message after delivery had been reported failed.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			return command.Process.Kill()
		}
		return nil
	}

	var output strings.Builder
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	return output.String(), err
}

// classify turns a failed codex queue call into a result the sender can act on.
func classify(output string, err error) inbox.Result {
	text := strings.TrimSpace(output)
	switch {
	case strings.Contains(text, "no rollout found"):
		// The thread exists but holds no conversation: Codex creates the file
		// behind it only once a message has been exchanged. Waiting is right —
		// the message lands after the session's first turn.
		return inbox.Result{
			State:  inbox.Pending,
			Detail: "this codex session has not exchanged a message yet; delivery happens after its first turn",
		}
	case strings.Contains(text, "No active session found"):
		return inbox.Result{
			State:  inbox.Failed,
			Detail: "codex no longer knows this thread: " + firstLine(text),
		}
	case text != "":
		return inbox.Result{State: inbox.Failed, Detail: "codex queue refused the message: " + firstLine(text)}
	default:
		return inbox.Result{State: inbox.Failed, Detail: "codex queue failed: " + err.Error()}
	}
}

func noticePrefix(message inbox.Message) string {
	if inbox.KindOf(message) == inbox.Error {
		return "🔴"
	}
	return noticeMark
}

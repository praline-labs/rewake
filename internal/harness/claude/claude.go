// Package claude runs Claude Code as a rewake session and delivers messages to
// it through the inbox socket every session of it listens on.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// ID is the launch command and the harness field of a session record.
const ID = "claude"

// socketFlag asks Claude Code to put its inbox socket where we can name it. The
// flag is undocumented but stable since 2.1.224; without it the socket lands
// under a path derived from the pid, which we would then have to discover.
const socketFlag = "--messaging-socket-path"

// introFlag adds to the system prompt for one launch. It appends, so the user's
// own configuration is untouched.
const introFlag = "--append-system-prompt"

// toolFlag allows the rewake commands without a confirmation prompt for this
// launch. Without it the agent can be messaged but cannot answer until a person
// approves each reply.
const toolFlag = "--allowedTools"

// childMarkers are the variables a Claude Code session exports to its children.
// Inherited by a new session they point it at the parent's socket and switch its
// transcript off, so a session started from inside another one must not see them.
var childMarkers = []string{
	"CLAUDECODE",
	"CLAUDE_CODE_CHILD_SESSION",
	"CLAUDE_CODE_SESSION_ID",
	"CLAUDE_CODE_BRIDGE_SESSION_ID",
	"CLAUDE_CODE_ENTRYPOINT",
	"CLAUDE_CODE_EXECPATH",
	"CLAUDE_CODE_MESSAGING_SOCKET",
	"CLAUDE_CODE_MESSAGING_TOKEN",
	"CLAUDE_PID",
	"CLAUDE_PLUGIN_DATA",
	"CLAUDE_EFFORT",
	"CLAUDE_CODE_SUBAGENT_MODEL",
}

// dialTimeout bounds one delivery attempt. The socket is local, so a connection
// that takes longer than this is not slow, it is wrong.
const dialTimeout = 2 * time.Second

type claudeHarness struct{}

// New returns the Claude Code harness.
func New() harness.Harness { return claudeHarness{} }

func (claudeHarness) ID() string    { return ID }
func (claudeHarness) Title() string { return "Claude Code" }

func (claudeHarness) Summary() string {
	return "Start Claude Code as a rewake session. Messages reach it in seconds."
}

func (claudeHarness) Examples() []string {
	return []string{
		"rewake claude",
		"rewake --name api claude --model haiku",
	}
}

func (claudeHarness) Notes() []string {
	return []string{
		"Delivery goes through the session inbox socket, so a message arrives within seconds and wakes an idle session.",
		"Arguments after the harness name are passed to claude untouched, with one exception: a --help written first asks rewake for this page instead of starting the harness.",
		"An identical message from the same sender within thirty seconds is dropped by Claude Code itself; rewake puts a short id in every message to keep them apart.",
	}
}

func (claudeHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	args := append([]string{}, request.Args...)
	socket := request.Socket
	owns := false

	if harness.HasFlag(args, socketFlag) {
		// The caller named their own socket. Their flag wins, and delivery has
		// to find it from the record, so read it back rather than guess. It is
		// theirs, not ours: this session never removes that file.
		socket = flagValue(args, socketFlag)
	} else {
		if len(socket) > maxSocketPath {
			return harness.LaunchPlan{}, fmt.Errorf("the socket path %s is longer than the %d bytes a unix socket allows; use a shorter session name or REWAKE_DIR", socket, maxSocketPath)
		}
		// A leftover file from a session that died keeps Claude Code from
		// binding the same path.
		if err := removeStaleSocket(socket); err != nil {
			return harness.LaunchPlan{}, err
		}
		owns = true
		args = harness.AddFlags(args, socketFlag, socket)
	}

	if request.Intro && !harness.HasFlag(args, introFlag) {
		args = harness.AddFlags(args, introFlag, harness.Intro(request.Name))
	}
	if !harness.HasFlag(args, toolFlag) {
		args = harness.AddFlags(args, toolFlag, "Bash(rewake:*)")
	}

	return harness.LaunchPlan{
		Command:    "claude",
		Args:       args,
		Env:        harness.SessionEnv(request, childMarkers),
		Socket:     socket,
		OwnsSocket: owns,
	}, nil
}

// maxSocketPath is the limit the kernel puts on a unix socket path.
const maxSocketPath = 103

// maxLine is the longest line the session socket accepts, protocol side.
const maxLine = 1 << 20

func (claudeHarness) Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result {
	if session.Socket == "" {
		return inbox.Result{State: inbox.Failed, Detail: "this session has no inbox socket"}
	}

	line, err := json.Marshal(envelope{
		Type:     "user",
		Message:  payload{Role: "user", Content: harness.MessageText(message)},
		Priority: "next",
	})
	if err != nil {
		return inbox.Result{State: inbox.Failed, Detail: "the message could not be encoded: " + err.Error()}
	}
	// Claude Code drops a line longer than this and closes the connection, so a
	// write that succeeds would otherwise be reported as delivered while the
	// receiver never saw it. The limit is on the encoded line, not the text: JSON
	// escaping can grow a message several times over.
	if len(line)+1 > maxLine {
		return inbox.Result{
			State: inbox.Failed,
			Detail: fmt.Sprintf("the message is %d bytes once encoded, over the %d byte limit of the session socket; send less text or a path to a file",
				len(line)+1, maxLine),
		}
	}

	dialer := net.Dialer{Timeout: dialTimeout}
	connection, err := dialer.DialContext(ctx, "unix", session.Socket)
	if err != nil {
		// The socket appears a moment after the process does, and it is
		// recreated across a restart, so a session that is still alive gets
		// another attempt rather than a refusal.
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return inbox.Result{State: inbox.Pending, Detail: "the session is not listening yet"}
		}
		return inbox.Result{State: inbox.Failed, Detail: "could not reach the session: " + err.Error()}
	}
	defer connection.Close()

	_ = connection.SetWriteDeadline(time.Now().Add(dialTimeout))
	if _, err := connection.Write(append(line, '\n')); err != nil {
		return inbox.Result{State: inbox.Failed, Detail: "could not write to the session: " + err.Error()}
	}
	return inbox.Result{State: inbox.Delivered, Via: "socket"}
}

// envelope is one line of the session inbox protocol. Priority "next" puts the
// message after the tool call in flight and starts a turn when the session is
// idle, which is what a message from a peer should do.
type envelope struct {
	Type     string  `json:"type"`
	Message  payload `json:"message"`
	Priority string  `json:"priority"`
}

type payload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// removeStaleSocket clears a socket file left by a session that is gone.
//
// Only one error proves the owner is gone: connection refused, which the kernel
// returns when a socket file has no listener behind it. Every other failure —
// a full backlog, a timeout, a permission error — describes this moment, not the
// owner, and deleting the file then cuts off a session that is still running.
func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket; remove it or use another session name", path)
	}

	connection, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err == nil {
		connection.Close()
		return fmt.Errorf("%s is a live socket; another session is using this name", path)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("%s exists and could not be checked (%v); leaving it alone", path, err)
	}
	return os.Remove(path)
}

// flagValue reads the value of a flag the caller passed.
func flagValue(args []string, flag string) string {
	for index, arg := range args {
		if arg == flag && index+1 < len(args) {
			return args[index+1]
		}
		if len(arg) > len(flag)+1 && arg[:len(flag)+1] == flag+"=" {
			return arg[len(flag)+1:]
		}
	}
	return ""
}

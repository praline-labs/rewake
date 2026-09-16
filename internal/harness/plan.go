package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// LaunchRequest is what the wrapper knows before starting a harness.
type LaunchRequest struct {
	// Name is the session name already claimed for this run.
	Name string
	// Dir is the state directory.
	Dir string
	// Args are the caller's arguments, to be passed through untouched.
	Args []string
	// Intro asks for the short briefing that tells the agent it runs under
	// rewake and how to answer.
	Intro bool
	// Socket is the path the harness should use for its inbox socket, when it
	// has one.
	Socket string
}

// LaunchPlan is how the wrapper starts the harness.
type LaunchPlan struct {
	// Command is the executable to run, looked up in PATH.
	Command string
	// Args is the full argument list.
	Args []string
	// Env is the complete environment of the child.
	Env []string
	// Socket, when set, is recorded in the session so senders can reach it.
	Socket string
	// CodexHome, when set, is recorded so delivery uses the same state.
	CodexHome string
}

// Harness describes one coding-agent CLI: how it is presented, how it is
// started, and how a message reaches a running session of it.
type Harness interface {
	// ID is the launch command and the value stored in a session record.
	ID() string
	// Title is the human name, used in prose.
	Title() string
	// Summary is the one-line guide entry. Says what starting it gives you.
	Summary() string
	// Examples are real invocations, copied verbatim by whoever reads help.
	Examples() []string
	// Notes are decisions and limits worth knowing before starting it.
	Notes() []string
	// Launch turns a request into the command to run.
	Launch(request LaunchRequest) (LaunchPlan, error)
	// Deliver hands one message to a running session and says what happened.
	Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result
}

// Intro is the briefing handed to an agent at launch. It is short on purpose: it
// costs context in every turn of the session, so it says who you are, how to
// reach the others, and what an incoming message looks like — nothing else.
func Intro(name string) string {
	return strings.Join([]string{
		fmt.Sprintf("You are running inside rewake as the session %q.", name),
		"Other agent sessions on this machine can message you, and you can message them.",
		"- rewake list — who is running",
		`- rewake send <name> "text" — deliver text to a session`,
		`Incoming messages start with "[rewake] message from: <name>" and end with the exact command to answer them.`,
	}, "\n")
}

// MessageText is what the receiving agent reads.
//
// The shape is dictated by how agents misread the first version: a header of
// "from <name> · <id>" was copied whole into the reply, which then addressed a
// session called "shell · 33f2". So the name stands alone on its own line, the
// id sits in brackets, and the reply line is a command that can be run as
// written. The id is there because Claude Code drops a repeat of identical text
// from the same sender within thirty seconds, and a dropped message is silence.
func MessageText(message inbox.Message) string {
	body := fmt.Sprintf("[rewake] message from: %s (id %s)\n\n%s", message.From, shortID(message.ID), message.Text)
	if message.From == ShellSender {
		return body + "\n\n[rewake] The sender is a plain shell, not a session, and cannot be replied to."
	}
	return body + fmt.Sprintf("\n\n[rewake] To answer, run: rewake send %s \"your reply\"", message.From)
}

// ShellSender is the sender name used when a message comes from a shell that is
// not a rewake session.
const ShellSender = "shell"

func shortID(id string) string {
	if len(id) <= 4 {
		return id
	}
	return id[len(id)-4:]
}

// SessionEnv returns the environment for a harness: the current one, with the
// markers of a parent agent session removed and the rewake variables added.
func SessionEnv(request LaunchRequest, strip []string) []string {
	drop := map[string]bool{
		state.SessionEnv: true,
		state.DirEnv:     true,
	}
	for _, name := range strip {
		drop[name] = true
	}

	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		if found && drop[name] {
			continue
		}
		if found && name == "PATH" {
			entry = "PATH=" + pathWithSelf(value)
		}
		env = append(env, entry)
	}
	return append(env,
		state.SessionEnv+"="+request.Name,
		state.DirEnv+"="+request.Dir,
	)
}

// pathWithSelf makes sure the agent can run the same rewake that started it.
// Found by a live run: the agent was told to answer with rewake send, tried, and
// got "command not found" because the binary was not on its PATH.
func pathWithSelf(path string) string {
	executable, err := os.Executable()
	if err != nil {
		return path
	}
	dir := filepath.Dir(executable)
	for _, entry := range filepath.SplitList(path) {
		if entry == dir {
			return path
		}
	}
	return dir + string(os.PathListSeparator) + path
}

// HasFlag reports whether the caller already passed a flag, in either the
// "--flag value" or the "--flag=value" form. A harness adds its own only when
// the caller has not: their flag is the one they meant.
func HasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

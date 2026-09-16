package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
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
	// Epoch identifies this run of the name.
	Epoch string
	// Role is what the session is for. A role that reports nothing gets no
	// end-of-turn hook.
	Role role.Role
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
	// OwnsSocket says this run created the socket path and may remove it when
	// the session ends. A path the caller named belongs to the caller: deleting
	// it could cut off a session that is still running on it.
	OwnsSocket bool
	// CodexHome, when set, is recorded so delivery uses the same state.
	CodexHome string
	// Notes are things the caller should know about this launch: a setting that
	// could not be read, a briefing that was skipped. They are printed once, to
	// stderr, and do not stop the launch.
	Notes []string
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

// Intro is the briefing handed to an agent at launch. It says what rewake is and
// where the instructions are, nothing more: it costs context in every turn, and
// the guide is one command away and always matches the binary.
func Intro(name string, part role.Role) string {
	if part.ID == "" {
		part = role.Default()
	}
	return strings.Join([]string{
		fmt.Sprintf("You are running inside rewake as the session %q.", name),
		"rewake lets agent sessions on this machine message each other; a message waiting for you is announced by a line with \"Rewake: <session> <kind>\".",
		"Run `rewake guide` before you send or read messages: it explains how.",
		part.Brief,
	}, "\n")
}

// Notice is the one line that announces waiting mail, the same for every
// harness: "Rewake: codex finished, 1 new message". It carries no text of the
// message on purpose. The agent fetches that itself, so it knows the message
// came through a tool, not from the person at the keyboard.
func Notice(message inbox.Message) string {
	count := message.Unread
	if count < 1 {
		count = 1
	}
	noun := "messages"
	if count == 1 {
		noun = "message"
	}
	return fmt.Sprintf("Rewake: %s %s, %d new %s", message.From, inbox.KindOf(message), count, noun)
}

// NoticeID is the part of the message id a notice carries. Claude Code drops
// identical text from the same sender within thirty seconds, and two notices of
// the same kind from the same session would otherwise be the same text.
func NoticeID(message inbox.Message) string {
	return "rewake-" + shortID(message.ID)
}

// TurnEnded is the hidden command a harness calls when a turn of its session
// ends. It is not in the guide: agents have no reason to run it.
const TurnEnded = "turn-ended"

// TurnEndedArgv is the command a harness runs at the end of a turn, by absolute
// path: the hook runs with whatever PATH the harness has at that moment.
func TurnEndedArgv() ([]string, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("could not find the rewake binary: %w", err)
	}
	return []string{executable, TurnEnded}, nil
}

// TurnEndedCommand is TurnEndedArgv as one shell command line.
func TurnEndedCommand() (string, error) {
	argv, err := TurnEndedArgv()
	if err != nil {
		return "", err
	}
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", `'\''`)+"'")
	}
	return strings.Join(quoted, " "), nil
}

// ShellSender is the sender name used when a message comes from a shell that is
// not a rewake session.
const ShellSender = "shell"

// shortID is the part of the id the receiver sees. Eight hex characters of the
// random tail, not four: the id only has to make otherwise identical messages
// different, and a four-character tail repeats often enough to matter.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[len(id)-8:]
}

// SessionEnv returns the environment for a harness: the current one, with the
// markers of a parent agent session removed and the rewake variables added.
func SessionEnv(request LaunchRequest, strip []string) []string {
	drop := map[string]bool{
		state.SessionEnv: true,
		state.EpochEnv:   true,
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
		state.EpochEnv+"="+request.Epoch,
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

// AddFlags inserts flags rewake adds into the caller's argument list, in front
// of a "--" terminator when there is one. Everything after that terminator is a
// positional argument of the harness, so appending there would turn a flag into
// part of a prompt.
func AddFlags(args []string, added ...string) []string {
	terminator := -1
	for index, arg := range args {
		if arg == "--" {
			terminator = index
			break
		}
	}
	if terminator < 0 {
		return append(append([]string{}, args...), added...)
	}
	out := append([]string{}, args[:terminator]...)
	out = append(out, added...)
	return append(out, args[terminator:]...)
}

// BeforeTerminator returns the arguments up to "--". What follows is input for
// the harness — a prompt, most often — and a flag-looking word in it is text,
// not a setting.
func BeforeTerminator(args []string) []string {
	for index, arg := range args {
		if arg == "--" {
			return args[:index]
		}
	}
	return args
}

// HasFlag reports whether the caller already passed a flag, in either the
// "--flag value" or the "--flag=value" form. A harness adds its own only when
// the caller has not: their flag is the one they meant.
func HasFlag(args []string, flag string) bool {
	for _, arg := range BeforeTerminator(args) {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

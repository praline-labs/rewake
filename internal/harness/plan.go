package harness

import (
	"context"

	"github.com/praline-labs/rewake/internal/brief"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

// LaunchRequest is what the wrapper knows before starting a harness.
type LaunchRequest struct {
	// Name is the session name already claimed for this run.
	Name string
	// Dir is the shared state root, never a room subdirectory.
	Dir string
	// RoomDir is the room's own directory, where the session's mail is.
	RoomDir string
	// Room is the isolated conversation this launch belongs to.
	Room string
	// RoleReason explains the explicit or automatic role selection.
	RoleReason string
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
	// Role determines reporting and additive permission requests.
	Role role.Role
	// Command, when set, is the program started in place of the harness's
	// own — a person's wrapper script around it, named with --command. It
	// replaces the program and nothing else: every argument, variable, socket
	// and setting the adapter adds is the same.
	Command string
	// ObservationSocket is where a harness without a backend of its own may
	// have its session's telemetry sent: a path of this run, like Socket.
	ObservationSocket string
	// ControlDir is where this run takes control requests, made by the
	// wrapper before the launch and removed after it; empty when it could not
	// be made (docs/remote-control.md).
	ControlDir string
	// GrantDirs are directories main confirmed again for the conversation
	// this launch resumes (Resumer); a harness that takes a directory at
	// launch is started with them.
	GrantDirs []string
}

// LaunchPlan is how the wrapper starts the harness.
type LaunchPlan struct {
	Backend Backend
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
	// Observer, when set, is a session-owned telemetry source for a harness
	// that has no Backend. The wrapper starts it before the harness and
	// publishes what it reports for as long as the session runs.
	Observer Observer
	// Lane, when set, delivers in place of the harness's Deliver for as long
	// as the session runs. Only for a harness without a Backend.
	Lane Lane
	// Notes are things the caller should know about this launch: a setting that
	// could not be read, a briefing that was skipped. They are printed once, to
	// stderr, and do not stop the launch.
	Notes []string
}

// Program is the executable a launch starts: the one named with --command, or
// the harness's own.
func (r LaunchRequest) Program(own string) string {
	if r.Command != "" {
		return r.Command
	}
	return own
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
	// SingleUseFlags are the flags this harness takes at most once, each
	// naming one parameter by all its spellings and saying whether it carries
	// a value.
	//
	// It exists because an alias and a launch line can both name the same
	// thing, and what happens then is the harness's property, not a rule we
	// get to make. Codex refuses to parse a repeated --model, while a repeated
	// --add-dir is how a second directory is added — so the first has to be
	// replaced and the second must not be. A flag missing from this list is
	// simply appended, which is the safe answer for anything unlisted.
	//
	// Returning nothing is therefore not a neutral answer: it says every flag
	// may be repeated, and for a harness that refuses repeats it turns an
	// alias plus a typed flag into a launch that will not parse.
	SingleUseFlags() []Flag
	// ProtectedDirs are where the harness keeps its own configuration,
	// credentials and sessions. rewake never grants a worker write access to
	// them, whichever harness the worker runs (docs/grants.md): a worker that
	// could write there could change its own permissions, or another
	// session's.
	ProtectedDirs() []string
	// Launch turns a request into the command to run.
	Launch(request LaunchRequest) (LaunchPlan, error)
	// Deliver hands one message to a running session and says what happened.
	Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result
}

// Steerable is a harness whose running sessions take a main's control
// requests — `rewake compact` and `rewake interrupt` — through their run's
// control directory (docs/remote-control.md). A harness that is not one is
// refused before anything is written.
type Steerable interface {
	// CompactFocus says whether a compaction may carry a focus: text the
	// summary is to keep. Where the harness has no way to pass one for a
	// single compaction, a focus is a wrong call, refused before sending.
	CompactFocus() bool
	// InterruptTrace says what the interrupted session's model is shown of an
	// interrupt, completing "whoever waits on it reads stopped, and …" in the
	// answer to `rewake interrupt`.
	InterruptTrace() string
}

// Flag is one parameter of a harness, by every spelling it answers to.
//
// TakesValue is here because dropping a flag means dropping what belongs to
// it, and a switch owns nothing: treating the next token as a value would
// silently take away the prompt standing after --search.
type Flag struct {
	// Spellings are the forms of one parameter, longest-known first:
	// {"--model", "-m"}.
	Spellings []string
	// TakesValue says whether the next argument belongs to this flag.
	TakesValue bool
}

// BriefContext carries identity to the shared text package without assembling prose.
func (r LaunchRequest) BriefContext() brief.Context {
	return brief.Context{Name: r.Name, Room: r.Room, Role: r.Role, Reason: r.RoleReason}
}

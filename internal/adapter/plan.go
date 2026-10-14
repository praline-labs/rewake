package adapter

import (
	"github.com/praline-labs/rewake/internal/core/brief"
	"github.com/praline-labs/rewake/internal/core/role"
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
	// ControlDir is where this run takes control requests, made by the
	// wrapper before the launch and removed after it; empty when it could not
	// be made (docs/remote-control.md).
	ControlDir string
	// RunDir is this run's own directory, made by the wrapper after the
	// claim and removed — with everything in it — once the run's services,
	// children and sockets have stopped, or by a later launch once the run
	// is proven dead (docs/v2/stage3-rundir.md). What the
	// adapter writes for this run alone goes here; mail, grants, receipts and
	// journals do not. No socket goes here: its path could pass the limit a
	// unix socket allows, so Socket keeps its short form.
	RunDir string
	// GrantDirs are directories main confirmed again for the conversation
	// this launch resumes (Resumer); a harness that takes a directory at
	// launch is started with them.
	GrantDirs []string
	// Version is what the launch read of the harness's version before the
	// claim (VersionGate); its start runs nothing more to learn it. Empty
	// for an adapter whose gate has no minimum.
	Version string
	// Conversation is the id rewake chose for the new conversation this
	// launch starts, a UUID, set only where the adapter's offer names
	// conversations (ConversationOffer). Launch passes it with the offer's
	// flag, or refuses: a launch that dropped it would leave a record naming
	// a conversation that does not exist.
	Conversation string
	// Env is the run's whole environment, built by the host: the current
	// one less what the adapter's runs must not inherit (EnvNotInherited),
	// plus the rewake variables that name the run. The adapter starts the
	// harness with it, adding only its own.
	Env []string
}

// VersionGate is how a launch learns whether the installed harness is one the
// adapter starts: the oldest version it starts, and the arguments that ask the
// installed program for its version (docs/v2/design-api.md#minimum-versions).
// The host reads it before the claim and refuses below the minimum or when no
// version can be read; a gate without a minimum reads nothing.
type VersionGate struct {
	Minimum string
	Args    []string
}

// LaunchPlan is how the wrapper starts the harness, and the capabilities of
// the run it starts: each one nil where the run has none.
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
	// Notes are things the caller should know about this launch: a setting that
	// could not be read, a briefing that was skipped. They are printed once, to
	// stderr, and do not stop the launch.
	Notes []string

	// Service is what of the run's own serves the session beside the
	// harness, started before it and closed after it.
	Service Service
	// Wake carries notices; without it the letter is written and no notice
	// reaches the session.
	Wake Wake
	// TurnBoundary reports the run's turns; without it no turn's end
	// reports.
	TurnBoundary TurnBoundary
	// Telemetry reports the run's state; without it the state is unknown.
	Telemetry Telemetry
	// Thread follows the run's conversation; without it a ThreadTracker
	// harness is asked, or the conversation is unknown.
	Thread ThreadSource
	// Live reports what the run proves of the capabilities above; without
	// it the run's live set is what the plan carries, from its start.
	Live LiveReporter
}

// Program is the executable a launch starts: the one named with --command, or
// the harness's own.
func (r LaunchRequest) Program(own string) string {
	if r.Command != "" {
		return r.Command
	}
	return own
}

// Harness is the Launch capability, which every adapter has: how one
// coding-agent CLI is presented and started. What else it offers is its Offer;
// what one run of it can do is in the run's plan.
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
	// get to make. A harness may refuse to parse a repeated --model, while a
	// repeated --add-dir is how a second directory is added — so the first has to be
	// replaced and the second must not be. A flag missing from this list is
	// simply appended, which is the safe answer for anything unlisted.
	//
	// Returning nothing is therefore not a neutral answer: it says every flag
	// may be repeated, and for a harness that refuses repeats it turns an
	// alias plus a typed flag into a launch that will not parse.
	SingleUseFlags() []Flag
	// ProtectedDirs are where the harness keeps its own metadata: its
	// configuration, credentials and sessions, and what it keeps inside a
	// project. rewake never grants a worker write access to them, whichever
	// harness the worker runs (docs/grants.md).
	ProtectedDirs() Protected
	// Version is the gate a launch reads before the claim.
	Version() VersionGate
	// EnvNotInherited names the variables a run must not inherit from the
	// environment rewake was started in: what a session of the harness
	// exports to its children, which would point a new session at its
	// parent's. The host leaves them out of LaunchRequest.Env.
	EnvNotInherited() []string
	// Offers is what the adapter offers beyond Launch, before any launch.
	Offers() Offer
	// Launch turns a request into the command to run and the capabilities
	// of the run.
	Launch(request LaunchRequest) (LaunchPlan, error)
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

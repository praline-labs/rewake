// Package claude runs Claude Code as a rewake session and delivers messages to
// it through the inbox socket every session of it listens on.
package claude

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
	"github.com/praline-labs/rewake/internal/core/ask"
	"github.com/praline-labs/rewake/internal/core/brief"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/proc"
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

// modelFlag and effortFlag are how Claude Code takes these for one session.
const modelFlag = "--model"

const effortFlag = "--effort"

// toolFlag allows the rewake commands without a confirmation prompt for this
// launch. Without it the agent can be messaged but cannot answer until a person
// approves each reply.
const toolFlag = "--allowedTools"

// toolFlagKebab is the other spelling of toolFlag: 2.1.280's --help lists
// "--allowedTools, --allowed-tools <tools...>" (docs/research-launch.md). A
// person's own rule under it is theirs just the same.
const toolFlagKebab = "--allowed-tools"

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
	// A run's mod socket is its own: one inherited from the run a session
	// was started from names a socket that serves another harness.
	modSocketEnv,
}

// dialTimeout bounds one delivery attempt. The socket is local, so a connection
// that takes longer than this is not slow, it is wrong.
const dialTimeout = 2 * time.Second

type claudeHarness struct{}

// New returns the Claude Code harness.
func New() adapter.Harness { return claudeHarness{} }

func (claudeHarness) ID() string    { return ID }
func (claudeHarness) Title() string { return "Claude Code" }

func (claudeHarness) Summary() string {
	return "Start Claude Code as a rewake session."
}

func (claudeHarness) Examples() []string {
	return []string{
		"rewake --main claude",
		"rewake --write --name api claude",
		"rewake --main --name lead claude --continue",
		"rewake --main claude --worktree=fix-login",
	}
}

// claudeDefaults are the launch settings rewake may take from the environment.
//
// Every way a person can state either setting for one launch, checked against
// the installed 2.1.270 rather than from memory. Both are ordinary flags —
// "--model <model>" and "--effort <level>" — written apart or with '=', and
// neither has a short form: "claude -m x" answers "unknown option '-m'".
// Checking that with --help would have proved nothing: Claude Code accepts an
// unknown flag beside --help and prints the help anyway.
//
// Deliberately not consulted: the settings files, and anything passed through
// --settings. rewake does not read or edit a person's configuration, and this
// adapter cannot see into a settings file it was handed. Somebody who sets a
// model there and also sets the environment variable gets the variable, which
// is the one they set for rewake specifically.
//
// Neither setting has a configuration-key form to check.
func claudeDefaults() []adapter.Default {
	return []adapter.Default{
		{
			Env:     "REWAKE_CLAUDE_MODEL",
			What:    "model",
			Present: func(args []string) bool { return adapter.HasFlag(args, modelFlag) },
			Apply: func(args []string, value string) []string {
				return adapter.AddFlags(args, modelFlag, value)
			},
		},
		{
			Env:     "REWAKE_CLAUDE_EFFORT",
			What:    "reasoning effort",
			Present: func(args []string) bool { return adapter.HasFlag(args, effortFlag) },
			Apply: func(args []string, value string) []string {
				return adapter.AddFlags(args, effortFlag, value)
			},
		},
	}
}

func (claudeHarness) Notes() []string {
	return []string{
		"Messages arrive through the session's inbox socket within seconds and wake an idle session.",
		"Each turn's end reaches rewake through the rewake mod, which rewake writes into the run's directory and loads for that launch alone; a task or question to the session fails until the mod's hello is admitted, and a role that owes reports refuses to start when rewake knows the mod will not load.",
		"A turn's end is not held open: a turn that ends after rewake pending without a new mark closes the task as answered.",
		"A launch through --command reaches the mod only when the command ends by exec-ing claude: a wrapper that runs it as a child is never admitted, and the session takes no tasks.",
		"Arguments after claude reach it as written, beside the flags rewake adds for delivery, except: a --help first asks rewake for this page; --worktree is rewake's, below; and a typed flag replaces an alias's copy of it.",
		"--worktree starts the session in a new checkout of HEAD on a new branch under rewake's worktree directory, at the same place within the repository; --worktree=<name> names both, and a name after a space is refused, since it would reach claude as the prompt. rewake worktree --help says how to land and remove it. It starts a new conversation only: --continue or -c, --resume or -r, --from-pr, --teleport, --fork-session, the attach and respawn commands, Claude Code's own -w and --tmux, --cloud and --environment are refused beside it. -w alone stays Claude Code's own worktree, inside the repository.",
	}
}

// SingleUseFlags are the flags Claude Code resolves to a single value. Read
// from `claude --help` on 2.1.270; it has no short spellings for these.
//
// Claude Code accepts a repeated flag and takes the last occurrence, so
// replacing the alias's copy is a convenience here — it keeps the command
// readable and harnesses behaving alike — where a harness that refuses a
// repeat needs it to parse the launch at all.
//
// Absent on purpose: --add-dir, --plugin-dir, --plugin-url, --mcp-config and
// the other lists it spells with `<values...>` — each of those is repeated to
// add another entry, and replacing an alias's entry with a typed one would
// remove a directory or a server nobody asked to remove.
func (claudeHarness) SingleUseFlags() []adapter.Flag {
	return []adapter.Flag{
		{Spellings: []string{"--model"}, TakesValue: true},
		{Spellings: []string{"--effort"}, TakesValue: true},
		{Spellings: []string{"--fallback-model"}, TakesValue: true},
		{Spellings: []string{"--permission-mode"}, TakesValue: true},
		{Spellings: []string{"--settings"}, TakesValue: true},
		{Spellings: []string{"--agent"}, TakesValue: true},
		{Spellings: []string{sessionIDFlag}, TakesValue: true},
		// rewake's own, taken before the launch and read only with =<name>, so
		// a switch here: a typed one replaces an alias's.
		{Spellings: []string{"--worktree"}},
	}
}

// Offers: Wake, and a turn boundary that the run's mod proves with its hello
// from inside the harness, so only after the harness starts; it reports every
// end and holds none open (docs/v2/stage4-turns.md).
//
// Spaces: a main run hosts them once its mod's hello is admitted; a run whose
// mod did not load has none. Conversation: --session-id <uuid> starts the new
// conversation under rewake's id (`claude --help`, 2.1.296).
func (claudeHarness) Offers() adapter.Offer {
	return adapter.Offer{
		Wake:         true,
		Telemetry:    true,
		Control:      adapter.ControlOffer{Compact: true, CompactFocus: true, Interrupt: true},
		TurnBoundary: adapter.TurnBoundaryOffer{Reports: true, AfterStart: true},
		Spaces:       true, Conversation: adapter.ConversationOffer{Flag: sessionIDFlag},
	}
}

// sessionIDFlag names a new conversation's id; Claude Code refuses one that
// is not a UUID or is already in use (`claude --help`, 2.1.296).
const sessionIDFlag = "--session-id"

// conversationRefusal refuses rewake's id for a launch whose arguments
// already name or continue a conversation: the record would name one the
// harness never started.
func conversationRefusal(args []string) error {
	if adapter.HasFlag(args, sessionIDFlag) {
		return fmt.Errorf("rewake names this launch's conversation and the arguments name one with %s; remove it", sessionIDFlag)
	}
	if word := claudeContinuation(adapter.BeforeTerminator(args)); word != "" {
		return fmt.Errorf("rewake names this launch's new conversation and %s continues one; remove it", word)
	}
	return nil
}

func (claudeHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	args := append([]string{}, request.Args...)
	if adapter.HasFlag(args, worktreeFlag) {
		// The launch command takes the flag before a launch is planned; one
		// reaching here came some other way, and Claude Code would make a
		// worktree of its own instead of rewake's.
		return adapter.LaunchPlan{}, fmt.Errorf("%s reached the Claude Code launch; start it with rewake claude %s so rewake makes the checkout, or give -w for Claude Code's own", worktreeFlag, worktreeFlag)
	}
	socket := request.Socket
	owns := false

	if adapter.HasFlag(args, socketFlag) {
		// The caller named their own socket. Their flag wins, and delivery has
		// to find it from the record, so read it back rather than guess. It is
		// theirs, not ours: this session never removes that file.
		socket = flagValue(args, socketFlag)
	} else {
		if len(socket) > maxSocketPath {
			return adapter.LaunchPlan{}, fmt.Errorf("the socket path %s is longer than the %d bytes a unix socket allows; use a shorter session name or REWAKE_DIR", socket, maxSocketPath)
		}
		// A leftover file from a session that died keeps Claude Code from
		// binding the same path.
		if err := removeStaleSocket(socket); err != nil {
			return adapter.LaunchPlan{}, err
		}
		owns = true
		args = adapter.AddFlags(args, socketFlag, socket)
	}

	args, notes := adapter.ApplyDefaults(args, claudeDefaults())
	reply := replyPath(request, socket, owns)
	if len(reply) > maxSocketPath {
		notes = append(notes, "not hearing back about held messages: the socket path "+reply+" is longer than a unix socket allows")
		reply = ""
	}
	if request.Intro && !adapter.HasFlag(args, introFlag) {
		args = adapter.AddFlags(args, introFlag, brief.Intro(request.BriefContext()))
	}
	if len(adapter.FlagValues(args, toolFlag, toolFlagKebab)) == 0 {
		args = adapter.AddFlags(args, toolFlag, "Bash(rewake:*)")
	}

	if request.Conversation != "" {
		if err := conversationRefusal(args); err != nil {
			return adapter.LaunchPlan{}, err
		}
		args = adapter.AddFlags(args, sessionIDFlag, request.Conversation)
	}

	wake := newLane(socket, reply, owns)
	wake.room = request.RoomDir
	var service adapter.Service
	var live adapter.LiveReporter
	var turns adapter.TurnBoundary
	var telemetry adapter.Telemetry
	var spaces adapter.SpacePanel
	env := request.Env
	modSocket := modSocketPath(request)
	switch {
	case len(modSocket) > maxSocketPath:
		service = modless{lane: wake, cause: "its socket path " + modSocket + " is longer than a unix socket allows; start it with a shorter session name or REWAKE_DIR"}
	case request.RunDir == "" || request.Socket == "":
		service = modless{lane: wake, cause: "the launch has no run directory or inbox socket of rewake's to load it from"}
	default:
		// The mod is this run's alone: written into its directory, loaded
		// by this launch's flag, its socket named in this launch's
		// environment.
		args = adapter.AddFlags(args, pluginDirFlag, modDir(request.RunDir))
		env = withEnv(env, modSocketEnv, modSocket)
		run := &modRun{lane: wake, dir: modDir(request.RunDir), socket: modSocket, hostsSpaces: request.Role.Spaces, controls: request.ControlDir != ""}
		run.server = modhost.New(modhost.Config{
			Socket: modSocket, Role: request.Role.ID, Record: filepath.Join(request.RunDir, "hello.json"), Wait: harnessWait,
			ControlDir: request.ControlDir, Epoch: request.Epoch,
			HostsSpaces: request.Role.Spaces, Live: run.transition, Ended: runEnded,
			Asked: askedOf(request),
		})
		service, live, turns, telemetry = run, run, run, run.server
		if request.Role.Spaces {
			spaces = run.server
		}
	}
	return adapter.LaunchPlan{
		Command:      request.Program("claude"),
		Args:         args,
		Env:          env,
		Socket:       socket,
		OwnsSocket:   owns,
		Notes:        notes,
		Service:      service,
		Wake:         wake,
		Live:         live,
		TurnBoundary: turns,
		Telemetry:    telemetry,
		Spaces:       spaces,
	}, nil
}

// askedOf tells main of a worker's request: a session of the roles that take
// work, general and write, asks main rather than wait at a screen nobody may
// watch; main's own and any other role's requests stay at its screen.
func askedOf(request adapter.LaunchRequest) func(string, ask.Offer, time.Time) error {
	if request.Role.ID != role.General.ID && request.Role.ID != role.Write.ID {
		return nil
	}
	return func(id string, offer ask.Offer, deadline time.Time) error {
		return ask.TellMain(request.RoomDir, request.Name, request.Epoch, id, offer, deadline)
	}
}

// runEnded says a run is proven ended, for the mod's question about the
// runs whose ends the harness's store keeps: of another boot, or its wrapper
// gone with its start time. One alive or that cannot be told is not.
func runEnded(epoch string) bool { return registry.ObserveRun(epoch) == proc.IdentityEnded }

// flagValue reads the value of a flag the caller passed. Only before "--": what
// follows is input for the harness, where a flag-looking word is text.
func flagValue(args []string, flag string) string {
	for index, arg := range adapter.BeforeTerminator(args) {
		if arg == flag && index+1 < len(args) {
			return args[index+1]
		}
		if len(arg) > len(flag)+1 && arg[:len(flag)+1] == flag+"=" {
			return arg[len(flag)+1:]
		}
	}
	return ""
}

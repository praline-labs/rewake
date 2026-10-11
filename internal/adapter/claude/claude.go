// Package claude runs Claude Code as a rewake session and delivers messages to
// it through the inbox socket every session of it listens on.
package claude

import (
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/brief"
	"github.com/praline-labs/rewake/internal/core/role"
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
		"rewake --main --name api claude --continue",
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
		"This rewake is not told when a Claude Code turn ends, so Claude Code runs a main session only: a role that owes reports refuses to start on it, and a task or question to it fails, until stage 4 gives it a turn boundary.",
		"Arguments after claude reach it as written, beside the flags rewake adds for delivery, except: a --help first asks rewake for this page; --worktree is rewake's, below; and a typed flag replaces an alias's copy of it.",
		"Claude Code takes no grant in this rewake: not a directory, since the permission hook it took them through is gone until stage 6, and not Git metadata, so a write session commits within its own permissions. A directory a session needs, the owner adds: --add-dir at the launch or /add-dir in its terminal.",
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
		// rewake's own, taken before the launch and read only with =<name>, so
		// a switch here: a typed one replaces an alias's.
		{Spellings: []string{"--worktree"}},
	}
}

// Offers: Launch and Wake only in stage 3, with no turn boundary until the
// mod brings one in stage 4 (docs/v2/stage3-steps-adapters.md, S9). Claude
// Code runs its commands without a sandbox of its own, so `rewake send`
// reaches the wrapper's socket to register a grant.
func (claudeHarness) Offers() adapter.Offer {
	return adapter.Offer{Wake: true, Permissions: adapter.PermissionsOffer{ReachesWrapper: true}}
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
		args = adapter.AddFlags(args, introFlag, brief.Intro(briefContext(request)))
	}
	if len(adapter.FlagValues(args, toolFlag, toolFlagKebab)) == 0 {
		args = adapter.AddFlags(args, toolFlag, "Bash(rewake:*)")
	}
	args = grantDirs(args, request.GrantDirs)

	wake := newLane(socket, reply, owns)
	return adapter.LaunchPlan{
		Command:    request.Program("claude"),
		Args:       args,
		Env:        request.Env,
		Socket:     socket,
		OwnsSocket: owns,
		Notes:      notes,
		Service:    wake,
		Wake:       wake,
	}, nil
}

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

// briefContext is the request's briefing with what the role texts leave to
// the harness: a session that may commit does so within its own permissions,
// since Claude Code takes no --grant-git.
func briefContext(request adapter.LaunchRequest) brief.Context {
	context := request.BriefContext()
	if request.Role.GitWrite {
		context.Harness = role.Section{Title: "ON CLAUDE CODE", Lines: []string{
			"Claude Code takes no --grant-git: you commit within your own permissions, as the owner set them.",
		}}
	}
	return context
}

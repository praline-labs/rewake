// Package claude runs Claude Code as a rewake session and delivers messages to
// it through the inbox socket every session of it listens on.
package claude

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/brief"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/claude/telemetry"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// ID is the launch command and the harness field of a session record.
const ID = "claude"

// The collector is where a delivery learns its conversation; a change that
// made it stop answering would leave every report without threadChanged and
// nothing failing.
var _ harness.ThreadSource = (*telemetry.Collector)(nil)

// Its plugin serves control requests; a change that dropped the method would
// refuse every compact and interrupt as unsupported.
var _ harness.Steerable = claudeHarness{}

// socketFlag asks Claude Code to put its inbox socket where we can name it. The
// flag is undocumented but stable since 2.1.224; without it the socket lands
// under a path derived from the pid, which we would then have to discover.
const socketFlag = "--messaging-socket-path"

// introFlag adds to the system prompt for one launch. It appends, so the user's
// own configuration is untouched.
const introFlag = "--append-system-prompt"

// settingsFlag layers a settings object over the user's for one launch. Claude
// Code merges the layers, so the hooks it carries run next to the user's own
// rather than instead of them; it reads only one, which settings.go handles.
const settingsFlag = "--settings"

// modelFlag and effortFlag are how Claude Code takes these for one session.
const modelFlag = "--model"

const effortFlag = "--effort"

// toolFlag allows the rewake commands without a confirmation prompt for this
// launch. Without it the agent can be messaged but cannot answer until a person
// approves each reply.
const toolFlag = "--allowedTools"

// autocompactFlag sets the auto-compact window, which the harness then treats
// as the context window; the collector needs it to show that window.
const autocompactFlag = "--autocompact"

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

// CompactFocus: the plugin passes a focus as the compaction's instructions,
// which reach the summary request (docs/research-claude-actions.md).
func (claudeHarness) CompactFocus() bool { return true }

// InterruptTrace: a plugin abort leaves the model no trace, so rewake adds a
// line to the session's next notice (docs/remote-control.md).
func (claudeHarness) InterruptTrace() string { return "its next notice says you interrupted it" }

func (claudeHarness) Summary() string {
	return "Start Claude Code as a rewake session. Messages reach it in seconds."
}

func (claudeHarness) Examples() []string {
	return []string{
		"rewake claude",
		"rewake --name api claude --continue",
		"rewake claude --worktree=fix-login",
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
// Unlike Codex, neither setting has a configuration-key form to check.
func claudeDefaults() []harness.Default {
	return []harness.Default{
		{
			Env:     "REWAKE_CLAUDE_MODEL",
			What:    "model",
			Present: func(args []string) bool { return harness.HasFlag(args, modelFlag) },
			Apply: func(args []string, value string) []string {
				return harness.AddFlags(args, modelFlag, value)
			},
		},
		{
			Env:     "REWAKE_CLAUDE_EFFORT",
			What:    "reasoning effort",
			Present: func(args []string) bool { return harness.HasFlag(args, effortFlag) },
			Apply: func(args []string, value string) []string {
				return harness.AddFlags(args, effortFlag, value)
			},
		},
	}
}

func (claudeHarness) Notes() []string {
	return []string{
		"Delivery goes through the session inbox socket, so a message arrives within seconds and wakes an idle session.",
		"Arguments after the harness name are passed to claude untouched, with one exception: a --help written first asks rewake for this page instead of starting the harness.",
		"An identical message from the same sender within thirty seconds is dropped by Claude Code itself; rewake puts a short id in every message to keep them apart.",
		"--worktree is rewake's here: rewake adds a checkout of HEAD on a new branch under its worktree directory and starts the session in it, at the same place within the repository; --worktree=<name> names both. rewake worktree land takes the branch's commits into the checkout it came from, finish lands and removes it, ls lists and rm removes. It starts a new conversation only: --continue, --resume, --from-pr, --teleport and --fork-session are refused beside it. -w stays Claude Code's own worktree, inside the repository.",
	}
}

// SingleUseFlags are the flags Claude Code resolves to a single value. Read
// from `claude --help` on 2.1.270; it has no short spellings for these.
//
// The reason differs from Codex's, and the difference matters: Claude Code
// accepts a repeated flag and takes the last occurrence, so replacing the
// alias's copy is a convenience here — it keeps the command readable and the
// two harnesses behaving alike — where on Codex it is what keeps the launch
// from failing to parse at all.
//
// Absent on purpose: --add-dir, --plugin-dir, --plugin-url, --mcp-config and
// the other lists it spells with `<values...>` — each of those is repeated to
// add another entry, and replacing an alias's entry with a typed one would
// remove a directory or a server nobody asked to remove.
func (claudeHarness) SingleUseFlags() []harness.Flag {
	return []harness.Flag{
		{Spellings: []string{"--model"}, TakesValue: true},
		{Spellings: []string{"--effort"}, TakesValue: true},
		{Spellings: []string{"--fallback-model"}, TakesValue: true},
		{Spellings: []string{"--permission-mode"}, TakesValue: true},
		{Spellings: []string{"--settings"}, TakesValue: true},
		{Spellings: []string{"--agent"}, TakesValue: true},
	}
}

// ReachesWrapper: Claude Code runs its commands without a sandbox of its own,
// so `rewake send` reaches the wrapper's socket to register a grant.
func (claudeHarness) ReachesWrapper() bool { return true }

// SupportsDirGrant: a grant reaches a running session through the permission
// hooks rewake installs (permission.go).
func (claudeHarness) SupportsDirGrant() bool { return true }

func (claudeHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	args := append([]string{}, request.Args...)
	if harness.HasFlag(args, worktreeFlag) {
		// The launch command takes the flag before a launch is planned; one
		// reaching here came some other way, and Claude Code would make a
		// worktree of its own instead of rewake's.
		return harness.LaunchPlan{}, fmt.Errorf("%s reached the Claude Code launch; start it with rewake claude %s so rewake makes the checkout, or give -w for Claude Code's own", worktreeFlag, worktreeFlag)
	}
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
		args = harness.AddFlags(args, introFlag, brief.Intro(request.BriefContext()))
	}
	args, notes := harness.ApplyDefaults(args, claudeDefaults())
	env := harness.SessionEnv(request, childMarkers)
	var observer harness.Observer
	var drawn <-chan struct{}
	var marks interruptMarks
	observation := request.ObservationSocket
	if len(observation) > maxSocketPath {
		notes = append(notes, "not collecting telemetry: the socket path "+observation+" is longer than a unix socket allows")
		observation = ""
	}
	if observation != "" {
		collector := telemetry.NewCollector(observation)
		collector.Controls(request.ControlDir)
		if values := harness.FlagValues(args, autocompactFlag); len(values) > 0 {
			// The harness takes the last one, as it does for any option.
			collector.LaunchedWith(values[len(values)-1])
		}
		observer, drawn, marks = collector, collector.Drawn(), collector
	}
	reply := replyPath(request, socket, owns)
	if len(reply) > maxSocketPath {
		notes = append(notes, "not hearing back about held messages: the socket path "+reply+" is longer than a unix socket allows")
		reply = ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return harness.LaunchPlan{}, err
	}
	args, settingsNotes := applySettings(args, cwd, request.Role.Silent, observation)
	notes = append(notes, settingsNotes...)
	args, env, pluginNotes := applyPlugin(args, env, observation, request.ControlDir)
	notes = append(notes, pluginNotes...)
	if !harness.HasFlag(args, toolFlag) {
		args = harness.AddFlags(args, toolFlag, "Bash(rewake:*)")
	}

	return harness.LaunchPlan{
		Command:    request.Program("claude"),
		Args:       args,
		Env:        env,
		Socket:     socket,
		OwnsSocket: owns,
		Notes:      notes,
		Observer:   observer,
		Lane:       newLane(reply, owns, drawn, marks),
	}, nil
}

// replyPath is where this run hears back about its notices. It has to be a
// .sock in the same directory as the session's own socket, the only place
// Claude Code sends a receipt to (docs/research-launch.md). Beside a socket of
// rewake's own it is that socket's name with .reply; beside one the caller
// named, or where that name would be too long, a name of this run's own.
func replyPath(request harness.LaunchRequest, socket string, owns bool) string {
	if socket == "" {
		return ""
	}
	if own := strings.TrimSuffix(socket, ".sock") + replySuffix; owns && len(own) <= maxSocketPath {
		return own
	}
	sum := sha256.Sum256([]byte(request.Name + "\x00" + request.Epoch))
	return filepath.Join(filepath.Dir(socket), fmt.Sprintf("rewake-%x.reply.sock", sum[:8]))
}

// maxSocketPath is the limit the kernel puts on a unix socket path.
const maxSocketPath = 103

// Deliver writes the notice with no reply address, so the session reports
// nothing back and a successful write is all there is to know. The lane
// (lane.go) is the path a running wrapper uses; this one is what is left when
// it cannot listen.
func (claudeHarness) Deliver(ctx context.Context, session registry.Session, message inbox.Message) inbox.Result {
	return writeNotice(ctx, session, newEnvelope(message, ""))
}

func newEnvelope(message inbox.Message, interrupter string) envelope {
	return envelope{
		Type:     "user",
		Message:  payload{Role: "user", Content: notification(message, interrupter)},
		Priority: "next",
	}
}

// writeNotice writes one line to the session's inbox socket.
func writeNotice(ctx context.Context, session registry.Session, notice envelope) inbox.Result {
	if session.Socket == "" {
		return inbox.Result{State: inbox.Failed, Detail: "this session has no inbox socket"}
	}

	line, err := json.Marshal(notice)
	if err != nil {
		return inbox.Result{State: inbox.Failed, Detail: "the message could not be encoded: " + err.Error()}
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
	defer func() { _ = connection.Close() }()

	_ = connection.SetWriteDeadline(time.Now().Add(dialTimeout))
	if _, err := connection.Write(append(line, '\n')); err != nil {
		return inbox.Result{State: inbox.Failed, Detail: "could not write to the session: " + err.Error()}
	}
	return inbox.Result{State: inbox.Delivered, Via: "socket"}
}

// envelope is one line of the session inbox protocol. Priority "next" puts the
// message after the tool call in flight and starts a turn when the session is
// idle, which is what a message from a peer should do.
//
// From and MsgID ask for receipts: the session reports what its inbound gate
// did with the line to the socket From names, quoting MsgID (lane.go).
type envelope struct {
	Type     string  `json:"type"`
	Message  payload `json:"message"`
	Priority string  `json:"priority"`
	From     string  `json:"from,omitempty"`
	MsgID    string  `json:"msg_id,omitempty"`
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
		_ = connection.Close()
		return fmt.Errorf("%s is a live socket; another session is using this name", path)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("%s exists and could not be checked (%v); leaving it alone", path, err)
	}
	return os.Remove(path)
}

// flagValue reads the value of a flag the caller passed. Only before "--": what
// follows is input for the harness, where a flag-looking word is text.
func flagValue(args []string, flag string) string {
	for index, arg := range harness.BeforeTerminator(args) {
		if arg == flag && index+1 < len(args) {
			return args[index+1]
		}
		if len(arg) > len(flag)+1 && arg[:len(flag)+1] == flag+"=" {
			return arg[len(flag)+1:]
		}
	}
	return ""
}

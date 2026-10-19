// Package pi starts pi with a per-run extension carrying notices and turn ends.
package pi

import "github.com/praline-labs/rewake/internal/adapter"

// ID names the harness; MinimumVersion is the first supported extension API,
// the floor for a pi named with --command.
const (
	ID             = "pi"
	MinimumVersion = "1.2.0"
	socketEnv      = "REWAKE_PI_SOCKET"
	// agentDirEnv moves pi's whole agent directory; skipVersionCheckEnv
	// keeps pi from offering an update of a copy rewake updates.
	agentDirEnv         = "PI_CODING_AGENT_DIR"
	skipVersionCheckEnv = "PI_SKIP_VERSION_CHECK"
	sessionIDFlag       = "--session-id"
	worktreeFlag        = "--worktree"
)

type harness struct{}

// New returns the pi adapter.
func New() adapter.Harness { return harness{} }

func (harness) ID() string      { return ID }
func (harness) Title() string   { return "pi" }
func (harness) Summary() string { return "Start pi as a rewake session." }

func (harness) Examples() []string {
	return []string{"rewake --main pi", "rewake --name api pi", "rewake pi --worktree=fix-login"}
}

func (harness) Notes() []string {
	return []string{
		"Notices wake an idle session and steer a busy one after its current tool batch. The per-run extension carries notices and reports through a local socket.",
		"rewake starts the pi installed with it, " + PinnedVersion + ", with node (Node.js " + NodeMinimum + " or later); a pi of another version there refuses the launch. A pi named with --command starts at " + MinimumVersion + " or later, with a note when it is not " + PinnedVersion + ". REWAKE_PI names a pi package or entry script for a build npm did not install.",
		"pi keeps its login, settings, extensions, skills, prompts, themes, packages and sessions in ~/.config/rewake/pi/agent, apart from the person's own pi; log in once there with /login. Its extensions, the project's included, load beside rewake's; project resources ask for trust as pi does.",
		"Tasks and questions are accepted after the extension connects from the harness process. A --command wrapper must exec pi, not start it as a child.",
		"A turn ends at agent_settled, after automatic retries and queued work. Its end is not held open; use rewake pending before an interim end.",
		"Context, model and thinking level come from the native session. Compact accepts an optional focus and refuses a running turn; interrupt stops a running turn. Limits are not reported.",
		"REWAKE_PI_MODEL and REWAKE_PI_EFFORT supply --model and --thinking when the launch does not name them.",
		"--worktree[=<name>] starts a new conversation in a checkout rewake makes. Continue or resume a conversation from its existing checkout, without --worktree.",
	}
}

// These flags resolve to one value in the 1.3.0 CLI. Resource lists
// and --append-system-prompt are repeatable and deliberately absent.
func (harness) SingleUseFlags() []adapter.Flag {
	return []adapter.Flag{
		{Spellings: []string{"--model"}, TakesValue: true},
		{Spellings: []string{"--provider"}, TakesValue: true},
		{Spellings: []string{"--thinking"}, TakesValue: true},
		{Spellings: []string{"--models"}, TakesValue: true},
		{Spellings: []string{"--api-key"}, TakesValue: true},
		{Spellings: []string{"--session"}, TakesValue: true},
		{Spellings: []string{sessionIDFlag}, TakesValue: true},
		{Spellings: []string{"--session-dir"}, TakesValue: true},
		{Spellings: []string{"--fork"}, TakesValue: true},
		{Spellings: []string{"--name", "-n"}, TakesValue: true},
		{Spellings: []string{"--tools", "-t"}, TakesValue: true},
		{Spellings: []string{"--exclude-tools", "-xt"}, TakesValue: true},
		{Spellings: []string{"--system-prompt"}, TakesValue: true},
		{Spellings: []string{"--mode"}, TakesValue: true},
		{Spellings: []string{"--tui-mode"}, TakesValue: true},
		{Spellings: []string{"--use-theme"}, TakesValue: true},
		{Spellings: []string{"--continue", "-c"}},
		{Spellings: []string{"--resume", "-r"}},
		{Spellings: []string{"--print", "-p"}},
		{Spellings: []string{"--no-session"}},
		{Spellings: []string{"--no-extensions", "-ne"}},
		{Spellings: []string{"--no-approve", "-na"}},
		{Spellings: []string{worktreeFlag}},
	}
}

func (harness) Version() adapter.VersionGate {
	return adapter.VersionGate{Minimum: MinimumVersion, Args: []string{"--version"}, Pinned: PinnedVersion}
}

func (harness) EnvNotInherited() []string {
	return []string{socketEnv, agentDirEnv, skipVersionCheckEnv, "PI_CODING_AGENT", "PI_SESSION_ID", "PI_SESSION_FILE", "PI_PROVIDER", "PI_MODEL", "PI_REASONING_LEVEL"}
}

func (harness) Offers() adapter.Offer {
	return adapter.Offer{
		Wake: true, Telemetry: true,
		Control:      adapter.ControlOffer{Compact: true, Interrupt: true, CompactFocus: true, InterruptTrace: "its next turn sees who interrupted it"},
		TurnBoundary: adapter.TurnBoundaryOffer{Reports: true, AfterStart: true},
		Conversation: adapter.ConversationOffer{Flag: sessionIDFlag},
	}
}

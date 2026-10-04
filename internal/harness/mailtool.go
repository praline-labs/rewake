package harness

import (
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/state"
)

// The launch side of the mail tool (docs/mail-bridge-launch.md): a harness
// that can carry the tool says, before the run is claimed, whether this
// launch gets it — after proving the name rewake free in every source it
// covers — and, once the run is claimed, adds the tool to its plan.

// ToolCheckRequest is what a harness knows of a launch before the claim.
type ToolCheckRequest struct {
	// Args are the settled arguments: aliases and defaults applied.
	Args []string
	// Command is the caller's --command, if any: a check runs the program
	// the launch will (Executable).
	Command string
	// Cwd is the launch directory.
	Cwd string
	// Env is the environment the harness will have, less the run's values.
	Env []string
	// StateRoot is where a check's private directory goes.
	StateRoot string
	// Gates are this launch's gates.
	Gates Gates
}

// Executable is the program a check runs: the caller's --command, else the
// harness's own, as LaunchRequest.Program resolves it.
func (r ToolCheckRequest) Executable(own string) string {
	if r.Command != "" {
		return r.Command
	}
	return own
}

// ToolDecision is a harness's answer before the claim.
type ToolDecision struct {
	// Inject says the launch adds the tool.
	Inject bool
	// Reason, when it does not, says why in the words a diagnostic allows.
	Reason string
}

// MailToolHarness is a harness that can carry the mail tool. Its check
// returns a NameTakenError or a CheckFailedError to refuse the launch, and
// no other error: anything else it cannot do leaves the tool out.
type MailToolHarness interface {
	CheckMailTool(request ToolCheckRequest) (ToolDecision, error)
}

// ToolServer is the mail tool's server as the plan adds it.
type ToolServer struct {
	// Executable is this rewake, resolved once.
	Executable string
	// Env are the launch values the server starts with, in this order.
	Env []EnvValue
	// ContextSocket is the run's context endpoint, for the hooks.
	ContextSocket string
	// ConfigFile is where a harness that takes the server as a file writes
	// it, beside the run's sockets and removed with them.
	ConfigFile string
	// Gates are this launch's gates.
	Gates Gates
}

// EnvValue is one variable of the server's environment.
type EnvValue struct{ Name, Value string }

// ToolArgs are the server's arguments.
var ToolArgs = []string{"bridge-serve"}

// NewToolServer builds the server for a run.
func NewToolServer(executable, root, room, name, epoch, capability, contextSocket, configFile string, gates Gates) *ToolServer {
	return &ToolServer{
		Executable: executable,
		Env: []EnvValue{
			{state.DirEnv, root},
			{state.RoomEnv, room},
			{state.SessionEnv, name},
			{state.EpochEnv, epoch},
			{bridge.CapabilityEnv, capability},
		},
		ContextSocket: contextSocket, ConfigFile: configFile, Gates: gates,
	}
}

// ToolName is the tool as an allowance names it on Claude Code.
const ToolName = "mcp__rewake__rewake"

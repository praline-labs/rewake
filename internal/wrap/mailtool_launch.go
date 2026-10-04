package wrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/state"
)

// The launch side of the mail tool (docs/mail-bridge-launch.md#the-launch-in-order):
// before the claim, whether this launch gets the tool and that the name is
// free; after it, the server the plan adds.

// toolChoice is what a launch decided before the claim.
type toolChoice struct {
	gates harness.Gates
	// version is what the launch read of its harness's version, for a
	// harness that reads it on every launch.
	version harness.Version
	// inject says the plan adds the tool; note, when it does not, says why.
	inject bool
	note   string
}

// chooseTool decides the tool before the run is claimed, so a refusal
// publishes nothing. The error is the harness's refusal, as it words it.
// The version is taken only where a closed gate could change the choice
// (docs/mail-bridge-version.md): under --no-mail-tool, or for a harness
// without the tool, the gates are not consulted, apart from the read a
// harness takes on every launch.
func chooseTool(request Request, cwd string) (toolChoice, error) {
	program := request.Command
	if program == "" {
		program = request.Harness.ID()
	}
	choice := toolChoice{gates: harness.ResolveGates(request.Harness.ID(), "", request.AssumedGates)}
	var read *harness.Version
	if reader, ok := request.Harness.(harness.LaunchVersionReader); ok {
		version, err := reader.ReadLaunchVersion(program, harness.CheckEnv(), cwd)
		if err != nil {
			return choice, err
		}
		choice.version, read = version, &version
	}
	checker, carries := request.Harness.(harness.MailToolHarness)
	switch {
	case !carries || request.MailTool == nil || transports[request.Harness.ID()] == "":
		return choice, nil
	case request.NoMailTool:
		choice.note = "--no-mail-tool"
		return choice, nil
	}
	decision, err := checker.CheckMailTool(harness.ToolCheckRequest{
		Args: request.Args, Command: request.Command, Cwd: cwd,
		Env: harness.CheckEnv(), StateRoot: state.RootForRoom(request.Dir),
		Version: read, Assumed: request.AssumedGates,
	})
	if err != nil {
		return choice, err
	}
	choice.inject, choice.note = decision.Inject, decision.Reason
	if decision.Inject {
		// Without the tool the gates decide nothing more.
		choice.gates = decision.Gates
	}
	return choice, nil
}

// server is the tool's server for the plan, or nil: no tool chosen, or no
// endpoint to serve it, which costs the tool and never the session.
func (c toolChoice) server(request Request, session sessionNames, tool *mailTool) (*harness.ToolServer, string) {
	if !c.inject {
		return nil, c.note
	}
	if tool.endpoint == nil {
		return nil, "the run's context endpoint could not start"
	}
	executable, err := os.Executable()
	if err == nil {
		executable, err = filepath.EvalSymlinks(executable)
	}
	if err != nil {
		return nil, "rewake's own executable could not be resolved"
	}
	return harness.NewToolServer(executable, state.RootForRoom(request.Dir), session.room, session.name, session.epoch,
		tool.capability, state.ContextPath(request.Dir, session.name, session.epoch),
		state.ToolConfigPath(request.Dir, session.name, session.epoch), c.gates), ""
}

// sessionNames are the claimed run's names the server is started with.
type sessionNames struct{ room, name, epoch string }

// toolNote is the launch note of a run without the tool, "" when it has it
// or its harness carries none.
func toolNote(reason string) string {
	if reason == "" {
		return ""
	}
	return "the mail tool is not added: " + reason + "; letters are read in the shell with rewake inbox"
}

// announceGates says on stderr, before anything else of the launch, which
// gates it takes as closed: a run resting on an assumption is never silent.
func announceGates(assumed []string) {
	if len(assumed) == 0 {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "rewake: taking gates %s as closed for this launch (%s, for verification only)\n",
		strings.Join(assumed, ", "), harness.GatesAssumedEnv)
}

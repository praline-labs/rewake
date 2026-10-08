package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
)

// The name check of the mail tool on Codex, before the run is claimed
// (docs/mail-bridge-launch-codex.md#the-name-check-a-separate-app-server-before-the-claim):
// a separate app-server with the caller's -c values and none of ours, asked
// for every layer, then ended.

var (
	_ harness.MailToolHarness     = codexHarness{}
	_ harness.LaunchVersionReader = codexHarness{}
)

// preflightBound bounds the whole check, from start to the group's end; a
// variable so a test of a hanging server need not wait the whole of it.
var preflightBound = 10 * time.Second

// preflightTemplate is what the person may run by hand when it fails.
const preflightTemplate = "codex mcp get rewake with the same -c values"

// remoteControlOff keeps an app-server rewake starts from offering remote
// control, as the owned server does.
const remoteControlOff = "CODEX_INTERNAL_APP_SERVER_REMOTE_CONTROL_DISABLED=1"

// CheckMailTool decides the tool for a launch and proves the name free.
func (codexHarness) CheckMailTool(request harness.ToolCheckRequest) (harness.ToolDecision, error) {
	version := harness.Version{Unknown: harness.VersionNotRead}
	if request.Version != nil {
		version = *request.Version
	}
	gates := harness.ResolveGates("codex", version.Value, request.Assumed)
	if reason := gatesLeaveOut(gates, version); reason != "" {
		return harness.ToolDecision{Reason: reason, Gates: gates}, nil
	}
	cwd, err := gitWorkingDirectory(request.Args)
	if err != nil {
		// The launch refuses the same arguments itself, with this reason.
		return harness.ToolDecision{Reason: "the working directory could not be resolved", Gates: gates}, nil
	}
	config, requirements, err := preflight(request, cwd)
	if err != nil {
		return harness.ToolDecision{}, err
	}
	if where, taken := takenWhere(config, request.Args); taken {
		return harness.ToolDecision{}, &harness.NameTakenError{Where: where}
	}
	if requiresMCP(requirements) {
		return harness.ToolDecision{Reason: "gate G4: managed requirements name MCP servers, and which of them admit rewake's is not settled", Gates: gates}, nil
	}
	return harness.ToolDecision{Inject: true, Gates: gates}, nil
}

// gatesLeaveOut is the gates' share of the choice, "" when they leave the
// tool in: the same before the claim and again at the start, when the
// version is not confirmed and the choice is made with none.
func gatesLeaveOut(gates harness.Gates, version harness.Version) string {
	if !gates.Open(harness.GateG2) {
		return ""
	}
	if version.Value == "" {
		return version.Note()
	}
	return "gate G2: the servers a thread registers beyond the configuration cannot be listed yet"
}

// ReadLaunchVersion is the launch's one --version, before the claim and on
// every launch: the transport's compatibility note takes it, and the gates
// where the tool is chosen (docs/mail-bridge-version.md).
func (codexHarness) ReadLaunchVersion(program string, env []string, dir string) (harness.Version, error) {
	return harness.ReadVersion(program, env, dir)
}

// preflight runs the check's app-server and reads its answers.
func preflight(request harness.ToolCheckRequest, cwd string) (configReply, any, error) {
	program := request.Executable("codex")
	failed := func(outcome string) error {
		return &harness.CheckFailedError{Program: program, Label: "config/read", Outcome: outcome, Template: preflightTemplate}
	}
	// Private, beside the run's own state, and short enough for a socket path.
	dir, err := os.MkdirTemp(request.StateRoot, "chk")
	if err != nil {
		return configReply{}, nil, failed(harness.OutcomeNoStart)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	socket := filepath.Join(dir, "s")
	args := append(serverConfigArgs(request.Args), "app-server", "--listen", "unix://"+socket)
	env := append(append([]string{}, request.Env...), remoteControlOff)
	check, err := harness.StartCheck(harness.CheckSpec{
		Label: "config/read", Program: program, Args: args, Env: env, Dir: cwd, Bound: preflightBound,
	})
	if err != nil {
		return configReply{}, nil, failed(harness.OutcomeNoStart)
	}
	config, requirements, outcome := askServer(check, socket, cwd)
	if check.End() != nil {
		return configReply{}, nil, failed(harness.OutcomeNotEnded)
	}
	if outcome != "" {
		return configReply{}, nil, failed(outcome)
	}
	return config, requirements, nil
}

// askServer connects to the check's server once it listens and asks both
// questions, within the check's bound. It answers an outcome class on
// failure, never the server's words.
func askServer(check *harness.CheckProcess, socket, cwd string) (configReply, any, string) {
	ctx, cancel := context.WithDeadline(context.Background(), check.Deadline())
	defer cancel()
	go func() {
		select {
		case <-check.Exited():
			cancel()
		case <-ctx.Done():
		}
	}()
	client, err := waitForServer(ctx, socket)
	if err != nil {
		return configReply{}, nil, boundOrExit(check)
	}
	defer client.close()
	return readConfig(ctx, client, cwd, check)
}

// readConfig asks config/read with layers and configRequirements/read.
func readConfig(ctx context.Context, client *rpcClient, cwd string, check *harness.CheckProcess) (configReply, any, string) {
	var raw json.RawMessage
	if err := client.call(ctx, "config/read", map[string]any{"cwd": cwd, "includeLayers": true}, &raw); err != nil {
		return configReply{}, nil, callOutcome(err, check)
	}
	config, ok := parseConfigReply(raw)
	if !ok {
		return configReply{}, nil, harness.OutcomeUnrecognized
	}
	var rawRequirements json.RawMessage
	if err := client.call(ctx, "configRequirements/read", map[string]any{}, &rawRequirements); err != nil {
		return configReply{}, nil, callOutcome(err, check)
	}
	var requirements map[string]any
	if json.Unmarshal(rawRequirements, &requirements) != nil || requirements == nil {
		return configReply{}, nil, harness.OutcomeUnrecognized
	}
	return config, requirements, ""
}

// waitForServer connects once the server listens, retrying until ctx ends.
func waitForServer(ctx context.Context, socket string) (*rpcClient, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		client, err := connectRPC(ctx, socket, nil)
		if err == nil {
			return client, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// callOutcome classes a failed call: an error the server answered is not
// recognized, anything else is the bound or the server's exit. Results are
// taken raw, so no decoding error reaches here.
func callOutcome(err error, check *harness.CheckProcess) string {
	if _, answered := err.(*rpcError); answered {
		return harness.OutcomeUnrecognized
	}
	return boundOrExit(check)
}

// boundOrExit says why no answer came: the server exited, or the bound passed.
func boundOrExit(check *harness.CheckProcess) string {
	select {
	case <-check.Exited():
		if code, ok := check.Wait(); ok {
			return harness.OutcomeExit(code)
		}
	default:
	}
	return harness.OutcomeBound
}

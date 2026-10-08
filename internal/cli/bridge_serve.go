package cli

import (
	"encoding/json"
	"os"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/bridge/server"
	"github.com/praline-labs/rewake/internal/state"
)

// The internal commands of the mail tool (docs/mail-bridge-server.md): the
// server the harness starts, and the hook that tells the wrapper what the
// harness recorded of a call.
const (
	BridgeServe = "bridge-serve"
	BridgeHook  = "bridge-hook"
)

func init() { validateTicket = confirmTicket }

// confirmTicket has the run's wrapper confirm a ticket, once, for this
// process.
func confirmTicket(dir, name, epoch string, ticket bridge.Ticket) error {
	return endpoint.Confirm(state.ContextPath(dir, name, epoch), ticket)
}

// handleBridgeServe runs the server until the harness closes its stdin. A
// server missing its launch values still runs: every call it gets is then
// refused as one no wrapper can authorize.
func handleBridgeServe(ctx *Context, _ Call) error {
	cfg := server.Config{
		Name: os.Getenv(state.SessionEnv), Epoch: os.Getenv(state.EpochEnv),
		Capability: os.Getenv(bridge.CapabilityEnv),
		Words:      ToolCheck,
		Executable: "/proc/self/exe",
		Env:        server.ChildEnv(os.Getenv),
		Version:    Version,
	}
	if dir, err := state.Dir(); err == nil && cfg.Name != "" && cfg.Epoch != "" {
		cfg.Dir, cfg.Endpoint = dir, state.ContextPath(dir, cfg.Name, cfg.Epoch)
	}
	return server.Run(cfg, os.Stdin, ctx.Stdout)
}

// handleBridgeHook passes a hook's input to the wrapper and returns once the
// wrapper recorded it or its bound ran out. It prints nothing and never
// fails: it changes no permission decision.
func handleBridgeHook(_ *Context, call Call) error {
	socket := ""
	if len(call.Raw) > 0 {
		socket = call.Raw[0]
	}
	payload := readPayload(os.Stdin)
	var event struct {
		Name string `json:"hook_event_name"`
	}
	_ = json.Unmarshal(payload, &event)
	limit := endpoint.HookWait
	if event.Name != "PreToolUse" {
		limit += endpoint.ConfirmWait
	}
	_ = endpoint.Observe(socket, payload, endpoint.LimitsFrom(os.LookupEnv), limit)
	return nil
}

package claude

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/praline-labs/rewake/internal/harness"
)

// What a launch adds for the mail tool on Claude Code
// (docs/mail-bridge-launch.md#claude-code): one --mcp-config file holding only
// our server, one allowance naming only our tool, and the hooks that tell the
// run's wrapper what the harness recorded of each call. Nothing else of the
// person's configuration is touched, and a caller's --strict-mcp-config stays.

// The hook events the observer runs on.
const (
	postToolUse        = "PostToolUse"
	postToolUseFailure = "PostToolUseFailure"
)

// bridgeHookTimeout bounds the observer hook. It runs in the foreground, so
// the observation is in place before the harness runs the call.
const bridgeHookTimeout = 5

// toolServerTimeout is our server's own timeout, in milliseconds: the
// ticket's deadline of 25 seconds falls inside it.
const toolServerTimeout = 30000

// BridgeHookCommand is the observer hook's command line for a run.
func BridgeHookCommand(tool *harness.ToolServer) string {
	return harness.ShellQuote([]string{tool.Executable, "bridge-hook", tool.ContextSocket})
}

// observeTool adds the observer hooks, matching our tool only, after
// rewake's other hooks of each event; merged, they follow the person's own.
func (l launchLayer) observeTool(command string) {
	if command == "" {
		return
	}
	entry := hookEntry{Kind: "command", Command: command, Timeout: bridgeHookTimeout}
	for _, event := range []string{preToolUse, postToolUse, postToolUseFailure} {
		l.hooks[event] = append(l.hooks[event], hookMatcher{Matcher: harness.ToolName, Hooks: []hookEntry{entry}})
	}
}

// serverFile is the --mcp-config file of a run: our server and nothing else.
func serverFile(tool *harness.ToolServer) ([]byte, error) {
	env := map[string]string{}
	for _, value := range tool.Env {
		env[value.Name] = value.Value
	}
	entry := map[string]any{
		"type": "stdio", "command": tool.Executable, "args": harness.ToolArgs,
		"env": env, "timeout": toolServerTimeout,
	}
	return json.Marshal(map[string]any{"mcpServers": map[string]any{"rewake": entry}})
}

// injectTool writes the run's server file, 0600 — it holds the capability,
// which a file keeps off the command line — and adds the flags naming it and
// allowing the tool. The allowance is a flag of its own, beside whatever the
// caller allowed under either spelling: the harness applies both.
func injectTool(args []string, tool *harness.ToolServer) ([]string, error) {
	raw, err := serverFile(tool)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(tool.ConfigFile)
	file, err := os.OpenFile(tool.ConfigFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("could not write the tool's server file: %w", err)
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		_ = os.Remove(tool.ConfigFile)
		return nil, fmt.Errorf("could not write the tool's server file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tool.ConfigFile)
		return nil, fmt.Errorf("could not write the tool's server file: %w", err)
	}
	return harness.AddFlags(args, mcpConfigFlag, tool.ConfigFile, toolFlag, harness.ToolName), nil
}

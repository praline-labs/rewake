package codex

import (
	"encoding/json"
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
)

// What a launch adds for the mail tool on Codex
// (docs/mail-bridge-launch-codex.md#what-is-added): leaves under the absent
// mcp_servers.rewake table, as -c values to the owned app-server and to the
// terminal, after the caller's own, and nothing outside that table.

// toolTable is the table our leaves live under.
var toolTable = []string{"mcp_servers", "rewake"}

// toolTimeoutSec bounds a call on Codex's side: the ticket's deadline of 25
// seconds falls inside it.
const toolTimeoutSec = 30

// toolLeaf is one leaf of our entry: its path below toolTable, its value as
// config/read answers it, and the same value in TOML for -c.
type toolLeaf struct {
	path  []string
	value any
	toml  string
}

// toolLeaves are the leaves a run adds, in the order they are passed.
func toolLeaves(tool *harness.ToolServer) []toolLeaf {
	leaves := []toolLeaf{
		{path: []string{"command"}, value: tool.Executable, toml: quoteTOML(tool.Executable)},
		{path: []string{"args"}, value: stringsAny(harness.ToolArgs), toml: tomlStrings(harness.ToolArgs)},
	}
	for _, env := range tool.Env {
		leaves = append(leaves, toolLeaf{path: []string{"env", env.Name}, value: env.Value, toml: quoteTOML(env.Value)})
	}
	return append(leaves,
		toolLeaf{path: []string{"enabled_tools"}, value: []any{"rewake"}, toml: tomlStrings([]string{"rewake"})},
		toolLeaf{path: []string{"tool_timeout_sec"}, value: float64(toolTimeoutSec), toml: "30"},
		toolLeaf{path: []string{"default_tools_approval_mode"}, value: "approve", toml: quoteTOML("approve")},
		toolLeaf{path: []string{"omit_tools_from"}, value: []any{"code_mode"}, toml: tomlStrings([]string{"code_mode"})},
	)
}

// toolConfigArgs are the -c arguments of the leaves.
func toolConfigArgs(leaves []toolLeaf) []string {
	args := make([]string, 0, 2*len(leaves))
	for _, leaf := range leaves {
		key := strings.Join(append(append([]string{}, toolTable...), leaf.path...), ".")
		args = append(args, configFlag, key+"="+leaf.toml)
	}
	return args
}

// injectTool adds the leaves after the caller's arguments; serverConfigArgs
// then forwards every -c to the app-server in the same order.
func injectTool(args []string, tool *harness.ToolServer) ([]string, []toolLeaf) {
	if tool == nil {
		return args, nil
	}
	leaves := toolLeaves(tool)
	return harness.AddFlags(args, toolConfigArgs(leaves)...), leaves
}

func tomlStrings(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, quoteTOML(value))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func stringsAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// sameJSON compares two values as JSON decodes them: numbers as float64,
// arrays as []any, tables as map[string]any.
func sameJSON(a, b any) bool {
	left, err1 := json.Marshal(a)
	right, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(left) == string(right)
}

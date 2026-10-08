package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
)

// The name check of the mail tool on Claude Code, before the run is claimed
// (docs/mail-bridge-launch.md#claude-code): which launches get no tool, and
// for the rest every source of a server named rewake the check covers.

var (
	_ harness.MailToolHarness = claudeHarness{}
	_ harness.ToolTimeout     = claudeHarness{}
)

// ToolTimeoutVariable is where the person sets how long Claude Code waits
// for an MCP tool call.
func (claudeHarness) ToolTimeoutVariable() string { return "MCP_TOOL_TIMEOUT" }

// mcpConfigFlag takes a list: every word up to the next one that starts with
// a dash, the =form, and repeated flags.
const mcpConfigFlag = "--mcp-config"

// mcpGetBound bounds `mcp get rewake`; a variable so a test of a hanging
// check need not wait the whole of it.
var mcpGetBound = 10 * time.Second

// mcpGetTemplate is what the person may run by hand when the check fails.
const mcpGetTemplate = "claude mcp get rewake"

// CheckMailTool decides the tool for a launch and proves the name free.
// The version is taken last among the reasons for no tool: only where a
// closed gate could still change the choice (docs/mail-bridge-version.md).
func (claudeHarness) CheckMailTool(request harness.ToolCheckRequest) (harness.ToolDecision, error) {
	gates := harness.ResolveGates("claude", "", request.Assumed)
	noTool := func(reason string) (harness.ToolDecision, error) {
		return harness.ToolDecision{Reason: reason, Gates: gates}, nil
	}
	args := harness.BeforeTerminator(request.Args)
	if ownWorktree(args) {
		return noTool("-w: the harness picks its worktree after the launch, where no check can run")
	}
	if harness.HasFlag(args, bareFlag) {
		return noTool("--bare: no hook is known to observe the tool's calls")
	}
	if !settingsMergeable(args, request.Cwd) {
		return noTool("the --settings given cannot be read or merged, so no hook would observe the tool's calls")
	}
	if reason := managedMCP(); reason != "" {
		return noTool(reason)
	}
	version := harness.Version{Unknown: harness.VersionNotRead}
	switch {
	case request.Version != nil:
		version = *request.Version
	case harness.GatesNeedVersion("claude"):
		version = harness.ClaudeVersion(request.Env)
	}
	gates = harness.ResolveGates("claude", version.Value, request.Assumed)
	if gates.Open(harness.GateG7) {
		if version.Value == "" {
			return noTool(version.Note())
		}
		return noTool("gate G7: whether a plugin's server can be named rewake is not settled")
	}
	if err := checkMCPConfigs(args, request.Cwd); err != nil {
		return harness.ToolDecision{}, err
	}
	if err := checkParentProjects(request.Cwd); err != nil {
		return harness.ToolDecision{}, err
	}
	if err := mcpGet(request); err != nil {
		return harness.ToolDecision{}, err
	}
	return harness.ToolDecision{Inject: true, Gates: gates}, nil
}

// ownWorktree says whether the harness's own -w is among the arguments, in
// any spelling its parser takes: -w, -w=<name>, -w<name>, or w among the
// letters of a group of short flags. The group form over-covers rather than
// misses.
func ownWorktree(args []string) bool {
	for _, arg := range args {
		if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.ContainsRune(arg[1:], 'w') {
			return true
		}
	}
	return false
}

// settingsMergeable says whether the caller's --settings, the last one as
// the harness reads it, is one rewake's hooks can be merged into.
func settingsMergeable(args []string, cwd string) bool {
	values := harness.FlagValues(args, settingsFlag)
	if len(values) == 0 {
		return true
	}
	caller, err := readCallerLayer(values[len(values)-1], cwd)
	if err != nil {
		return false
	}
	// The probe carries a status line, as a run's layer does: a caller's
	// status line the merge cannot take fails the launch's merge too.
	probe := launchLayer{hooks: map[string][]hookMatcher{preToolUse: {{Matcher: harness.ToolName}}}, status: "probe"}
	_, err = probe.merge(caller)
	return err == nil
}

// MCPConfigValues are the values of every --mcp-config, in order: the words
// after the flag up to the next that starts with a dash, the =form, repeated
// flags, and nothing after "--". harness.FlagValues takes one word per flag,
// which would miss the second file of a list.
func MCPConfigValues(args []string) []string {
	visible := harness.BeforeTerminator(args)
	var values []string
	for index := 0; index < len(visible); index++ {
		arg := visible[index]
		if value, joined := strings.CutPrefix(arg, mcpConfigFlag+"="); joined {
			values = append(values, value)
			continue
		}
		if arg != mcpConfigFlag {
			continue
		}
		for index+1 < len(visible) && !strings.HasPrefix(visible[index+1], "-") {
			index++
			values = append(values, visible[index])
		}
	}
	return values
}

// checkMCPConfigs reads every --mcp-config value, a file from the launch
// directory or inline JSON; one that cannot be read or parsed refuses, as
// the harness would fail on it.
func checkMCPConfigs(args []string, cwd string) error {
	for position, value := range MCPConfigValues(args) {
		where := harness.Where{Flag: mcpConfigFlag + " value", Position: position + 1}
		raw := []byte(strings.TrimSpace(value))
		if !bytes.HasPrefix(raw, []byte("{")) {
			path := value
			if !filepath.IsAbs(path) {
				path = filepath.Join(cwd, path)
			}
			read, err := os.ReadFile(path)
			if err != nil {
				return &harness.CheckFailedError{Where: &where, Outcome: harness.OutcomeUnreadable}
			}
			raw = read
		}
		named, ok := namesRewake(raw)
		if !ok {
			return &harness.CheckFailedError{Where: &where, Outcome: harness.OutcomeUnreadable}
		}
		if named {
			return &harness.NameTakenError{Where: where}
		}
	}
	return nil
}

// checkParentProjects reads every .mcp.json above the launch directory, up
// to the root. Whether the harness reads them is gate G5; reading them all
// over-covers, as for --mcp-config. The launch directory's own is mcp get's.
func checkParentProjects(cwd string) error {
	dir := filepath.Clean(cwd)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
		path := filepath.Join(dir, ".mcp.json")
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		where := harness.Where{Scope: harness.ScopeProject, Path: path}
		if err != nil {
			return &harness.CheckFailedError{Where: &where, Outcome: harness.OutcomeUnreadable}
		}
		named, ok := namesRewake(raw)
		if !ok {
			return &harness.CheckFailedError{Where: &where, Outcome: harness.OutcomeUnreadable}
		}
		if named {
			return &harness.NameTakenError{Where: where}
		}
	}
}

// namesRewake reads a server file: whether its mcpServers name rewake, and
// whether it could be read as one at all.
func namesRewake(raw []byte) (bool, bool) {
	var file struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &file) != nil {
		return false, false
	}
	_, named := file.Servers["rewake"]
	return named, true
}

// mcpGet asks the harness itself, in the launch directory and with its
// environment. Absent, it exits 1 saying so; present, it exits 0 with a
// Scope line — and has started an approved entry once, as its health check
// does. Its output prints the entry's environment: only the Scope line is
// read, and none of it is shown.
func mcpGet(request harness.ToolCheckRequest) error {
	program := request.Executable(ID)
	failed := func(outcome string) error {
		return &harness.CheckFailedError{Program: program, Label: "mcp get", Outcome: outcome, Template: mcpGetTemplate}
	}
	check, err := harness.StartCheck(harness.CheckSpec{
		Label: "mcp get", Program: program, Args: []string{"mcp", "get", "rewake"},
		Env: request.Env, Dir: request.Cwd, Bound: mcpGetBound, Capture: true,
	})
	if err != nil {
		return failed(harness.OutcomeNoStart)
	}
	code, answered := check.Wait()
	if err := check.End(); err != nil {
		return failed(harness.OutcomeNotEnded)
	}
	if !answered {
		return failed(harness.OutcomeBound)
	}
	return readMCPGet(code, check.Output(), failed)
}

// readMCPGet reads mcp get's answer, as recorded on 2.1.284.
func readMCPGet(code int, output []byte, failed func(string) error) error {
	text := string(output)
	switch code {
	case 1:
		if strings.Contains(text, `No MCP server named "rewake"`) {
			return nil
		}
		return failed(harness.OutcomeUnrecognized)
	case 0:
		for _, line := range strings.Split(text, "\n") {
			label, value, found := strings.Cut(strings.TrimSpace(line), ":")
			if found && label == "Scope" {
				return &harness.NameTakenError{Where: harness.Where{Scope: scopeOf(value)}, Started: true}
			}
		}
		return failed(harness.OutcomeUnrecognized)
	}
	return failed(harness.OutcomeExit(code))
}

// scopeOf names a Scope line's value in the diagnostics' vocabulary; the
// value itself is never shown. Only its first word names the scope: the
// words after it explain, and "User config (available in all your
// projects)" carries another scope's name among them.
func scopeOf(value string) string {
	fields := strings.Fields(strings.ToLower(value))
	if len(fields) == 0 {
		return harness.ScopeUnnamed
	}
	switch fields[0] {
	case "local":
		return harness.ScopeLocal
	case "project":
		return harness.ScopeProject
	case "user":
		return harness.ScopeUser
	case "managed", "enterprise":
		return harness.ScopeManaged
	}
	return harness.ScopeUnnamed
}

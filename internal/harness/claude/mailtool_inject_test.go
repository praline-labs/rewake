package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

// What a launch adds for the tool (docs/mail-bridge-launch.md#claude-code):
// launched with and without it over the caller's lines, the two differ only by
// rewake's additions, and nothing of the person's configuration changes a byte.

const capability = "CAPABILITY-secret-value"

// digestTree is every path under root with its mode and content's digest.
func digestTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		digest := info.Mode().String()
		if entry.Type().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(raw)
			digest += " " + hex.EncodeToString(sum[:])
		}
		tree[path] = digest
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// personsFiles writes the configuration a person keeps: user and local
// scopes, project servers and settings, each carrying the sentinel.
func personsFiles(t *testing.T, home, cwd string) {
	t.Helper()
	for path, content := range map[string]string{
		filepath.Join(home, ".claude.json"):                  `{"mcpServers":{"mine":{"command":"x","env":{"K":"` + sentinel + `"}}}}`,
		filepath.Join(home, ".claude", "settings.json"):      `{"permissions":{"allow":["Read"]},"env":{"K":"` + sentinel + `"}}`,
		filepath.Join(cwd, ".mcp.json"):                      `{"mcpServers":{"project":{"command":"` + sentinel + `"}}}`,
		filepath.Join(cwd, ".claude", "settings.local.json"): `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"` + sentinel + `"}]}]}}`,
		filepath.Join(cwd, "caller-settings.json"):           `{"hooks":{"PostToolUse":[{"matcher":"Edit","hooks":[{"type":"command","command":"fmt"}]}]},"statusLine":{"type":"command","command":"mine","padding":1}}`,
		filepath.Join(cwd, "servers.json"):                   `{"mcpServers":{"other":{"command":"x"}}}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// callerLines are what a caller may pass beside the tool.
var callerLines = [][]string{
	nil,
	{"--allowedTools", "Read"},
	{"--allowed-tools", "Bash(git:*)", "Edit"},
	{"--allowedTools=Read"},
	{"--strict-mcp-config", "--mcp-config", "servers.json"},
	{"--mcp-config", "servers.json", `{"mcpServers":{"x":{"command":"y"}}}`},
	{"--settings", "caller-settings.json"},
	{"--settings", `{"hooks":{"PreToolUse":[{"matcher":"mcp__rewake__rewake","hooks":[{"type":"command","command":"theirs"}]}]}}`},
	{"--model", "x", "--", "a prompt with -w and --mcp-config in it"},
	{"-c", "--permission-mode", "plan"},
}

func TestTheLaunchOnlyAddsTheTool(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	personsFiles(t, home, cwd)
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Chdir(cwd)
	before := []map[string]string{digestTree(t, home), digestTree(t, cwd)}
	for _, line := range callerLines {
		sock := t.TempDir()
		tool := harness.NewToolServer("/opt/rewake", "/state", "room", "api", "e1", capability,
			filepath.Join(sock, "ctx.sock"), filepath.Join(sock, "api.e1.mcp.json"), harness.ResolveGates(ID, "", nil))
		request := harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Args: line, Intro: true, Socket: filepath.Join(sock, "s.sock"), Role: role.General}
		without, err := New().Launch(request)
		if err != nil {
			t.Fatal(err)
		}
		request.MailTool = tool
		with, err := New().Launch(request)
		if err != nil {
			t.Fatal(err)
		}
		if with.ToolLeftOut != "" {
			t.Fatalf("%q: the tool was left out: %s", line, with.ToolLeftOut)
		}
		if problem := onlyAdded(without.Args, with.Args, tool); problem != "" {
			t.Fatalf("%q: %s\nwithout %q\nwith    %q", line, problem, without.Args, with.Args)
		}
		checkServerFile(t, tool)
	}
	after := []map[string]string{digestTree(t, home), digestTree(t, cwd)}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("a launch changed the person's files")
	}
}

// onlyAdded says how with differs from without beyond rewake's additions;
// "" when it does not.
func onlyAdded(without, with []string, tool *harness.ToolServer) string {
	if strings.Contains(strings.Join(with, "\x00"), capability) {
		return "the capability is on the command line"
	}
	end := slices.Index(with, "--")
	if end < 0 {
		end = len(with)
	}
	rest := slices.Clone(with)
	for _, pair := range [][2]string{{mcpConfigFlag, tool.ConfigFile}, {toolFlag, harness.ToolName}} {
		found := -1
		for index := 0; index+1 < end; index++ {
			if rest[index] == pair[0] && rest[index+1] == pair[1] {
				found = index
				break
			}
		}
		if found < 0 {
			return pair[0] + " " + pair[1] + " was not added before --"
		}
		rest = slices.Delete(rest, found, found+2)
		end -= 2
	}
	if len(rest) != len(without) {
		return "more than the two flags were added"
	}
	for index := range rest {
		switch {
		case index > 0 && rest[index-1] == settingsFlag:
			if problem := settingsAdded(without[index], rest[index], tool); problem != "" {
				return problem
			}
		case index > 0 && rest[index-1] == introFlag:
			if rest[index] == without[index] {
				return "the briefing does not mention the tool"
			}
		case rest[index] != without[index]:
			return "argument " + rest[index] + " differs from " + without[index]
		}
	}
	return ""
}

// settingsAdded says how the settings layer with the tool differs from the
// one without beyond the three observer hooks, appended after every other.
func settingsAdded(without, with string, tool *harness.ToolServer) string {
	var a, b map[string]any
	if json.Unmarshal([]byte(without), &a) != nil || json.Unmarshal([]byte(with), &b) != nil {
		return "a settings layer is not JSON"
	}
	hooks, _ := b["hooks"].(map[string]any)
	ours := map[string]any{"matcher": harness.ToolName, "hooks": []any{map[string]any{"type": "command", "command": BridgeHookCommand(tool), "timeout": float64(bridgeHookTimeout)}}}
	for _, event := range []string{preToolUse, postToolUse, postToolUseFailure} {
		matchers, _ := hooks[event].([]any)
		if len(matchers) == 0 || !reflect.DeepEqual(matchers[len(matchers)-1], ours) {
			return "the " + event + " observer is not the last hook of its event"
		}
		if len(matchers) == 1 {
			delete(hooks, event)
		} else {
			hooks[event] = matchers[:len(matchers)-1]
		}
	}
	if !reflect.DeepEqual(a, b) {
		return "the settings layer changed beyond the observer hooks"
	}
	return ""
}

// checkServerFile reads the run's server file: 0600, our server alone.
func checkServerFile(t *testing.T, tool *harness.ToolServer) {
	t.Helper()
	info, err := os.Stat(tool.ConfigFile)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the server file: %v %v", err, info)
	}
	raw, _ := os.ReadFile(tool.ConfigFile)
	var file struct {
		Servers map[string]struct {
			Kind    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			Timeout int               `json:"timeout"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &file) != nil || len(file.Servers) != 1 {
		t.Fatalf("the server file holds %s", raw)
	}
	entry, ok := file.Servers["rewake"]
	if !ok || entry.Kind != "stdio" || entry.Command != tool.Executable || !slices.Equal(entry.Args, harness.ToolArgs) ||
		entry.Timeout != toolServerTimeout || len(entry.Env) != len(tool.Env) {
		t.Fatalf("the server entry is %+v", entry)
	}
	for _, value := range tool.Env {
		if entry.Env[value.Name] != value.Value {
			t.Fatalf("the server's %s is %q", value.Name, entry.Env[value.Name])
		}
	}
}

// A layer that cannot carry the hooks leaves the tool out, and the launch
// is the same as one without it.
func TestALayerWithoutHooksLeavesTheToolOut(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	sock := t.TempDir()
	tool := harness.NewToolServer("/opt/rewake", "/state", "room", "api", "e1", capability,
		filepath.Join(sock, "ctx.sock"), filepath.Join(sock, "api.e1.mcp.json"), harness.ResolveGates(ID, "", nil))
	request := harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Args: []string{"--settings", `{"hooks":3}`}, Intro: true, Socket: filepath.Join(sock, "s.sock"), Role: role.General}
	without, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	request.MailTool = tool
	with, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	if with.ToolLeftOut == "" || !slices.Equal(with.Args, without.Args) {
		t.Fatalf("left out %q, args %q against %q", with.ToolLeftOut, with.Args, without.Args)
	}
	if _, err := os.Stat(tool.ConfigFile); err == nil {
		t.Fatal("a launch without the tool wrote its server file")
	}
}

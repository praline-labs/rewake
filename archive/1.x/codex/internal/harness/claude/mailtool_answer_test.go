package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
)

// What mcp get may answer, and the parent directories' .mcp.json: the
// answers' shape is 2.1.284's — absent, exit 1 with the sentence; present,
// exit 0 with a Scope line among lines that print the entry's environment.

// presentAnswer is mcp get's answer for an entry in a scope, its
// environment carrying the sentinel.
func presentAnswer(scope string) string {
	return `cat <<'EOF'
rewake:
  Scope: ` + scope + `
  Status: ✓ Connected
  Type: stdio
  Command: node
  Args: server.js --token ` + sentinel + `
  Environment:
    TOKEN=` + sentinel + `

To remove this server, run: claude mcp remove "rewake" -s x
EOF
exit 0`
}

func TestEveryAnswerOfMcpGet(t *testing.T) {
	previous := mcpGetBound
	mcpGetBound = 300 * time.Millisecond
	t.Cleanup(func() { mcpGetBound = previous })

	taken := func(scope string) func(error) string {
		return func(err error) string {
			var name *harness.NameTakenError
			if !errors.As(err, &name) || name.Where.Scope != scope || !name.Started {
				return "want the name taken in " + scope + " scope"
			}
			return ""
		}
	}
	failed := func(outcome string) func(error) string {
		return func(err error) string {
			var check *harness.CheckFailedError
			if !errors.As(err, &check) || check.Outcome != outcome || check.Template != mcpGetTemplate {
				return "want the check failed: " + outcome
			}
			if !strings.Contains(err.Error(), "claude mcp get") {
				return "the diagnostic does not name the program and its check"
			}
			return ""
		}
	}
	free := func(err error) string {
		if err != nil {
			return "want the name free"
		}
		return ""
	}
	for _, tc := range []struct {
		name, body string
		judge      func(error) string
	}{
		{"absent", absentAnswer, free},
		{"absent on stdout", `echo 'No MCP server named "rewake".'; exit 1`, free},
		{"user", presentAnswer("User config (available in all your projects)"), taken(harness.ScopeUser)},
		{"local", presentAnswer("Local config (private to you in this project)"), taken(harness.ScopeLocal)},
		{"project", presentAnswer("Project config (shared via .mcp.json)"), taken(harness.ScopeProject)},
		{"project pending", presentAnswer("Project config (shared via .mcp.json)") + "\n# ⏸ Pending approval", taken(harness.ScopeProject)},
		{"managed", presentAnswer("Managed config"), taken(harness.ScopeManaged)},
		{"enterprise", presentAnswer("Enterprise config"), taken(harness.ScopeManaged)},
		{"unknown scope", presentAnswer("Dynamic " + sentinel), taken(harness.ScopeUnnamed)},
		{"exit 0 without a scope", `echo "TOKEN=` + sentinel + `"; exit 0`, failed(harness.OutcomeUnrecognized)},
		{"exit 1 other words", `echo "error: ` + sentinel + `" >&2; exit 1`, failed(harness.OutcomeUnrecognized)},
		{"exit 2", `echo "` + sentinel + `"; exit 2`, failed(harness.OutcomeExit(2))},
		{"killed", `kill -9 $$`, failed(harness.OutcomeExit(137))},
		{"hangs", `echo "` + sentinel + `"; sleep 30`, failed(harness.OutcomeBound)},
		{"ignores TERM", `trap '' TERM; while :; do sleep 1; done`, failed(harness.OutcomeBound)},
		{"leaves a server running", `(trap '' HUP; sleep 30) & echo 'No MCP server named "rewake".'; exit 1`, free},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cwd := filepath.Join(t.TempDir(), "launch")
			config := filepath.Join(t.TempDir(), "config")
			if err := os.MkdirAll(cwd, 0o700); err != nil {
				t.Fatal(err)
			}
			mark := filepath.Join(t.TempDir(), "mark")
			program := fakeClaude(t, t.TempDir(), mark, tc.body)
			_, err := claudeHarness{}.CheckMailTool(harness.ToolCheckRequest{
				Command: program, Cwd: cwd, Assumed: []string{harness.GateG7},
				Env: []string{"PATH=" + os.Getenv("PATH"), "CLAUDE_CONFIG_DIR=" + config},
			})
			if problem := tc.judge(err); problem != "" {
				t.Fatalf("%s, got %v", problem, err)
			}
			if err != nil && strings.Contains(err.Error(), sentinel) {
				t.Fatalf("the diagnostic carries the answer: %v", err)
			}
			raw, readErr := os.ReadFile(mark)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if want := cwd + "|" + config + "|mcp get rewake\n"; string(raw) != want {
				t.Fatalf("the check ran as %q, want %q", raw, want)
			}
		})
	}
}

func TestAProgramThatCannotStartRefuses(t *testing.T) {
	_, err := claudeHarness{}.CheckMailTool(harness.ToolCheckRequest{
		Command: filepath.Join(t.TempDir(), "absent"), Cwd: t.TempDir(),
		Assumed: []string{harness.GateG7},
	})
	var check *harness.CheckFailedError
	if !errors.As(err, &check) || check.Outcome != harness.OutcomeNoStart {
		t.Fatalf("got %v", err)
	}
}

// Every .mcp.json above the launch directory is read, up to the root; the
// launch directory's own is mcp get's (gate G5, read over-covering).
func TestTheParentDirectoriesServerFiles(t *testing.T) {
	for _, tc := range []struct {
		name    string
		files   map[string]string // directory below the root, content
		refuses string            // directory whose file refuses, "" for none
		outcome string            // "" for the name taken
	}{
		{name: "none"},
		{name: "own directory only", files: map[string]string{"a/b/c": `{"mcpServers":{"rewake":{}}}`}},
		{name: "parent names it", files: map[string]string{"a/b": `{"mcpServers":{"rewake":{"env":{"K":"` + sentinel + `"}}}}`}, refuses: "a/b"},
		{name: "root of the tree names it", files: map[string]string{"a": `{"mcpServers":{"rewake":{}}}`}, refuses: "a"},
		{name: "parent clean", files: map[string]string{"a/b": `{"mcpServers":{"other":{}}}`, "a": `{}`}},
		{name: "nearest refuses first", files: map[string]string{"a/b": `{"mcpServers":{"rewake":{}}}`, "a": `{"mcpServers":{"rewake":{}}}`}, refuses: "a/b"},
		{name: "broken parent", files: map[string]string{"a": `{"mcpServers": ` + sentinel}, refuses: "a", outcome: harness.OutcomeUnreadable},
		{name: "unreadable parent", files: map[string]string{"a/b": "unreadable"}, refuses: "a/b", outcome: harness.OutcomeUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cwd := filepath.Join(root, "a", "b", "c")
			if err := os.MkdirAll(cwd, 0o700); err != nil {
				t.Fatal(err)
			}
			for dir, content := range tc.files {
				path := filepath.Join(root, dir, ".mcp.json")
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				if content == "unreadable" {
					if err := os.Chmod(path, 0); err != nil {
						t.Fatal(err)
					}
					if _, err := os.ReadFile(path); err == nil {
						t.Skip("this user reads a file of mode 0")
					}
				}
			}
			mark := filepath.Join(t.TempDir(), "mark")
			program := fakeClaude(t, t.TempDir(), mark, absentAnswer)
			_, err := claudeHarness{}.CheckMailTool(harness.ToolCheckRequest{
				Command: program, Cwd: cwd, Env: []string{"PATH=" + os.Getenv("PATH")},
				Assumed: []string{harness.GateG7},
			})
			if tc.refuses == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			want := harness.Where{Scope: harness.ScopeProject, Path: filepath.Join(root, tc.refuses, ".mcp.json")}
			var taken *harness.NameTakenError
			var check *harness.CheckFailedError
			switch {
			case tc.outcome == "" && errors.As(err, &taken) && taken.Where == want && !taken.Started:
			case tc.outcome != "" && errors.As(err, &check) && check.Where != nil && *check.Where == want && check.Outcome == tc.outcome:
			default:
				t.Fatalf("got %v, want %s at %+v", err, tc.outcome, want)
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatalf("the diagnostic carries the file: %v", err)
			}
			if _, statErr := os.Stat(mark); statErr == nil {
				t.Fatal("mcp get ran after a refusal")
			}
		})
	}
}

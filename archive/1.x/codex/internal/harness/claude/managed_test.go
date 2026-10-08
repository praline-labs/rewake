package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// A managed MCP file is asked for by lstat alone: only ENOENT proves it
// absent; present in any form, or not checkable, it leaves the tool out, and
// it never refuses the launch (docs/mail-bridge-launch.md#claude-code).
func TestOnlyAMissingManagedMCPFileLetsTheToolIn(t *testing.T) {
	root := t.TempDir()
	notDir := filepath.Join(root, "file")
	if err := os.WriteFile(notDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		place func(path string) error
		dir   string
		want  string
	}{
		{"absent", func(string) error { return nil }, "", ""},
		{"absent directory", func(string) error { return nil }, filepath.Join(root, "none"), ""},
		{"a file", func(path string) error { return os.WriteFile(path, []byte("{}"), 0o600) }, "", "is present"},
		{"unreadable", func(path string) error { return os.WriteFile(path, []byte("{}"), 0o000) }, "", "is present"},
		{"a directory", func(path string) error { return os.Mkdir(path, 0o700) }, "", "is present"},
		{"a dangling link", func(path string) error { return os.Symlink(filepath.Join(root, "nowhere"), path) }, "", "is present"},
		{"a parent that is no directory", func(string) error { return nil }, notDir, "could not be checked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := managedDir
			t.Cleanup(func() { managedDir = previous })
			managedDir = tc.dir
			if managedDir == "" {
				managedDir = t.TempDir()
				if err := tc.place(filepath.Join(managedDir, "managed-mcp.json")); err != nil {
					t.Fatal(err)
				}
			}
			if got := managedMCP(); tc.want == "" && got != "" || !strings.Contains(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			mark := filepath.Join(t.TempDir(), "mark")
			program := fakeClaude(t, t.TempDir(), mark, absentAnswer)
			version := harness.Version{Value: "2.1.284"}
			decision, err := claudeHarness{}.CheckMailTool(harness.ToolCheckRequest{
				Command: program, Cwd: t.TempDir(), Version: &version,
				Env: []string{"PATH=" + os.Getenv("PATH"), "CLAUDE_CONFIG_DIR=" + t.TempDir()},
			})
			_, ran := os.Stat(mark)
			switch {
			case err != nil:
				t.Fatalf("the managed file refused the launch: %v", err)
			case tc.want == "" && (!decision.Inject || ran != nil):
				t.Fatalf("absent: got %+v, mcp get ran %v", decision, ran == nil)
			case tc.want != "" && (decision.Inject || !strings.Contains(decision.Reason, tc.want) || ran == nil):
				t.Fatalf("got %+v, mcp get ran %v", decision, ran == nil)
			}
		})
	}
}

// The rules that leave the tool out before the version read it: an unknown
// version is not the reason a -w launch has no tool.
func TestRuleFourLeavesTheToolOutBeforeTheVersion(t *testing.T) {
	previous := managedDir
	t.Cleanup(func() { managedDir = previous })
	managedDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(managedDir, "managed-mcp.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-w"}, {"--bare"}, nil} {
		decision, err := claudeHarness{}.CheckMailTool(harness.ToolCheckRequest{Args: args, Cwd: t.TempDir(), Env: []string{"PATH="}})
		if err != nil || decision.Inject || strings.Contains(decision.Reason, "version") {
			t.Fatalf("%q: got %+v %v", args, decision, err)
		}
	}
}

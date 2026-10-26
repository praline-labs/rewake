package claude

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLayer(t *testing.T, path, statusLine string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"model":"x","statusLine":`+statusLine+`}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The person's line is what Claude Code would draw: the project's settings
// over the user's, the local over the project's, key by key; a project-only
// line is found, and a linked worktree's local settings are the main
// checkout's.
func TestThePersonsLineFollowsClaudeCodesLayers(t *testing.T) {
	user, project := t.TempDir(), t.TempDir()
	if got := ownStatusLine(user, project); got.Command != "" {
		t.Fatalf("no line anywhere: %+v", got)
	}
	writeLayer(t, filepath.Join(project, ".claude", "settings.json"), `{"type":"command","command":"project-line"}`)
	if got := ownStatusLine(user, project); got.Type != "command" || got.Command != "project-line" {
		t.Fatalf("a project-only line: %+v", got)
	}
	writeLayer(t, filepath.Join(user, "settings.json"), `{"type":"command","command":"user-line","refreshInterval":4}`)
	if got := ownStatusLine(user, project); got.Command != "project-line" || got.RefreshInterval == nil {
		t.Fatalf("the project's over the user's, the user's interval kept: %+v", got)
	}
	writeLayer(t, filepath.Join(project, ".claude", "settings.local.json"), `{"command":"local-line"}`)
	if got := ownStatusLine(user, project); got.Command != "local-line" || got.Type != "command" {
		t.Fatalf("the local over the project's: %+v", got)
	}

	main := t.TempDir()
	if err := os.MkdirAll(filepath.Join(main, ".git", "worktrees", "w"), 0o700); err != nil {
		t.Fatal(err)
	}
	tree := t.TempDir()
	if err := os.WriteFile(filepath.Join(tree, ".git"), []byte("gitdir: "+filepath.Join(main, ".git", "worktrees", "w")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeLayer(t, filepath.Join(main, ".claude", "settings.local.json"), `{"type":"command","command":"main-local"}`)
	if got := ownStatusLine(t.TempDir(), tree); got.Command != "main-local" {
		t.Fatalf("a worktree's local settings: %+v", got)
	}
}

// rewake statusline runs the line of the project the input names.
func TestStatusLineRunsTheProjectsLine(t *testing.T) {
	project := t.TempDir()
	writeLayer(t, filepath.Join(project, ".claude", "settings.json"), `{"type":"command","command":"cat >/dev/null; echo project-line-marker"}`)
	t.Setenv(configDirEnv, t.TempDir())
	t.Setenv(modSocketEnv, "")
	var out bytes.Buffer
	claudeHarness{}.StatusLine(context.Background(), []byte(`{"workspace":{"project_dir":"`+project+`"}}`), &out)
	if strings.TrimSpace(out.String()) != "project-line-marker" {
		t.Fatalf("the project's line: %q", out.String())
	}
}

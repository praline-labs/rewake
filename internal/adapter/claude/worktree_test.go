package claude

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Claude Code works where it is started, so a launch in a worktree works in
// the very directory rewake changed into — not the checkout's root, not the
// directory above. The workflow suite held this on the Claude Code column
// until that column retired in S9; this test holds it until stage 4 brings
// the column back.
func TestALaunchWorksInTheDirectoryItWasStartedIn(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "repo", "service")
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	// Reached through a link, as a checkout under a linked home is: the
	// answer is the real path.
	link := filepath.Join(root, "linked")
	if err := os.Symlink(filepath.Join(root, "repo"), link); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(link, "service"))
	want, err := filepath.EvalSymlinks(inner)
	if err != nil {
		t.Fatal(err)
	}

	args := []string{"--model", "m", "fix the login"}
	dir, kept, err := New().(claudeHarness).LaunchDirectory(args)
	if err != nil {
		t.Fatal(err)
	}
	if dir != want {
		t.Errorf("launch directory = %s, want %s, where the session was started", dir, want)
	}
	if !slices.Equal(kept, args) {
		t.Errorf("arguments = %q, want them as given: Claude Code has no flag choosing where it works", kept)
	}
}

func TestAWorktreeRefusesWhatWouldWorkElsewhere(t *testing.T) {
	h := New().(claudeHarness)
	for _, refused := range [][]string{
		{"-w"},
		{"-pw"},
		{"--tmux"},
		{"--cloud"},
		{"--environment", "x"},
		{"--continue"},
		{"-c"},
		{"-pc"},
		{"--resume", "id"},
		{"-rID"},
		{"attach"},
		{"--fork-session"},
	} {
		err := h.WorktreeRefusal(refused)
		if err == nil {
			t.Errorf("%q beside --worktree was accepted", refused)
			continue
		}
		if strings.Contains(err.Error(), "continues one") && !strings.Contains(err.Error(), "rewake claude") {
			t.Errorf("%q: the refusal names no launch that starts: %v", refused, err)
		}
	}
	// A value letter ends a cluster, and a word after -- is the prompt's.
	for _, accepted := range [][]string{
		{"-dcache"}, {"-ncircle"}, {"--model", "m"}, {"--", "continue"}, {"--", "--continue"},
	} {
		if err := h.WorktreeRefusal(accepted); err != nil {
			t.Errorf("%q beside --worktree was refused: %v", accepted, err)
		}
	}
}

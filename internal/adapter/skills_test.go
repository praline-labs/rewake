package adapter

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The folders a harness reads skills from climb from main's directory to its
// repository's root, never to or past the home directory.
func TestTheFoldersClimbToTheRepository(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "work", "repo")
	deep := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, want := UpToRepository(deep, home), []string{deep, filepath.Join(repo, "a"), repo, filepath.Join(home, "work")}; !slices.Equal(got, want) {
		t.Errorf("without a repository: %q, want %q", got, want)
	}
	// A worktree's .git is a file, and it is the root as well.
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := UpToRepository(deep, home), []string{deep, filepath.Join(repo, "a"), repo}; !slices.Equal(got, want) {
		t.Errorf("in a repository: %q, want %q", got, want)
	}
	if got := UpToRepository(home, home); len(got) != 0 {
		t.Errorf("the home directory itself: %q", got)
	}
}

package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitPointerLimitIncludesTrailingPadding(t *testing.T) {
	base := t.TempDir()
	metadata := filepath.Join(base, "metadata")
	if err := os.Mkdir(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	pointer := filepath.Join(base, "pointer")
	raw := "gitdir: " + metadata + strings.Repeat("\n", 128*1024)
	if err := os.WriteFile(pointer, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := readGitPath(pointer, "gitdir: ")
	if err == nil {
		t.Fatalf("oversized pointer accepted: %d bytes resolves to %s", len(raw), path)
	}
}

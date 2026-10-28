package claude

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestClaudeCodesSkillFolders(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "repo")
	env := map[string]string{}
	getenv := func(name string) string { return env[name] }
	want := []string{filepath.Join(home, ".claude", "skills"), filepath.Join(dir, ".claude", "skills")}
	if got := (claudeHarness{}).SkillFolders(getenv, home, dir); !slices.Equal(got, want) {
		t.Errorf("folders %q, want %q", got, want)
	}
	env[configDirEnv] = "/config"
	if got := (claudeHarness{}).SkillFolders(getenv, home, dir); got[0] != filepath.Join("/config", "skills") {
		t.Errorf("the configuration directory's skills come first: %q", got)
	}
}

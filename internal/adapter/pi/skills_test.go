package pi

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestPisSkillFolders(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "repo")
	env := map[string]string{}
	getenv := func(name string) string { return env[name] }
	got := (harness{}).SkillFolders(getenv, home, dir)
	// The climb goes to the file system's root without a repository; the
	// user's .agents/skills is read last, once.
	want := []string{filepath.Join(dir, ".pi", "skills"), filepath.Join(dir, ".agents", "skills")}
	if !slices.Equal(got[:2], want) {
		t.Errorf("first %q, want %q", got[:2], want)
	}
	tail := []string{filepath.Join(home, ".pi", "agent", "skills"), filepath.Join(home, ".agents", "skills")}
	if !slices.Equal(got[len(got)-2:], tail) || slices.Index(got, tail[1]) != len(got)-1 {
		t.Errorf("last %q, want %q once", got, tail)
	}
	env[AgentDirEnv] = "/agent"
	if got := (harness{}).SkillFolders(getenv, home, dir); got[len(got)-2] != filepath.Join("/agent", "skills") {
		t.Errorf("the agent directory's skills: %q", got)
	}
}

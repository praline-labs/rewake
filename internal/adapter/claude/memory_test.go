package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// Every worktree of one repository, and any directory in it, is one
// project; two templates are two directories of it.
func TestMemoryIsPerProjectAndTemplate(t *testing.T) {
	top := t.TempDir()
	repo := filepath.Join(top, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "x")
	git(t, repo, "worktree", "add", "-q", filepath.Join(top, "wt"))
	other := filepath.Join(top, "plain")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	want, err := memoryDir("/f", projectRoot(repo), "opus-high-write")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(repo, "sub"), filepath.Join(top, "wt")} {
		if got, err := memoryDir("/f", projectRoot(dir), "opus-high-write"); err != nil || got != want {
			t.Errorf("%s: %s, want %s (%v)", dir, got, want, err)
		}
	}
	if filepath.Dir(want) != filepath.Join("/f", "memory", filepath.Base(filepath.Dir(want))) || filepath.Base(want) != "opus-high-write" {
		t.Fatalf("the layout: %s", want)
	}
	sonnet, _ := memoryDir("/f", projectRoot(filepath.Join(top, "wt")), "sonnet-medium-write")
	plain, _ := memoryDir("/f", projectRoot(other), "opus-high-write")
	if filepath.Dir(sonnet) != filepath.Dir(want) || sonnet == want || filepath.Dir(plain) == filepath.Dir(want) {
		t.Fatalf("templates %s, %s; another project %s", want, sonnet, plain)
	}
	for _, bad := range []string{"", "../x", "a/b", ".hidden"} {
		if _, err := memoryDir("/f", repo, bad); err == nil {
			t.Errorf("%q named a directory", bad)
		}
	}
}

// Version 2 takes out version 1's memory switch where it is still rewake's,
// and keeps the person's other changes.
func TestMemoryDefaultRetired(t *testing.T) {
	v1 := `{"rewake": {"defaults": 1}, "autoMemoryEnabled": false, "permissions": {"deny": ["SendMessage"]}}`
	dir := settingsFile(t, v1)
	path, settings, wrote, err := workerSettings(dir, "/home/p")
	if err != nil || !wrote {
		t.Fatalf("%v %v", wrote, err)
	}
	written := readBack(t, path)
	_, memory := written["autoMemoryEnabled"]
	if version, _ := settingsVersion(written); memory || version != defaultsVersion || len(denied(settings)) != 1 {
		t.Fatalf("%v", settings)
	}
}

// A worker's launch passes its memory on in the run's --settings, the
// directory made private; with the person's false, or raised from no
// template, none.
func TestRunSettingsWithMemory(t *testing.T) {
	folder, root := t.TempDir(), t.TempDir()
	passed, enter, notes, err := runSettings(map[string]any{"a": true}, folder, root, "opus", false, "/run")
	if err != nil || enter || len(notes) != 0 {
		t.Fatalf("%s %v %v", passed, notes, err)
	}
	overlay := overlayOf(t, passed)
	memory, _ := overlay["autoMemoryDirectory"].(string)
	if info, err := os.Stat(memory); err != nil || info.Mode().Perm() != 0o700 || filepath.Base(memory) != "opus" || overlay["autoMemoryEnabled"] != true || overlay["a"] != nil {
		t.Fatalf("%v: %v", overlay, err)
	}
	for _, c := range []struct {
		settings map[string]any
		key      string
	}{{map[string]any{"autoMemoryEnabled": false}, "opus"}, {map[string]any{}, ""}} {
		passed, _, _, err := runSettings(c.settings, folder, root, c.key, false, "/run")
		if overlay := overlayOf(t, passed); err != nil || overlay["autoMemoryEnabled"] != nil || overlay["autoMemoryDirectory"] != nil {
			t.Fatalf("%v %q: %v %v", c.settings, c.key, overlay, err)
		}
	}
}

// overlayOf decodes the JSON a run passes as its --settings.
func overlayOf(t *testing.T, passed string) map[string]any {
	t.Helper()
	overlay, err := parseSettings([]byte(passed))
	if err != nil {
		t.Fatalf("%s: %v", passed, err)
	}
	return overlay
}

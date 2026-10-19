package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
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
	want, err := memoryDir("/f", repo, "opus-high-write")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(repo, "sub"), filepath.Join(top, "wt")} {
		if got, err := memoryDir("/f", dir, "opus-high-write"); err != nil || got != want {
			t.Errorf("%s: %s, want %s (%v)", dir, got, want, err)
		}
	}
	if filepath.Dir(want) != filepath.Join("/f", "memory", filepath.Base(filepath.Dir(want))) || filepath.Base(want) != "opus-high-write" {
		t.Fatalf("the layout: %s", want)
	}
	sonnet, _ := memoryDir("/f", filepath.Join(top, "wt"), "sonnet-medium-write")
	plain, _ := memoryDir("/f", other, "opus-high-write")
	if filepath.Dir(sonnet) != filepath.Dir(want) || sonnet == want || filepath.Dir(plain) == filepath.Dir(want) {
		t.Fatalf("templates %s, %s; another project %s", want, sonnet, plain)
	}
	for _, bad := range []string{"", "../x", "a/b", ".hidden"} {
		if _, err := memoryDir("/f", repo, bad); err == nil {
			t.Errorf("%q named a directory", bad)
		}
	}
}

// The memory keys are laid over the settings and nothing else changes; the
// person's own false keeps the memory off.
func TestMemoryOverlay(t *testing.T) {
	settings := map[string]any{"idleCompaction": false, "rewake": map[string]any{"defaults": 2}}
	overlaid, on := withMemory(settings, "/m")
	if !on || len(overlaid) != 4 || overlaid["autoMemoryEnabled"] != true || overlaid["autoMemoryDirectory"] != "/m" || overlaid["idleCompaction"] != false {
		t.Fatalf("%v", overlaid)
	}
	if _, touched := settings["autoMemoryDirectory"]; touched {
		t.Fatal("the folder's settings were changed")
	}
	if _, on := withMemory(map[string]any{"autoMemoryEnabled": false}, "/m"); on {
		t.Fatal("the person's false was overridden")
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
	if version, _ := settingsVersion(written); memory || version != 2 || len(denied(settings)) != 1 {
		t.Fatalf("%v", settings)
	}
}

// A worker's launch passes a copy in its run directory with memory on, the
// directory made private; with the person's false, or raised from no
// template, it passes the file.
func TestLaunchSettingsWithMemory(t *testing.T) {
	base, err := os.MkdirTemp("", "mt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	runDir := filepath.Join(buildtest.Room(t, base, state.DefaultRoom), "runs", "api.e1")
	folder := t.TempDir()
	passed, notes, err := launchSettings("/f/settings.json", map[string]any{"a": true}, folder, runDir, "opus")
	if err != nil || len(notes) != 0 || passed != filepath.Join(runDir, "worker-settings.json") {
		t.Fatalf("%s %v %v", passed, notes, err)
	}
	settings := readBack(t, passed)
	memory, _ := settings["autoMemoryDirectory"].(string)
	if info, err := os.Stat(memory); err != nil || info.Mode().Perm() != 0o700 || filepath.Base(memory) != "opus" || settings["a"] != true {
		t.Fatalf("%v: %v", settings, err)
	}
	if passed, _, err := launchSettings("/f/settings.json", map[string]any{"autoMemoryEnabled": false}, folder, runDir, "opus"); err != nil || passed != "/f/settings.json" {
		t.Fatalf("the person's false: %s %v", passed, err)
	}
	if passed, _, err := launchSettings("/f/settings.json", map[string]any{}, folder, runDir, ""); err != nil || passed != "/f/settings.json" {
		t.Fatalf("no template: %s %v", passed, err)
	}
}

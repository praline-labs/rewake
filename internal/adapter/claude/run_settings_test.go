package claude

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A template that may enter a checkout is allowed EnterWorktree in its
// run's settings, and any other run has it denied there; an entry the
// folder's settings deny, or a launch with no run directory for the mod,
// gets none, said. Every run may read main's checkout.
func TestRunSettingsWithEntry(t *testing.T) {
	permissions := func(passed string) map[string]any {
		return overlayOf(t, passed)["permissions"].(map[string]any)
	}
	root := t.TempDir()
	read := "Read(/" + filepath.ToSlash(root) + "/**)"
	passed, enter, notes, err := runSettings(map[string]any{"autoMemoryEnabled": false}, t.TempDir(), root, "opus", true, "/run")
	if err != nil || !enter || len(notes) != 0 || !slices.Equal(permissions(passed)["allow"].([]any), []any{read, enterTool}) || permissions(passed)["deny"] != nil {
		t.Fatalf("entry: %s %v %v %v", passed, enter, notes, err)
	}
	passed, enter, _, _ = runSettings(map[string]any{}, t.TempDir(), root, "opus", false, "/run")
	if enter || !slices.Equal(permissions(passed)["deny"].([]any), []any{enterTool}) || !slices.Equal(permissions(passed)["allow"].([]any), []any{read}) {
		t.Fatalf("no entry: %s", passed)
	}
	person := map[string]any{"permissions": map[string]any{"deny": []any{"Agent", enterTool}}}
	for _, runDir := range []string{"/run", ""} {
		settings := map[string]any{}
		if runDir != "" {
			settings = person
		}
		passed, enter, notes, err := runSettings(settings, t.TempDir(), t.TempDir(), "opus", true, runDir)
		if err != nil || enter || len(notes) != 1 || !strings.HasPrefix(notes[0], "no worktree entry") || !slices.Equal(permissions(passed)["deny"].([]any), []any{enterTool}) {
			t.Fatalf("%q: %s %v %v %v", runDir, passed, enter, notes, err)
		}
	}
}

// A worker whose run may enter a checkout is told in its briefing that it
// starts in main's and enters before its first change; one that may not —
// no entry asked, or none possible without a run directory — is not, since
// it has no worktree_enter to call.
func TestTheBriefingFollowsTheRunsEntry(t *testing.T) {
	base, err := os.MkdirTemp("", "mt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	runDir := filepath.Join(buildtest.Room(t, base, state.DefaultRoom), "runs", "api.e1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, c := range []struct {
		enter  bool
		runDir string
		told   bool
	}{{true, runDir, true}, {false, runDir, false}, {true, "", false}} {
		plan, err := New().Launch(adapter.LaunchRequest{
			Name: "api", Dir: t.TempDir(), Intro: true, Role: role.Write, Template: "write",
			RunDir: c.runDir, WorktreeEntry: c.enter,
			Socket: filepath.Join(t.TempDir(), "api.sock"), Env: []string{"HOME=" + home},
		})
		if err != nil {
			t.Fatal(err)
		}
		intro := adapter.FlagValues(plan.Args, introFlag)
		if len(intro) != 1 {
			t.Fatalf("%+v: briefing %q", c, intro)
		}
		told := strings.Contains(intro[0], "\n"+entryBrief.Title+"\n") && strings.Contains(intro[0], "worktree_enter")
		if told != c.told || told != slices.Contains(plan.Env, adapter.WorktreeEnterEnv+"=1") {
			t.Fatalf("%+v: told %v, env %q, briefing %q", c, told, plan.Env, intro[0])
		}
	}
}

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

// A template that may enter a checkout gets a run copy even with its memory
// off, with EnterWorktree out of deny and allowed, the rest of deny kept and
// the person's settings untouched; without a run directory, no entry, said.
func TestLaunchSettingsWithEntry(t *testing.T) {
	base, err := os.MkdirTemp("", "mt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	runDir := filepath.Join(buildtest.Room(t, base, state.DefaultRoom), "runs", "api.e1")
	person := map[string]any{
		"autoMemoryEnabled": false,
		"permissions":       map[string]any{"deny": []any{"Agent", enterTool, "ExitWorktree", "EnterWorktree(x)"}},
	}
	passed, enter, notes, err := launchSettings("/f/settings.json", person, t.TempDir(), runDir, "opus", true)
	if err != nil || !enter || len(notes) != 0 || passed != filepath.Join(runDir, "worker-settings.json") {
		t.Fatalf("%s %v %v %v", passed, enter, notes, err)
	}
	permissions := readBack(t, passed)["permissions"].(map[string]any)
	if deny := permissions["deny"].([]any); !slices.Equal(deny, []any{"Agent", "ExitWorktree", "EnterWorktree(x)"}) {
		t.Fatalf("deny: %v", deny)
	}
	if allow := permissions["allow"].([]any); !slices.Equal(allow, []any{enterTool}) {
		t.Fatalf("allow: %v", allow)
	}
	if deny := denied(person); len(deny) != 4 || person["permissions"].(map[string]any)["allow"] != nil {
		t.Fatalf("the person's settings changed: %v", person)
	}
	passed, enter, notes, err = launchSettings("/f/settings.json", person, t.TempDir(), "", "opus", true)
	if err != nil || enter || passed != "/f/settings.json" || len(notes) != 2 {
		t.Fatalf("no run directory: %s %v %v %v", passed, enter, notes, err)
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

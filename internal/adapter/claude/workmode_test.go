package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A worker's runtime work mode reaches its briefing as the one behavioral
// section, and keys its memory: what it learns is about the kind of work, not
// the template (main's decision, October 21, 2026, then about types).
func TestAWorkModeReachesTheBriefingAndKeysTheMemory(t *testing.T) {
	base, err := os.MkdirTemp("", "mt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	runDir := filepath.Join(buildtest.Room(t, base, state.DefaultRoom), "runs", "api.e1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	mode := workmode.Mode{Name: "study", Text: "STUDY-TEXT", Source: workmode.User}
	plan, err := New().Launch(adapter.LaunchRequest{
		Name: "api", Dir: t.TempDir(), Intro: true, Role: role.Write, Template: "sonnet-write",
		RunDir: runDir, Mode: mode,
		Socket: filepath.Join(t.TempDir(), "api.sock"), Env: []string{"HOME=" + home},
	})
	if err != nil {
		t.Fatal(err)
	}
	intro := adapter.FlagValues(plan.Args, introFlag)
	if len(intro) != 1 || !strings.Contains(intro[0], "Runtime work mode: study.") || !strings.Contains(intro[0], "STUDY-TEXT") {
		t.Fatalf("the briefing %q", intro)
	}
	if strings.Contains(intro[0], "YOUR KIND OF WORK") {
		t.Fatalf("the old type briefing survived: %q", intro)
	}
	dir, _ := overlayOf(t, adapter.FlagValues(plan.Args, settingsFlag)[0])["autoMemoryDirectory"].(string)
	if filepath.Base(dir) != "study" {
		t.Fatalf("memory %q, want under study", dir)
	}
}

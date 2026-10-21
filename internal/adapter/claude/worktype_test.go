package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/worktype"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A worker raised for a type is told its text after its role's briefing, and
// keeps its memory under the type, not the template, in its project; one
// raised for none keeps it under its template as before.
func TestATypeReachesTheBriefingAndKeysTheMemory(t *testing.T) {
	base, err := os.MkdirTemp("", "mt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	runDir := filepath.Join(buildtest.Room(t, base, state.DefaultRoom), "runs", "api.e1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	for _, c := range []struct {
		work   worktype.Type
		memory string
	}{
		{worktype.Type{Name: "study", Text: "STUDY-TEXT"}, "study"},
		{worktype.Type{}, "sonnet-write"},
	} {
		plan, err := New().Launch(adapter.LaunchRequest{
			Name: "api", Dir: t.TempDir(), Intro: true, Role: role.Write, Template: "sonnet-write",
			RunDir: runDir, WorkType: c.work,
			Socket: filepath.Join(t.TempDir(), "api.sock"), Env: []string{"HOME=" + home},
		})
		if err != nil {
			t.Fatal(err)
		}
		intro := adapter.FlagValues(plan.Args, introFlag)
		told := len(intro) == 1 && strings.Contains(intro[0], "\nYOUR KIND OF WORK: study\n") && strings.Contains(intro[0], "STUDY-TEXT")
		if told != (c.work.Name != "") {
			t.Fatalf("%q: briefing %q", c.work.Name, intro)
		}
		dir, _ := settingsOf(t, plan.Args)["autoMemoryDirectory"].(string)
		if filepath.Base(dir) != c.memory {
			t.Fatalf("%q: memory %q, want under %s", c.work.Name, dir, c.memory)
		}
	}
}

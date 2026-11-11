package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// The run's line is rewake's command on rewake's interval, whatever the
// person's settings name: it shadows theirs, which is off in the run.
func TestTheRunsStatusLineIsRewakes(t *testing.T) {
	setting := statusLineSetting("/opt/it's/rewake")
	if setting["type"] != "command" || setting["command"] != `'/opt/it'\''s/rewake' statusline` || setting["refreshInterval"] != statusLineRefresh {
		t.Fatalf("the run's line: %v", setting)
	}
}

// A run with a mod is told its status line in its environment as well as in its
// --settings, since a --command wrapper's own --settings can be the last layer
// of the read Claude Code makes and would take the launch's away
// (docs/claude.md#the-cache-ttl); a launch with no mod is told nothing, there
// being no hook to read it.
func TestTheLaunchNamesTheStatusLineToTheRunMod(t *testing.T) {
	privateHome(t)
	sock := filepath.Join(t.TempDir(), "sock")
	named := func(plan adapter.LaunchPlan) []string {
		var found []string
		for _, kv := range plan.Env {
			if value, ok := strings.CutPrefix(kv, statusLineEnv+"="); ok {
				found = append(found, value)
			}
		}
		return found
	}
	want := statusLineSetting("/room/bin/rewake")
	for _, request := range []adapter.LaunchRequest{
		{Name: "api", Epoch: "e1", Dir: t.TempDir(), Socket: filepath.Join(sock, "api.e1.sock"), RunDir: filepath.Join(t.TempDir(), "api.e1"), Role: role.Write, Executable: "/room/bin/rewake"},
		// Main through a wrapper adds no --settings of its own, and the mod's
		// copy is what keeps the status line in the run.
		{Name: "main", Epoch: "e1", Dir: t.TempDir(), Socket: filepath.Join(sock, "main.e1.sock"), RunDir: filepath.Join(t.TempDir(), "main.e1"), Role: role.Main, Command: "wrapper", Executable: "/room/bin/rewake"},
	} {
		request.Env = []string{"HOME=" + os.Getenv("HOME"), statusLineEnv + "=/elsewhere"}
		plan, err := New().Launch(request)
		if err != nil {
			t.Fatal(err)
		}
		got := named(plan)
		if len(got) != 1 {
			t.Fatalf("%s: the status line named to the run: %q", request.Name, got)
		}
		var carried map[string]any
		if err := json.Unmarshal([]byte(got[0]), &carried); err != nil {
			t.Fatalf("%s: %v in %q", request.Name, err, got[0])
		}
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var expected map[string]any
		if err := json.Unmarshal(data, &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(carried, expected) {
			t.Fatalf("%s: the status line named to the run is %v, want %v", request.Name, carried, want)
		}
		if request.Command != "" && adapter.HasFlag(plan.Args, settingsFlag) {
			t.Fatalf("main's wrapped launch passed a competing %s: %q", settingsFlag, plan.Args)
		}
	}
	// A launch with no run directory loads no mod, and names no status line.
	plain, err := New().Launch(adapter.LaunchRequest{Name: "api", Epoch: "e1", Dir: t.TempDir(), Socket: filepath.Join(sock, "api.e2.sock"), Role: role.Main, Executable: "/room/bin/rewake"})
	if err != nil {
		t.Fatal(err)
	}
	if got := named(plain); len(got) != 0 {
		t.Fatalf("a launch with no mod was told a status line: %q", got)
	}
}

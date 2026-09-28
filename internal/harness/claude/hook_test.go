package claude

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

// The end of a turn is reported through a Stop hook in a settings layer passed
// for this launch only; Claude Code merges it with the user's own hooks.
func TestTurnHookIsLayeredForOneLaunch(t *testing.T) {
	plan := launch(t, nil, "")
	at := indexOf(plan.Args, settingsFlag)
	if at < 0 || at+1 >= len(plan.Args) {
		t.Fatalf("no %s in %v", settingsFlag, plan.Args)
	}
	var layer struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Kind    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(plan.Args[at+1]), &layer); err != nil {
		t.Fatalf("the settings layer is not JSON: %v", err)
	}
	stop := layer.Hooks["Stop"]
	if len(stop) != 1 || len(stop[0].Hooks) != 1 {
		t.Fatalf("Stop hooks = %+v, want exactly one", stop)
	}
	hook := stop[0].Hooks[0]
	if hook.Kind != "command" || !strings.HasSuffix(hook.Command, "'turn-ended'") {
		t.Errorf("hook = %+v, want a command running rewake turn-ended", hook)
	}
	// The command runs through a shell; running it for real is the only check
	// that the quoting holds and the path is the binary that exists.
	shell := exec.Command("sh", "-c", "set -- "+hook.Command+"; test -x \"$1\" && test \"$2\" = turn-ended")
	if out, err := shell.CombinedOutput(); err != nil {
		t.Errorf("the hook command does not name an executable followed by turn-ended: %v %s", err, out)
	}
	if len(layer.Hooks) != 4 || len(layer.Hooks["StopFailure"]) != 1 || len(layer.Hooks["PreToolUse"]) != 1 || len(layer.Hooks["PermissionRequest"]) != 1 {
		t.Errorf("hooks = %v, want Stop, StopFailure and the grant hooks", layer.Hooks)
	}
}

// The main session's turns are reported to nobody, so it gets no hook, and its
// briefing says what it is.
func TestMainObservesOnlyFailedTurns(t *testing.T) {
	plan, err := New().Launch(harness.LaunchRequest{Name: "lead", Dir: t.TempDir(), Intro: true, Role: role.Main})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	pos := indexOf(plan.Args, settingsFlag)
	if pos < 0 {
		t.Fatal("main has no failure hook")
	}
	var settings struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(plan.Args[pos+1]), &settings); err != nil {
		t.Fatal(err)
	}
	// Main may be given a directory too: it keeps the grant hooks.
	if len(settings.Hooks) != 3 || settings.Hooks["StopFailure"] == nil || settings.Hooks["PermissionRequest"] == nil {
		t.Fatalf("unexpected main hooks: %s", plan.Args[pos+1])
	}
	at := indexOf(plan.Args, introFlag)
	if at < 0 || !strings.Contains(plan.Args[at+1], "main session") {
		t.Errorf("args = %v, want a briefing that says this is the main session", plan.Args)
	}
}

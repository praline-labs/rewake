package claude

import (
	"encoding/json"
	"strings"
	"testing"
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
	if len(layer.Hooks) != 1 {
		t.Errorf("hooks = %v, want only Stop: the layer must not add anything else", layer.Hooks)
	}
}

// Only one --settings is read. The caller's is theirs, so rewake says it is not
// reporting turns instead of replacing it.
func TestCallerSettingsAreNotReplaced(t *testing.T) {
	plan := launch(t, []string{"--settings", "/home/u/team.json"}, "")
	count := 0
	for _, arg := range plan.Args {
		if arg == settingsFlag {
			count++
		}
	}
	if count != 1 || plan.Args[indexOf(plan.Args, settingsFlag)+1] != "/home/u/team.json" {
		t.Errorf("args = %v, want the caller's --settings alone", plan.Args)
	}
	if len(plan.Notes) == 0 || !strings.Contains(plan.Notes[0], "--settings") {
		t.Errorf("notes = %v, want a word on the turn reports that were skipped", plan.Notes)
	}
}

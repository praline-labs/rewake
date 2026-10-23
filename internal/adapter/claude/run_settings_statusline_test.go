package claude

import (
	"encoding/json"
	"fmt"
	"testing"
)

// A run with a directory for the mod gets rewake's status line in its
// --settings, which shadows the folder's own; a person's refresh interval
// there is kept, and a launch with no run directory has none.
func TestRunSettingsSetTheStatusLineOfARunWithAMod(t *testing.T) {
	line := func(settings map[string]any, runDir string) map[string]any {
		passed, _, _, err := runSettings(settings, t.TempDir(), t.TempDir(), "", false, runDir)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := overlayOf(t, passed)["statusLine"].(map[string]any)
		return got
	}
	got := line(map[string]any{}, "/run")
	command, _ := got["command"].(string)
	if got["type"] != "command" || fmt.Sprint(got["refreshInterval"]) != "30" || len(command) < len(" statusline") || command[len(command)-len(" statusline"):] != " statusline" {
		t.Fatalf("the status line: %v", got)
	}
	person := map[string]any{"statusLine": map[string]any{"type": "command", "command": "mine", "refreshInterval": json.Number("5")}}
	if got := line(person, "/run"); fmt.Sprint(got["refreshInterval"]) != "5" || got["command"] == "mine" {
		t.Fatalf("with the person's own: %v", got)
	}
	if got := line(map[string]any{}, ""); got != nil {
		t.Fatalf("without a run directory: %v", got)
	}
}

package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// A run with a directory for the mod gets rewake's status line in its
// --settings, which shadows the folder's own, interval and all, so the
// person's line is off in the run; a launch with no run directory has none.
func TestRunSettingsSetTheStatusLineOfARunWithAMod(t *testing.T) {
	line := func(settings map[string]any, runDir string) map[string]any {
		passed, _, _, err := runSettings(settings, t.TempDir(), t.TempDir(), "", false, runDir, "", "")
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
	if got := line(person, "/run"); fmt.Sprint(got["refreshInterval"]) != "30" || got["command"] == "mine" {
		t.Fatalf("with the person's own: %v", got)
	}
	if got := line(map[string]any{}, ""); got != nil {
		t.Fatalf("without a run directory: %v", got)
	}
}

// Main's launch gets the status line alone in its --settings, rewake's
// whatever the person's settings.json names, and one with --setting-sources
// gets it too; a launch with its own --settings keeps it and is told why
// main's row lacks the name, and one with no run directory or under
// --command gets nothing.
func TestMainsLaunchGetsTheStatusLineAlone(t *testing.T) {
	folder := t.TempDir()
	t.Setenv(configDirEnv, folder)
	// Main's project is the directory it starts in, here one with no layers.
	t.Chdir(t.TempDir())
	own := `{"model":"x","statusLine":{"type":"command","command":"mine","refreshInterval":7,"padding":2}}`
	if err := os.WriteFile(filepath.Join(folder, "settings.json"), []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	request := adapter.LaunchRequest{Role: role.Main, RunDir: "/run"}
	got := mainSettings(request, nil)
	if len(got.args) != 2 || got.args[0] != settingsFlag {
		t.Fatalf("the args: %q", got.args)
	}
	overlay := overlayOf(t, got.args[1])
	line, _ := overlay["statusLine"].(map[string]any)
	if len(overlay) != 1 || fmt.Sprint(line["refreshInterval"]) != "30" || line["command"] == "mine" {
		t.Fatalf("the overlay: %v", overlay)
	}
	for _, own := range [][]string{{settingsFlag, "{}"}, {settingsFlag + "={}"}} {
		if got := mainSettings(request, own); got.args != nil || len(got.notes) != 1 || !strings.Contains(got.notes[0], settingsFlag) {
			t.Fatalf("with the person's own %q: %+v", own, got)
		}
	}
	if got := mainSettings(request, []string{settingSourcesFlag, "user"}); len(got.args) != 2 {
		t.Fatalf("with --setting-sources: %+v", got)
	}
	for _, request := range []adapter.LaunchRequest{{Role: role.Main}, {Role: role.Main, RunDir: "/run", Command: "wrapper"}} {
		if got := mainSettings(request, nil); got.args != nil || got.notes != nil {
			t.Fatalf("%+v: %+v", request, got)
		}
	}
}

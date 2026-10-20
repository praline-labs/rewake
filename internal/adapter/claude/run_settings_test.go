package claude

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

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

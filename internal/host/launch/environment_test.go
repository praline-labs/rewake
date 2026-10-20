package launch

import (
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// A run's tools run the rewake that started it: the launch names its own
// executable over an inherited value, and a probe, which is no run, gets none.
func TestARunNamesItsOwnExecutable(t *testing.T) {
	t.Setenv(ExecutableEnv, "/elsewhere/rewake")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	values := func(env []string) []string {
		var found []string
		for _, entry := range env {
			if value, ok := strings.CutPrefix(entry, ExecutableEnv+"="); ok {
				found = append(found, value)
			}
		}
		return found
	}
	env := SessionEnv(adapter.LaunchRequest{Name: "api", Epoch: "1.2.boot", Dir: "/state/v2/rooms/default"}, nil)
	if got := values(env); len(got) != 1 || got[0] != self {
		t.Fatalf("the run's %s: %q, want %q", ExecutableEnv, got, self)
	}
	if got := values(ProbeEnv()); len(got) != 0 {
		t.Fatalf("a probe's %s: %q", ExecutableEnv, got)
	}
}

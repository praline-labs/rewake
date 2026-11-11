package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// --work-mode is space open's: list and close refuse it, as they refuse the
// other flags only open takes, and the old --type and --no-type are refused
// with what replaced them rather than as an unknown flag.
func TestTheWorkModeFlagBelongsToSpaceOpen(t *testing.T) {
	// A room with a build, so that close, a change, reaches its handler.
	liveSessionAs(t, "main-"+agentID, "main")
	t.Setenv(state.SessionEnv, "main-"+agentID)
	for _, argv := range [][]string{
		{"space", "list", "--work-mode", "review"},
		{"space", "close", "api", "--work-mode", "review"},
	} {
		code, _, errOut := run(argv...)
		if code != ExitUsage || !strings.Contains(errOut, "belongs to space open") {
			t.Errorf("%v: exit %d, %s", argv, code, errOut)
		}
	}
	for _, argv := range [][]string{
		{"space", "open", "api", "--alias", "w", "--type", "code"},
		{"space", "open", "api", "--alias", "w", "--no-type"},
	} {
		code, _, errOut := run(argv...)
		if code != ExitUsage || !strings.Contains(errOut, "no longer takes") || !strings.Contains(errOut, "--work-mode") {
			t.Errorf("%v: exit %d, %s", argv, code, errOut)
		}
	}
}

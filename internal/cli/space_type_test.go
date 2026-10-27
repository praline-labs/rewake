package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// --type and --no-type are space open's: list and close refuse them, as
// they refuse the other flags only open takes, rather than list or close as
// if they were not there.
func TestTheTypeFlagsBelongToSpaceOpen(t *testing.T) {
	// A room with a build, so that close, a change, reaches its handler.
	liveSessionAs(t, "main-"+agentID, "main")
	t.Setenv(state.SessionEnv, "main-"+agentID)
	for _, argv := range [][]string{
		{"space", "list", "--type", "review"},
		{"space", "list", "--no-type"},
		{"space", "close", "api", "--type", "review"},
		{"space", "close", "api", "--no-type"},
	} {
		code, _, errOut := run(argv...)
		if code != ExitUsage || !strings.Contains(errOut, "belongs to space open") {
			t.Errorf("%v: exit %d, %s", argv, code, errOut)
		}
	}
}

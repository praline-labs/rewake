package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// rewake restart names the parts it has: rewake itself is refused, naming
// the room's restart, before any host is asked; a part it does not have is
// a wrong call; and with no part and no manifest beside the build there is
// no plan, said as such.
func TestRestartRefusesARoomPart(t *testing.T) {
	t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "state"))
	t.Setenv(state.RoomEnv, "")
	t.Setenv(state.SessionEnv, "")
	t.Setenv(epochEnv, "")
	buildtest.Current(t)
	code, _, errOut := run("restart", "rewake")
	if code != ExitFailed || !strings.Contains(errOut, "rewake is loaded only by a restart of the room: end main and start it again") {
		t.Errorf("restart rewake: %d %q", code, errOut)
	}
	code, _, errOut = run("restart", "kettle")
	if code != ExitUsage || !strings.Contains(errOut, `no part "kettle" to restart: the parts are face, rewake, shell`) {
		t.Errorf("restart kettle: %d %q", code, errOut)
	}
	code, out, errOut := run("restart")
	if code != ExitOK || !strings.Contains(out, "Rewake: no plan: no manifest beside ") {
		t.Errorf("restart: %d %q %q", code, out, errOut)
	}
}

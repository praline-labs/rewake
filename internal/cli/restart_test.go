package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// rewake restart names the parts it has: rewake itself switches the room to
// the installed build, and has nothing to do when that is the room's; a part
// it does not have is a wrong call; and with no part and no manifest beside
// the build there is no plan, said as such.
func TestRestartNamesItsParts(t *testing.T) {
	t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "state"))
	t.Setenv(state.RoomEnv, "")
	t.Setenv(state.SessionEnv, "")
	t.Setenv(epochEnv, "")
	buildtest.Current(t)
	installSelf(t)
	code, out, errOut := run("restart", "rewake")
	if code != ExitOK || !strings.Contains(out, "Rewake: nothing to switch; the room runs the installed rewake") {
		t.Errorf("restart rewake: %d %q %q", code, out, errOut)
	}
	code, _, errOut = run("restart", "kettle")
	if code != ExitUsage || !strings.Contains(errOut, `no part "kettle" to restart: the parts are face, rewake, shell`) {
		t.Errorf("restart kettle: %d %q", code, errOut)
	}
	code, out, errOut = run("restart")
	if code != ExitOK || !strings.Contains(out, "Rewake: no plan: no manifest beside ") {
		t.Errorf("restart: %d %q %q", code, out, errOut)
	}
}

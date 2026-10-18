package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// peerSession is api, the session the test runs as, with web as a live peer.
func peerSession(t *testing.T) (string, registry.Session, registry.Session) {
	t.Helper()
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, self.Epoch())
	return dir, self, web
}

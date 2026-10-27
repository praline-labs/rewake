package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// interimLab is api after a turn that ended pending: web's task is read and
// owed, web has the interim message, and the next turn has started at start.
type interimLab struct {
	dir   string
	self  registry.Session
	start int64
}

// interimLab's times are on the boot clock below this test's own start; a
// turn the test ends afterwards ends now.
func newInterimLab(t *testing.T) interimLab {
	t.Helper()
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	readFrom(t, dir, peer)
	self, _ := registry.Lookup(dir, "api")
	if err := markPending(dir, "api", self.Epoch(), "the suite is running", markAt-8); err != nil {
		t.Fatal(err)
	}
	if err := completeTurn(dir, self, mail.TurnEnd{Text: "started the suite", Started: markAt - 10, Ended: markAt - 5}, "t"); err != nil {
		t.Fatal(err)
	}
	if got := kinds(reportsTo(t, dir, "web")); len(got) != 1 || got[0] != string(mail.Interim) {
		t.Fatalf("the first turn end gave web %v, want one interim message", got)
	}
	// The next turn starts as the interim one ends.
	turnStarted(t, dir, self, markAt-5)
	return interimLab{dir: dir, self: self, start: markAt - 5}
}

func (lab interimLab) owed() int { return len(mail.Open(lab.dir, "api", lab.self.Epoch()).Waiters()) }

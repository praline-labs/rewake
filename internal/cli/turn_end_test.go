package cli

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// boundaryEnd is a turn end as a harness's boundary hands it to the core:
// the text the turn ended on, whether it failed, whether the harness lets
// the end be held, and the conversation it ended in.
type boundaryEnd struct {
	text     string
	failed   bool
	holdable bool
	thread   string
}

// smokeEnd is the plain end most tests take: a turn that finished.
var smokeEnd = boundaryEnd{text: "the smoke is green"}

// completeTurn ends a turn that may not be held, as an adapter's boundary
// that cannot hold one reports it.
func completeTurn(dir string, self registry.Session, event mail.TurnEnd, currentThread string) error {
	return CompleteTurn(context.Background(), dir, self, event, currentThread, TurnEndOptions{BeforeReports: beforeReports})
}

// endTurn ends the turn of the session this test's environment names, as
// the wrapper does with the end its adapter's boundary reports: it ends now,
// it started at the latest start on record. It answers the reason a held end
// is continued with, "" when the end was published. A run that does not own
// its name has nothing to report, and ends nothing.
func endTurn(t *testing.T, end boundaryEnd) (string, error) {
	t.Helper()
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	self, _, err := ownRun(dir)
	if err != nil {
		return "", nil
	}
	event := mail.TurnEnd{Text: end.text, Failed: end.failed, Ended: boottime.Now()}
	event.Started, _ = mail.LatestTurnStart(dir, self.Name, self.Epoch())
	return EndTurn(context.Background(), dir, self, event, end.holdable, end.thread, TurnEndOptions{BeforeReports: beforeReports})
}

// mustEndTurn ends the turn as endTurn does, for a test that only needs the
// turn over: an end that fails is the test's failure, not something to skip.
func mustEndTurn(t *testing.T, end boundaryEnd) {
	t.Helper()
	if _, err := endTurn(t, end); err != nil {
		t.Fatal(err)
	}
}

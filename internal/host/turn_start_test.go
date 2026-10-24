package host

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// startBoundary is a run's TurnBoundary that keeps the handler it was given.
type startBoundary struct{ handler adapter.TurnHandler }

func (b *startBoundary) ReportTurns(handler adapter.TurnHandler) { b.handler = handler }

// A TurnBoundary reports its starts: a launch with a boundary records each
// timed start, the evidence `rewake pending` judges an
// earlier turn's mark by when that turn's end was lost
// (docs/rules/adapter-api.md#turnboundary). A start without a time records
// nothing.
func TestABoundaryRecordsItsStart(t *testing.T) {
	dir := stateDir(t)
	boundary := &startBoundary{}
	stop, err := startServing(context.Background(), Request{Dir: dir}, registry.Session{}, "api", "1.1",
		adapter.LaunchPlan{TurnBoundary: boundary}, newLiveSet(dir, "api", "1.1"), func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if boundary.handler.Started == nil {
		t.Fatal("the boundary was handed no Started")
	}
	boundary.handler.Started("thread", "turn-1", 123)
	boundary.handler.Started("thread", "turn-2", 0)
	// The read `rewake pending` makes (internal/cli/pending.go): a start
	// after a mark's time proves the mark's turn over.
	if got, err := mail.LatestTurnStart(dir, "api", "1.1"); got != 123 || err != nil {
		t.Fatalf("the latest start: %d %v, want 123", got, err)
	}
}

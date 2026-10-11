package wrap

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/registry"
)

// startBoundary is a run's TurnBoundary that keeps the handler it was given.
type startBoundary struct{ handler adapter.TurnHandler }

func (b *startBoundary) ReportTurns(handler adapter.TurnHandler) { b.handler = handler }

// A TurnBoundary reports its starts whatever else the run serves: a launch
// with a boundary and no mail tool — or one whose tool could not start —
// still records each timed start, the evidence `rewake pending` judges an
// earlier turn's mark by when that turn's end was lost
// (docs/v2/design-api.md#turnboundary). A start without a time records
// nothing.
func TestABoundaryWithoutAToolRecordsItsStart(t *testing.T) {
	dir := stateDir(t)
	boundary := &startBoundary{}
	stop, err := startServing(context.Background(), Request{Dir: dir}, registry.Session{}, "api", "1.1",
		adapter.LaunchPlan{TurnBoundary: boundary}, &mailTool{}, newLiveSet(dir, "api", "1.1"))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if boundary.handler.Tool != nil || boundary.handler.Started == nil {
		t.Fatalf("a run without a tool: Tool %v, Started set %t", boundary.handler.Tool, boundary.handler.Started != nil)
	}
	boundary.handler.Started("thread", "turn-1", 123)
	boundary.handler.Started("thread", "turn-2", 0)
	// The read `rewake pending` makes (internal/cli/pending.go): a start
	// after a mark's time proves the mark's turn over.
	if got, err := inbox.LatestTurnStart(dir, "api", "1.1"); got != 123 || err != nil {
		t.Fatalf("the latest start: %d %v, want 123", got, err)
	}
}

// With a tool, the same start is recorded once — by the host, the endpoint
// recording none — and opens its turn in the endpoint's table, so a call of
// that turn passes the check a call of a turn never started fails.
func TestABoundaryWithAToolRecordsItsStartOnceAndOpensItsTurn(t *testing.T) {
	tool := carrierTool(t)
	backend := &offeringBackend{}
	tool.attach(adapter.LaunchPlan{Service: backend})
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	backend.serve(os.Getpid(), start)

	var recorded []int64
	handler := tool.handler(func(at int64) { recorded = append(recorded, at) }, nil, nil)
	if handler.Tool == nil || handler.EndCapture == nil {
		t.Fatal("a run with a tool hands the boundary no tool or no gate")
	}
	handler.Started("A", "t1", 500)
	if !slices.Equal(recorded, []int64{500}) {
		t.Fatalf("recorded %v, want the one start", recorded)
	}

	// An end noted after t1 started: a call of t1, open in the table, is
	// then refused at the gate, after the table's check; a call of t2, never
	// started, at the table. Neither runs a child.
	handler.EndCapture()
	words := []string{"whoami"}
	for turn, refusal := range map[string]string{
		"t1": "ended before its ticket was issued",
		"t2": "never seen to start",
	} {
		callID := "c-" + turn
		handler.Tool.CallSeen(adapter.ObservedCall{ID: callID, Conversation: "A", Turn: turn, Words: words})
		answer, err := endpoint.CallTool(tool.path, endpoint.ToolCall{Tool: "whoami", CallID: callID, Conversation: "A", Turn: turn}, 5*time.Second)
		if err != nil || !answer.IsError || !strings.Contains(strings.Join(answer.Texts, "\n"), refusal) {
			t.Fatalf("a call of %s: %+v %v, want %q", turn, answer, err, refusal)
		}
	}
}

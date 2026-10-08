package cli

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// A stopped end and an error end of the same turn, each published twice and
// both observed before a later read, settle the scope they were observed with
// once: two reports, and the later letter still owed.
func TestStoppedAndErrorRetriesSettleOriginalScopeOnce(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self, _ := registry.Lookup(dir, "api")
	first := readFrom(t, dir, peer)
	clock, err := inbox.OpenReadClock(context.Background(), dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	stopped := harness.Completion{ID: "A/T", Thread: "A", Kind: inbox.Stopped, Text: "stopped", Boundary: clock.Snapshot()}
	final := stopped
	final.Kind = inbox.Error
	final.Text = "final error"
	// Both events were observed before the second message, but publication is delayed.
	second := readFrom(t, dir, peer)
	for _, event := range []harness.Completion{stopped, stopped, final, final} {
		if err := ReportCompletion(context.Background(), dir, self, event); err != nil {
			t.Fatal(err)
		}
	}
	reports := finishedFor(t, dir, peer.Name)
	waiters := inbox.Waiters(dir, self.Name, self.Epoch())
	if len(reports) != 2 || len(waiters) != 1 || len(waiters[0].Messages) != 1 || waiters[0].Messages[0] != second {
		t.Fatalf("first=%s later=%s reports=%v remaining=%v", first, second, reports, waiters)
	}
}

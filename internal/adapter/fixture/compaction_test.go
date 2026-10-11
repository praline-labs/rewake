//go:build rewakefixture

package fixture

import (
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// A compaction's end is counted once done, and the outcome of one a main asked
// for is kept for that main's letter; a refusal leaves an outcome and no count.
func TestACompactionsEndIsCountedAndItsOutcomeKept(t *testing.T) {
	b, _ := paired(t, adapter.TurnHandler{}, Served...)
	l := currentLink(b)
	before, after := int64(100), int64(10)
	b.compacted(l, Frame{Request: "r1", From: "lead", Outcome: "done", TokensBefore: &before, TokensAfter: &after})
	b.compacted(l, Frame{Request: "r2", From: "lead", Outcome: "refused", Reason: "in a turn"})
	b.compacted(l, Frame{Outcome: "done"})
	state := b.SessionState()
	if state.Compactions == nil || *state.Compactions != 2 || state.Coverage != "observed" {
		t.Fatalf("compactions %v, coverage %q; want 2 observed", state.Compactions, state.Coverage)
	}
	if len(state.CompactionEvents) != 2 || state.CompactionEvents[0].Request != "r1" || state.CompactionEvents[0].RequestedBy != "lead" ||
		state.CompactionEvents[1].Sequence != 2 || state.CompactionEvents[1].Request != "" {
		t.Fatalf("events %+v", state.CompactionEvents)
	}
	if len(state.CompactionOutcomes) != 2 || state.CompactionOutcomes[0].Outcome != "done" || *state.CompactionOutcomes[0].TokensAfter != 10 ||
		state.CompactionOutcomes[1].Reason != "in a turn" {
		t.Fatalf("outcomes %+v", state.CompactionOutcomes)
	}
}

// Without a live Control nothing is counted, and no count is published: zero
// would claim a coverage the program has not shown.
func TestCompactionsNeedALiveControl(t *testing.T) {
	b, _ := paired(t, adapter.TurnHandler{}, Wake, Telemetry)
	b.compacted(currentLink(b), Frame{Request: "r1", From: "lead", Outcome: "done"})
	b.mu.Lock()
	counted := b.compact.count
	b.mu.Unlock()
	if state := b.SessionState(); counted != 0 || state.Compactions != nil || len(state.CompactionOutcomes) != 0 {
		t.Fatalf("counted %d; published %v, %+v", counted, state.Compactions, state.CompactionOutcomes)
	}
}

// A new conversation the program reports is the one delivered to from then
// on, while Wake is live on the connection it came on.
func TestANewConversationIsTakenWhileWakeIsLive(t *testing.T) {
	b, _ := paired(t, adapter.TurnHandler{}, Served...)
	b.threadChanged(currentLink(b), Frame{Thread: "next"})
	if thread, err := b.Thread(); err != nil || thread != "next" {
		t.Fatalf("thread %q, %v", thread, err)
	}
	old := currentLink(b)
	b.mu.Lock()
	b.link = newLink(nil)
	b.mu.Unlock()
	b.threadChanged(old, Frame{Thread: "stale"})
	if thread, _ := b.Thread(); thread != "next" {
		t.Fatalf("a withdrawn connection changed the conversation to %q", thread)
	}
}

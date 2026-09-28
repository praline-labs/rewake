package telemetry

import (
	"slices"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/sessionstate"
)

type folding struct {
	state state
	clock int64
}

// send folds an event stamped with the next tick of the sender's clock.
func (f *folding) send(event Event) {
	f.clock++
	event.At = f.clock
	f.state.apply(event, time.Unix(0, f.clock))
}

func status(model, effort string, used *int64) Event {
	window := int64(200000)
	context := &Context{Window: &window}
	if used != nil {
		percent := int(*used * 100 / window)
		context.Used, context.Percent = used, &percent
	}
	return Event{Kind: StatusLine, Session: "s1", Model: model, Effort: effort, Context: context}
}

func count(value int64) *int64 { return &value }

// Nothing heard is unknown everywhere, including compactions: zero would be a
// count nobody made.
func TestNothingHeardIsUnknown(t *testing.T) {
	var f folding
	snapshot := f.state.snapshot()
	if snapshot.Selection != "unknown" || snapshot.Coverage != "unknown" || snapshot.Activity != nil ||
		snapshot.Compactions != nil || snapshot.Model != nil || snapshot.ContextUsed != nil || snapshot.Fresh {
		t.Errorf("snapshot = %+v, want unknown", snapshot)
	}
}

// The status line alone does not prove the hooks run, so compactions stay
// unknown until one does.
func TestStatusAloneLeavesCompactionsUnknown(t *testing.T) {
	var f folding
	f.send(status("m", "high", nil))
	snapshot := f.state.snapshot()
	if snapshot.Compactions != nil || snapshot.Coverage != "unknown" {
		t.Errorf("compactions = %v coverage %q, want unknown", snapshot.Compactions, snapshot.Coverage)
	}
	if *snapshot.Model != "m" || *snapshot.Effort != "high" || snapshot.ContextUsed != nil || *snapshot.ContextWindow != 200000 {
		t.Errorf("snapshot = %+v, want the model, effort and window, no fill", snapshot)
	}
	if snapshot.Activity != nil {
		t.Errorf("activity = %v, want unknown", *snapshot.Activity)
	}
}

// A whole session: start, a turn, a compaction inside it, the end.
func TestASessionFoldsIntoTheSnapshot(t *testing.T) {
	var f folding
	f.send(Event{Kind: SessionStart, Source: "startup", Model: "m", Session: "s1"})
	snapshot := f.state.snapshot()
	if *snapshot.Activity != "idle" || snapshot.WaitingFor == nil || len(snapshot.WaitingFor) != 0 {
		t.Errorf("after start: activity %v waiting %v, want idle and not waiting", *snapshot.Activity, snapshot.WaitingFor)
	}
	if *snapshot.Compactions != 0 || snapshot.Coverage != "observed" || *snapshot.Compacting {
		t.Errorf("after start: compactions %v coverage %q, want an observed zero", *snapshot.Compactions, snapshot.Coverage)
	}
	if *snapshot.Model != "m" || snapshot.Effort != nil || snapshot.Thread != "s1" {
		t.Errorf("after start: %+v", snapshot)
	}

	f.send(Event{Kind: UserPromptSubmit, Effort: "high"})
	snapshot = f.state.snapshot()
	if *snapshot.Activity != "working" || snapshot.WaitingFor != nil || *snapshot.Effort != "high" {
		t.Errorf("in a turn: activity %v waiting %v effort %v", *snapshot.Activity, snapshot.WaitingFor, snapshot.Effort)
	}

	f.send(status("m", "high", count(34727)))
	f.send(Event{Kind: PreCompact, Trigger: "auto"})
	if snapshot = f.state.snapshot(); !*snapshot.Compacting {
		t.Error("PreCompact did not mark a compaction in progress")
	}
	f.send(Event{Kind: SessionStart, Source: "compact"})
	if snapshot = f.state.snapshot(); *snapshot.Activity != "working" {
		t.Errorf("a compaction's SessionStart ended the turn: %v", *snapshot.Activity)
	}
	f.send(Event{Kind: PostCompact, Trigger: "auto"})
	f.send(status("m", "high", nil))
	snapshot = f.state.snapshot()
	if *snapshot.Compactions != 1 || *snapshot.Compacting || len(snapshot.CompactionEvents) != 1 || snapshot.CompactionEvents[0].Sequence != 1 {
		t.Errorf("after compaction: %v compacting %v events %v", *snapshot.Compactions, *snapshot.Compacting, snapshot.CompactionEvents)
	}
	if snapshot.ContextUsed != nil || snapshot.FilledPercent != nil || *snapshot.ContextWindow != 200000 {
		t.Errorf("after compaction the fill should be unknown again: %+v", snapshot)
	}

	f.send(Event{Kind: Stop})
	snapshot = f.state.snapshot()
	if *snapshot.Activity != "idle" || snapshot.ActivityAt == nil || !snapshot.ActivityFresh || !snapshot.Fresh {
		t.Errorf("after the turn: %+v", snapshot)
	}
}

// Sequence numbers only grow, and the cues are bounded like the Codex side's.
func TestCompactionSequenceIsMonotonicAndBounded(t *testing.T) {
	var f folding
	for range maxCompactionEvents + 5 {
		f.send(Event{Kind: PreCompact})
		f.send(Event{Kind: PostCompact})
	}
	snapshot := f.state.snapshot()
	if *snapshot.Compactions != maxCompactionEvents+5 || len(snapshot.CompactionEvents) != maxCompactionEvents {
		t.Fatalf("count %d events %d", *snapshot.Compactions, len(snapshot.CompactionEvents))
	}
	if !slices.IsSortedFunc(snapshot.CompactionEvents, func(a, b sessionstate.CompactionEvent) int { return int(a.Sequence) - int(b.Sequence) }) ||
		snapshot.CompactionEvents[len(snapshot.CompactionEvents)-1].Sequence != maxCompactionEvents+5 {
		t.Errorf("events = %v, want the latest, in order", snapshot.CompactionEvents)
	}
}

// A failed compaction runs PreCompact and nothing after; the end of the turn
// clears it.
func TestAFailedCompactionEndsWithTheTurn(t *testing.T) {
	var f folding
	f.send(Event{Kind: UserPromptSubmit})
	f.send(Event{Kind: PreCompact})
	f.send(Event{Kind: StopFailure})
	if snapshot := f.state.snapshot(); *snapshot.Compacting || *snapshot.Compactions != 0 {
		t.Errorf("compacting %v count %v, want neither", *snapshot.Compacting, *snapshot.Compactions)
	}
}

// Hooks may run in the background and arrive out of order: an older event
// does not undo a newer one.
func TestALateEventDoesNotOverwriteANewerOne(t *testing.T) {
	var s state
	s.apply(Event{At: 10, Kind: Stop}, time.Unix(0, 10))
	s.apply(Event{At: 5, Kind: UserPromptSubmit}, time.Unix(0, 11))
	if snapshot := s.snapshot(); *snapshot.Activity != "idle" {
		t.Errorf("activity = %v, want the newer idle", *snapshot.Activity)
	}
	s.apply(Event{At: 20, Kind: StatusLine, Model: "new"}, time.Unix(0, 20))
	s.apply(Event{At: 15, Kind: StatusLine, Model: "old"}, time.Unix(0, 21))
	if snapshot := s.snapshot(); *snapshot.Model != "new" {
		t.Errorf("model = %v, want the newer one", *snapshot.Model)
	}
}

// A model without an effort clears the previous model's effort: the status
// line leaves the key out rather than keeping it.
func TestAModelWithoutEffortClearsIt(t *testing.T) {
	var f folding
	f.send(status("model-a", "high", nil))
	f.send(status("model-b", "", nil))
	if snapshot := f.state.snapshot(); *snapshot.Model != "model-b" || snapshot.Effort != nil {
		t.Errorf("model %v effort %v, want model-b and unknown", *snapshot.Model, snapshot.Effort)
	}
}

// A permission prompt during a turn is waiting for approval, until the count
// moves — which happens only after a response, so after the answer.
func TestAPermissionPromptWaitsUntilTheTurnMoves(t *testing.T) {
	var f folding
	f.send(Event{Kind: Notification, Notice: "permission_prompt"})
	if snapshot := f.state.snapshot(); snapshot.WaitingFor != nil {
		t.Errorf("waiting %v outside a turn, want unknown", snapshot.WaitingFor)
	}
	f.send(Event{Kind: UserPromptSubmit})
	f.send(status("m", "", count(10)))
	f.send(Event{Kind: Notification, Notice: "permission_prompt"})
	if snapshot := f.state.snapshot(); !slices.Equal(snapshot.WaitingFor, []string{"approval"}) {
		t.Errorf("waiting = %v, want approval", snapshot.WaitingFor)
	}
	f.send(status("m", "", count(10)))
	if snapshot := f.state.snapshot(); !slices.Equal(snapshot.WaitingFor, []string{"approval"}) {
		t.Errorf("an unchanged count cleared the wait: %v", snapshot.WaitingFor)
	}
	f.send(status("m", "", count(20)))
	if snapshot := f.state.snapshot(); snapshot.WaitingFor != nil {
		t.Errorf("waiting = %v after the count moved, want unknown again", snapshot.WaitingFor)
	}
}

// /clear starts a new conversation, and the next event carries it.
func TestANewConversationIsFollowed(t *testing.T) {
	var f folding
	f.send(Event{Kind: SessionStart, Source: "startup", Session: "first"})
	f.send(Event{Kind: SessionStart, Source: "clear", Session: "second"})
	if snapshot := f.state.snapshot(); snapshot.Thread != "second" {
		t.Errorf("thread = %q, want the conversation after /clear", snapshot.Thread)
	}
}

// A kind nobody knows is not a hook that ran.
func TestAnUnknownKindSaysNothing(t *testing.T) {
	var f folding
	f.send(Event{Kind: "SomethingNew", Session: "s"})
	if snapshot := f.state.snapshot(); snapshot.Selection != "unknown" || snapshot.Thread != "" {
		t.Errorf("snapshot = %+v, want unknown", snapshot)
	}
}

// A compaction leaves the fill unknown, as before the first response, until a
// response is counted again: the harness reports zeros meanwhile, and the old
// value no longer holds either. A status line started before the compaction
// ended and arriving after it changes nothing; the window stays known.
func TestACompactionLeavesTheFillUnknownUntilTheNextResponse(t *testing.T) {
	var f folding
	f.send(Event{Kind: SessionStart, Source: "startup"})
	late := status("m", "", count(120000))
	late.At = f.clock + 1
	f.send(status("m", "", count(120000)))
	f.send(Event{Kind: PreCompact, Trigger: "manual"})
	f.send(Event{Kind: PostCompact, Trigger: "manual"})
	fill := func() (*int, *int64) {
		snapshot := f.state.snapshot()
		return snapshot.FilledPercent, snapshot.ContextWindow
	}
	if percent, window := fill(); percent != nil || window == nil || *window != 200000 {
		t.Fatalf("after the compaction: percent %v, window %v", percent, window)
	}
	f.state.apply(late, time.Unix(0, f.clock))
	if percent, _ := fill(); percent != nil {
		t.Fatalf("a status line from before the compaction brought back %d%%", *percent)
	}
	f.send(Event{Kind: SessionMeasure, Context: &Context{Used: count(120000)}})
	if percent, _ := fill(); percent != nil {
		t.Fatalf("the plugin's measure filled in %d%% while the status line runs", *percent)
	}
	f.send(status("m", "", count(20000)))
	if percent, _ := fill(); percent == nil || *percent != 10 {
		t.Fatalf("after the next response: percent %v, want 10", percent)
	}
}

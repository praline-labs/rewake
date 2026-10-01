package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// The plugin's events decode to exactly the fields the collector uses, and
// anything else — a hook's payload, an event nobody sends, a turn end with no
// reason — is not taken for one.
func TestThePluginsEventsDecode(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      Event
	}{
		{"ready", `{"plugin_event":"plugin.ready"}`, Event{Kind: PluginReady}},
		{"a turn start", `{"plugin_event":"turn.start","turn_id":"t1"}`, Event{Kind: TurnStart, Turn: "t1"}},
		{"an interrupted turn", `{"plugin_event":"turn.complete","turn_id":"t1","reason":"aborted"}`, Event{Kind: TurnComplete, Turn: "t1", Reason: ReasonAborted}},
		{"an answered turn", `{"plugin_event":"turn.complete","turn_id":"t2","reason":"answer"}`, Event{Kind: TurnComplete, Turn: "t2", Reason: "answer"}},
		// Text the plugin never sends is not read even when it is there.
		{"a turn end carrying text", `{"plugin_event":"turn.complete","reason":"answer","text":"the reply","answer":"the reply"}`, Event{Kind: TurnComplete, Reason: "answer"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DecodePlugin([]byte(tc.raw))
			if !ok || got.Kind != tc.want.Kind || got.Turn != tc.want.Turn || got.Reason != tc.want.Reason || got.Context != nil {
				t.Errorf("DecodePlugin(%s) = %+v, %v; want %+v", tc.raw, got, ok, tc.want)
			}
		})
	}
	for _, raw := range []string{
		`{"hook_event_name":"Stop","last_assistant_message":"x"}`,
		`{"plugin_event":"turn.step"}`,
		`{"plugin_event":"turn.complete","turn_id":"t1"}`,
		`{"plugin_event":"session.measure"}`,
		`{"plugin_event":"session.measure","context":{}}`,
		`not json`,
	} {
		if event, ok := DecodePlugin([]byte(raw)); ok {
			t.Errorf("DecodePlugin(%s) = %+v, want refused", raw, event)
		}
	}
}

// session.measure carries the context the harness measured; a percentage is
// rounded, and a value no count can be is dropped rather than trusted.
func TestAMeasureDecodesItsContext(t *testing.T) {
	event, ok := DecodePlugin([]byte(`{"plugin_event":"session.measure","context":{"tokens":52000,"window":200000,"percent":26.4}}`))
	if !ok || event.Context == nil || *event.Context.Used != 52000 || *event.Context.Window != 200000 || *event.Context.Percent != 26 {
		t.Fatalf("measure = %+v, %v", event.Context, ok)
	}
	event, ok = DecodePlugin([]byte(`{"plugin_event":"session.measure","context":{"window":200000}}`))
	if !ok || event.Context.Used != nil || event.Context.Percent != nil || *event.Context.Window != 200000 {
		t.Fatalf("a measure before the first response = %+v, %v", event.Context, ok)
	}
	event, ok = DecodePlugin([]byte(`{"plugin_event":"session.measure","context":{"tokens":0,"window":200000,"percent":0}}`))
	if !ok || event.Context.Used != nil || event.Context.Percent != nil {
		t.Fatalf("a measure of zero tokens = %+v, %v; want the window alone", event.Context, ok)
	}
	event, ok = DecodePlugin([]byte(`{"plugin_event":"session.measure","context":{"tokens":-1,"window":1.5,"percent":140}}`))
	if ok {
		t.Fatalf("a measure of nothing a count can be = %+v", event.Context)
	}
}

// An interrupted turn ends the activity it started: the plugin's turn end is
// what the hooks never said after an Esc.
func TestAnInterruptedTurnLeavesTheSessionIdle(t *testing.T) {
	var f folding
	f.send(Event{Kind: UserPromptSubmit})
	f.send(Event{Kind: TurnStart, Turn: "t1"})
	if snapshot := f.state.snapshot(); *snapshot.Activity != "working" {
		t.Fatalf("activity %q during the turn", *snapshot.Activity)
	}
	f.send(Event{Kind: TurnComplete, Turn: "t1", Reason: ReasonAborted})
	if snapshot := f.state.snapshot(); *snapshot.Activity != "idle" {
		t.Fatalf("activity %q after the interruption, want idle", *snapshot.Activity)
	}
	// A late turn start of the same turn does not make it work again.
	f.state.apply(Event{Kind: TurnStart, Turn: "t1", At: 1}, f.state.observedAt)
	if snapshot := f.state.snapshot(); *snapshot.Activity != "idle" {
		t.Fatalf("a late turn start set activity %q", *snapshot.Activity)
	}
}

// The status line says what the context is whenever it runs; the plugin's
// measure fills in only where it has said nothing.
func TestAMeasureFillsInOnlyForASilentStatusLine(t *testing.T) {
	var f folding
	f.send(Event{Kind: SessionMeasure, Context: &Context{Used: count(1000), Window: count(10000), Percent: intPointer(10)}})
	if snapshot := f.state.snapshot(); snapshot.ContextUsed == nil || *snapshot.ContextUsed != 1000 || *snapshot.FilledPercent != 10 {
		t.Fatalf("the measure alone gave %+v", snapshot)
	}
	f.send(status("m", "high", count(50000)))
	f.send(Event{Kind: SessionMeasure, Context: &Context{Used: count(2000), Window: count(10000), Percent: intPointer(20)}})
	if snapshot := f.state.snapshot(); *snapshot.ContextUsed != 50000 {
		t.Fatalf("the measure overrode the status line: %d", *snapshot.ContextUsed)
	}
}

// Whether an interruption is heard is unknown until a turn is heard; then it
// is observed if the plugin spoke, unobserved if not. The plugin's events do
// not stand in for the hooks' compaction count.
func TestTheSnapshotSaysWhetherInterruptionsAreHeard(t *testing.T) {
	var without folding
	without.send(status("m", "high", nil))
	if got := without.state.snapshot().Interruptions; got != "" {
		t.Fatalf("before any turn: %q", got)
	}
	without.send(Event{Kind: UserPromptSubmit})
	if got := without.state.snapshot().Interruptions; got != sessionstate.InterruptionsUnobserved {
		t.Fatalf("a turn with no plugin: %q", got)
	}
	var with folding
	with.send(Event{Kind: PluginReady})
	snapshot := with.state.snapshot()
	if snapshot.Interruptions != sessionstate.InterruptionsObserved || snapshot.Compactions != nil {
		t.Fatalf("the plugin alone: %q, compactions %v", snapshot.Interruptions, snapshot.Compactions)
	}
	with.send(Event{Kind: Stop})
	if got := with.state.snapshot().Interruptions; got != sessionstate.InterruptionsObserved {
		t.Fatalf("a turn with the plugin: %q", got)
	}
}

func intPointer(value int) *int { return &value }

// published collects what the collector hands the wrapper.
type published struct {
	mu   sync.Mutex
	seen []harness.Completion
}

func (p *published) handler() harness.CompletionHandler {
	return harness.CompletionHandler{
		Capture: func() *inbox.ReadBoundary { return &inbox.ReadBoundary{Through: 7} },
		Publish: func(_ context.Context, completion harness.Completion) error {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.seen = append(p.seen, completion)
			return nil
		},
	}
}

func (p *published) all() []harness.Completion {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]harness.Completion{}, p.seen...)
}

// An interrupted turn is published as stopped, bounded by what was read when
// it was heard, and its end is recorded as the next turn's earliest start. A
// turn that ended any other way publishes nothing here: its Stop or
// StopFailure hook reports it, and a second report would count it twice.
func TestAnInterruptedTurnIsPublishedAsStopped(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	collector := NewCollector(path)
	var out published
	collector.ReportTurns(out.handler())
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	RecordTurnStart(path, 100)
	Send(path, Event{Kind: SessionStart, Session: "conversation-1", At: 50})
	Send(path, Event{Kind: TurnComplete, Turn: "t1", Reason: "answer", At: 150})
	Send(path, Event{Kind: TurnComplete, Turn: "t2", Reason: ReasonAborted, At: 200})
	waitFor(t, "the stopped outcome", func() bool { return len(out.all()) == 1 })
	got := out.all()[0]
	if got.Kind != inbox.Stopped || got.Text != StoppedText || got.ID != "claude/t2" || got.Thread != "conversation-1" ||
		got.Started != 100 || got.Ended != 200 || got.Boundary == nil || got.Boundary.Through != 7 {
		t.Fatalf("published %+v", got)
	}
	waitFor(t, "the turn's end recorded", func() bool { return ReadTurnStart(TurnStartPath(path)) == 200 })
	if snapshot := collector.SessionState(); *snapshot.Activity != "idle" {
		t.Errorf("activity %q after the interruption", *snapshot.Activity)
	}
}

// Without a handler an interruption still sets activity and publishes
// nothing, and Close waits for nothing.
func TestWithoutAHandlerNothingIsPublished(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	collector := NewCollector(path)
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	Send(path, Event{Kind: TurnStart, Turn: "t1", At: 1})
	Send(path, Event{Kind: TurnComplete, Turn: "t1", Reason: ReasonAborted, At: 2})
	waitFor(t, "the interruption", func() bool {
		snapshot := collector.SessionState()
		return snapshot.Activity != nil && *snapshot.Activity == "idle"
	})
	collector.Close()
}

// The plugin directory the launch wrote goes with the collector even when the
// collector never started: the launch wrote it all the same.
func TestCloseRemovesThePluginOfACollectorThatNeverStarted(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	if err := os.MkdirAll(filepath.Join(PluginPath(path), "hooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	NewCollector(path).Close()
	if _, err := os.Stat(PluginPath(path)); !os.IsNotExist(err) {
		t.Fatalf("the plugin directory survived Close: %v", err)
	}
}

// With the mail tool's gate an interruption's boundary is taken through it,
// not through the plain capture: an acknowledgment writing at that moment
// lands on one side of the end (docs/mail-bridge-turns.md#a-turns-end-meets-its-calls).
func TestAnInterruptionIsCapturedThroughTheGate(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	collector := NewCollector(path)
	var out published
	handler := out.handler()
	gated := 0
	handler.EndCapture = func() (*inbox.ReadBoundary, int64) {
		gated++
		return &inbox.ReadBoundary{Through: 9}, 4242
	}
	collector.ReportTurns(handler)
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	Send(path, Event{Kind: TurnStart, Turn: "t1", At: 1})
	Send(path, Event{Kind: TurnComplete, Turn: "t1", Reason: ReasonAborted, At: 2})
	waitFor(t, "the stopped outcome", func() bool { return len(out.all()) == 1 })
	if got := out.all()[0]; got.Boundary == nil || got.Boundary.Through != 9 || gated != 1 {
		t.Fatalf("published %+v after %d captures through the gate", got, gated)
	}
}

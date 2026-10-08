package wrap

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// runningDetail is what a worker's wrapper may say when its wait for the end
// ran out with the compaction still running.
const runningDetail = "the compaction was not seen to end within 10m0s; it may still be running, and its end is reported when seen"

// A worker that stopped waiting with the compaction still running says so as
// an outcome of started, which is no outcome yet: main gets no letter for it,
// and gets the compaction's end, with the tokens, when the worker records it.
func TestACompactionStillRunningIsReportedByItsEnd(t *testing.T) {
	dir, main, peer, observer := lettersFixture(t)
	now := time.Now()
	running := sessionstate.CompactionOutcome{Request: letterA, RequestedBy: main.Name, Outcome: control.Started, Detail: runningDetail, EndedAt: now}
	letterState(t, dir, peer, nil, []sessionstate.CompactionOutcome{running})
	pend(t, dir, main, peer, letterA)
	scanTimes(t, observer, dir, main, 2)
	time.Sleep(letterWait)
	scanTimes(t, observer, dir, main, 2)
	if got := letters(t, dir, main.Name); len(got) != 0 {
		t.Fatalf("a letter while the compaction may still run: %+v", got)
	}
	before, after := int64(120000), int64(9000)
	letterState(t, dir, peer,
		[]sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: now, RequestedBy: main.Name, Request: letterA}},
		[]sessionstate.CompactionOutcome{running, {Request: letterA, RequestedBy: main.Name, Outcome: control.Done, TokensBefore: &before, TokensAfter: &after, EndedAt: now}})
	scanTimes(t, observer, dir, main, 2)
	want := "Rewake: compacted worker-fixture: 120000 tokens before, 9000 after (compaction 1)."
	if got := letters(t, dir, main.Name); len(got) != 1 || got[0].Text != want {
		t.Fatalf("letters %+v, want one saying %q", got, want)
	}
}

// A compaction still running whose end never comes gets its letter at the
// bound, saying what the worker said.
func TestACompactionStillRunningAtTheBoundSaysSo(t *testing.T) {
	saved := letterBound
	letterBound = 100 * time.Millisecond
	t.Cleanup(func() { letterBound = saved })
	dir, main, peer, observer := lettersFixture(t)
	letterState(t, dir, peer, nil, []sessionstate.CompactionOutcome{{Request: letterA, RequestedBy: main.Name, Outcome: control.Started, Detail: runningDetail, EndedAt: time.Now()}})
	pend(t, dir, main, peer, letterA)
	time.Sleep(letterBound)
	scanTimes(t, observer, dir, main, 2)
	want := "Rewake: the compaction of worker-fixture you asked for had no outcome within 100ms (" + runningDetail + "); rewake list shows whether it compacted."
	if got := letters(t, dir, main.Name); len(got) != 1 || got[0].Text != want {
		t.Fatalf("letters %+v, want one saying %q", got, want)
	}
}

// A worker may record a started after the compaction's end: its command
// publishes the answer after the end was recorded. The final outcome stands
// over it in either order, and main's letter comes from it at once.
func TestAStartedRecordedAfterTheEndDoesNotHideIt(t *testing.T) {
	now := time.Now()
	before, after := int64(120000), int64(9000)
	for _, c := range []struct {
		name   string
		final  sessionstate.CompactionOutcome
		events []sessionstate.CompactionEvent
		want   string
	}{
		{
			name:  "failed",
			final: sessionstate.CompactionOutcome{Request: letterA, Outcome: control.Failed, Detail: "the compaction was interrupted", EndedAt: now},
			want:  "Rewake: the compaction of worker-fixture you asked for failed (the compaction was interrupted).",
		},
		{
			name:   "done",
			final:  sessionstate.CompactionOutcome{Request: letterA, Outcome: control.Done, TokensBefore: &before, TokensAfter: &after, EndedAt: now},
			events: []sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: now, Request: letterA}},
			want:   "Rewake: compacted worker-fixture: 120000 tokens before, 9000 after (compaction 1).",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, main, peer, observer := lettersFixture(t)
			c.final.RequestedBy = main.Name
			for i := range c.events {
				c.events[i].RequestedBy = main.Name
			}
			running := sessionstate.CompactionOutcome{Request: letterA, RequestedBy: main.Name, Outcome: control.Started, Detail: runningDetail, EndedAt: now.Add(time.Millisecond)}
			letterState(t, dir, peer, c.events, []sessionstate.CompactionOutcome{c.final, running})
			pend(t, dir, main, peer, letterA)
			scanTimes(t, observer, dir, main, 2)
			if got := letters(t, dir, main.Name); len(got) != 1 || got[0].Text != c.want {
				t.Fatalf("letters %+v, want one saying %q", got, c.want)
			}
		})
	}
}

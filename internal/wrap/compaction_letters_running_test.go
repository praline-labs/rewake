package wrap

import (
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

// runningDetail is what a Codex worker says when its wait for the end ran out
// with the compaction still running.
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

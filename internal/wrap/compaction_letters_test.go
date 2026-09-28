package wrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

const (
	letterA = "0123456789abcdef0123456789abcdef"
	letterB = "fedcba9876543210fedcba9876543210"
)

// letterState saves a worker's snapshot with compactions and their outcomes.
func letterState(t *testing.T, dir string, peer registry.Session, events []sessionstate.CompactionEvent, outcomes []sessionstate.CompactionOutcome) {
	t.Helper()
	noticeState(t, dir, peer, "idle", uint64(len(events)), events)
	snapshot := sessionstate.Load(dir, peer.Name, peer.Epoch())
	snapshot.CompactionOutcomes = outcomes
	if err := sessionstate.Save(dir, peer.Name, peer.Epoch(), snapshot); err != nil {
		t.Fatal(err)
	}
}

// letters answers the letters main holds, other than availability notices.
func letters(t *testing.T, dir, name string) []inbox.Message {
	t.Helper()
	var found []inbox.Message
	for _, m := range availabilityFiles(t, dir, name) {
		if m.Availability == nil {
			found = append(found, m)
		}
	}
	return found
}

// pend leaves the record `rewake compact` leaves when it answers started
// or requested.
func pend(t *testing.T, dir string, main, peer registry.Session, id string) {
	t.Helper()
	pendAt(t, dir, main, peer, id, main.Epoch(), time.Now())
}

func pendAt(t *testing.T, dir string, main, peer registry.Session, id, epoch string, at time.Time) {
	t.Helper()
	release, err := control.Remember(dir, main.Name, control.Pending{ID: id, Worker: peer, AskerEpoch: epoch, AskedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func lettersFixture(t *testing.T) (string, registry.Session, registry.Session, *sessionNotices) {
	t.Helper()
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	return dir, main, peer, observer
}

func scanTimes(t *testing.T, observer *sessionNotices, dir string, main registry.Session, times int) {
	t.Helper()
	for range times {
		if err := observer.scan(context.Background(), dir, main); err != nil {
			t.Fatal(err)
		}
	}
}

// A compaction main asked for ends in one letter from the worker with the
// token counts and the count of compactions — what the command answered and
// the notice carried before — which owes nothing, and in no notice beside it.
func TestACompactionMainAskedForEndsInOneLetter(t *testing.T) {
	dir, main, peer, observer := lettersFixture(t)
	now := time.Now()
	before, after := int64(120000), int64(9000)
	letterState(t, dir, peer,
		[]sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: now, RequestedBy: main.Name, Request: letterA}},
		[]sessionstate.CompactionOutcome{{Request: letterA, RequestedBy: main.Name, Outcome: control.Done, TokensBefore: &before, TokensAfter: &after, EndedAt: now}})
	pend(t, dir, main, peer, letterA)
	scanTimes(t, observer, dir, main, 3)
	got := letters(t, dir, main.Name)
	want := "Rewake: compacted worker-fixture: 120000 tokens before, 9000 after (compaction 1)."
	if len(got) != 1 || got[0].Text != want || got[0].Kind != inbox.Note || inbox.Owed(got[0]) || got[0].From != peer.Name ||
		got[0].FromEpoch != peer.Epoch() || got[0].ToEpoch != main.Epoch() || got[0].Compaction == nil || got[0].Compaction.Count != 1 {
		t.Fatalf("letters %+v, want one saying %q", got, want)
	}
	if pending := control.PendingOf(dir, main.Name); len(pending) != 0 {
		t.Fatalf("the record outlived its letter: %+v", pending)
	}
}

// A refusal or a failure after the command answered goes at once, with its
// reason; a compaction another main asked for is that main's letter, and
// this main's record of that request waits on.
func TestARefusedOrFailedCompactionIsALetterToo(t *testing.T) {
	dir, main, peer, observer := lettersFixture(t)
	now := time.Now()
	letterState(t, dir, peer, nil, []sessionstate.CompactionOutcome{
		{Request: letterA, RequestedBy: main.Name, Outcome: control.Refused, Reason: control.NothingToCompact, Detail: "Not enough messages to compact.", EndedAt: now},
		{Request: letterB, RequestedBy: main.Name, Outcome: control.Failed, Detail: "the connection ended during the compaction", EndedAt: now},
		{Request: "00000000000000000000000000000000", RequestedBy: "other-main", Outcome: control.Failed, EndedAt: now},
	})
	pend(t, dir, main, peer, letterA)
	pend(t, dir, main, peer, letterB)
	pend(t, dir, main, peer, "00000000000000000000000000000000")
	scanTimes(t, observer, dir, main, 2)
	var texts []string
	for _, m := range letters(t, dir, main.Name) {
		texts = append(texts, m.Text)
	}
	joined := strings.Join(texts, "\n")
	if len(texts) != 2 || len(control.PendingOf(dir, main.Name)) != 1 ||
		!strings.Contains(joined, "Rewake: worker-fixture refused the compaction you asked for: nothing to compact (Not enough messages to compact.). Nothing was compacted.") ||
		!strings.Contains(joined, "Rewake: the compaction of worker-fixture you asked for failed (the connection ended during the compaction).") {
		t.Fatalf("letters %q", texts)
	}
}

// Each half waits for the other a moment, then goes alone and says what is
// missing; a worker gone does not keep it waiting.
func TestALetterWaitsForItsOtherHalfOnlyAMoment(t *testing.T) {
	saved := letterWait
	letterWait = 100 * time.Millisecond
	t.Cleanup(func() { letterWait = saved })
	dir, main, peer, observer := lettersFixture(t)
	now := time.Now()
	letterState(t, dir, peer, nil, []sessionstate.CompactionOutcome{{Request: letterA, RequestedBy: main.Name, Outcome: control.Done, EndedAt: now}})
	pend(t, dir, main, peer, letterA)
	pend(t, dir, main, peer, letterB)
	scanTimes(t, observer, dir, main, 1)
	if got := letters(t, dir, main.Name); len(got) != 0 {
		t.Fatalf("sent before the count could come: %+v", got)
	}
	time.Sleep(letterWait)
	scanTimes(t, observer, dir, main, 2)
	got := letters(t, dir, main.Name)
	if len(got) != 1 || !strings.HasPrefix(got[0].Text, "Rewake: compacted worker-fixture; its telemetry has not counted it") {
		t.Fatalf("letters %+v", got)
	}
	letterWait = time.Hour
	letterState(t, dir, peer, []sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: now, RequestedBy: main.Name, Request: letterB}}, nil)
	scanTimes(t, observer, dir, main, 1)
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	scanTimes(t, observer, dir, main, 1)
	var count *inbox.Message
	for _, m := range letters(t, dir, main.Name) {
		if m.Compaction != nil && m.Departure == nil {
			count = &m
		}
	}
	if count == nil || count.Text != "Rewake: compacted worker-fixture (compaction 1)." {
		t.Fatalf("the count's letter did not go when the worker went: %+v", letters(t, dir, main.Name))
	}
}

// A worker that leaves with no word of the compaction — killed mid-way, its
// outcome lost — does not leave main waiting: after a moment in which its
// last snapshot may still be written, the letter says it failed. A worker that
// finished and left at once gets the letter of its outcome.
func TestALetterComesWhenTheWorkerLeavesWithoutAWord(t *testing.T) {
	saved := letterWait
	letterWait = 100 * time.Millisecond
	t.Cleanup(func() { letterWait = saved })
	dir, main, peer, observer := lettersFixture(t)
	pend(t, dir, main, peer, letterA)
	pend(t, dir, main, peer, letterB)
	scanTimes(t, observer, dir, main, 2)
	if got := letters(t, dir, main.Name); len(got) != 0 {
		t.Fatalf("a letter before any word: %+v", got)
	}
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	scanTimes(t, observer, dir, main, 1)
	compactions := func() []string {
		var texts []string
		for _, m := range letters(t, dir, main.Name) {
			if m.Departure == nil {
				texts = append(texts, m.Text)
			}
		}
		return texts
	}
	if got := compactions(); len(got) != 0 {
		t.Fatalf("a failed letter before the worker's last snapshot could come: %q", got)
	}
	now := time.Now()
	letterState(t, dir, peer, []sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: now, RequestedBy: main.Name, Request: letterB}},
		[]sessionstate.CompactionOutcome{{Request: letterB, RequestedBy: main.Name, Outcome: control.Done, EndedAt: now}})
	time.Sleep(letterWait)
	scanTimes(t, observer, dir, main, 2)
	got := strings.Join(compactions(), "\n")
	for _, want := range []string{
		"Rewake: the compaction of worker-fixture you asked for failed (worker-fixture left before the compaction ended: not registered).",
		"Rewake: compacted worker-fixture (compaction 1).",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("letters %q, want %q", got, want)
		}
	}
	if len(compactions()) != 2 {
		t.Fatalf("letters %q", got)
	}
	if pending := control.PendingOf(dir, main.Name); len(pending) != 0 {
		t.Fatalf("the records outlived their letters: %+v", pending)
	}
}

// A worker that lives on with no word of the compaction gets its letter at
// the bound, and once.
func TestALetterComesAtTheBoundWithoutAWord(t *testing.T) {
	saved := letterBound
	letterBound = 100 * time.Millisecond
	t.Cleanup(func() { letterBound = saved })
	dir, main, peer, observer := lettersFixture(t)
	pend(t, dir, main, peer, letterA)
	scanTimes(t, observer, dir, main, 1)
	if got := letters(t, dir, main.Name); len(got) != 0 {
		t.Fatalf("a letter before the bound: %+v", got)
	}
	time.Sleep(letterBound)
	scanTimes(t, observer, dir, main, 3)
	want := "Rewake: no outcome of the compaction of worker-fixture you asked for was seen within 100ms; rewake list shows whether it compacted."
	if got := letters(t, dir, main.Name); len(got) != 1 || got[0].Text != want {
		t.Fatalf("letters %+v, want one saying %q", got, want)
	}
}

// A record an earlier run of main left is closed by the next run, to it, and
// the letter says whose request it was.
func TestAnEarlierRunsRequestIsClosedByTheNext(t *testing.T) {
	dir, main, peer, observer := lettersFixture(t)
	earlier := main.StartedAt.Add(-time.Hour)
	letterState(t, dir, peer, nil, []sessionstate.CompactionOutcome{{Request: letterA, RequestedBy: main.Name, Outcome: control.Failed, Detail: "the connection ended during the compaction", EndedAt: earlier}})
	pendAt(t, dir, main, peer, letterA, "111.0", earlier)
	scanTimes(t, observer, dir, main, 1)
	want := "Rewake: the compaction of worker-fixture you asked for failed (the connection ended during the compaction). You asked for it in an earlier run of this session."
	if got := letters(t, dir, main.Name); len(got) != 1 || got[0].Text != want || got[0].ToEpoch != main.Epoch() {
		t.Fatalf("letters %+v, want one saying %q", got, want)
	}
}

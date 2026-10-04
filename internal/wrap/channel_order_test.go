package wrap

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/proc"
)

// The keeper's wait for a close reorders nothing
// (docs/mail-bridge-channel.md#as-built): only the close waits, everything
// else folds when told, and the close, folded a heartbeat later, lands where
// it happened by event time. A close younger than a heartbeat waits for the
// next.

func TestACloseFoldedAfterLaterFailuresLandsWhereItHappened(t *testing.T) {
	k := newChannelKeeper(stateDir(t), "api", "1.2.b", "codex")
	k.begin(true, "")
	k.alive = func() bool { return true }
	hello, closed, refused := ago(4*heartbeat), ago(3*heartbeat), ago(2*heartbeat)
	k.tell(channel.Event{Kind: channel.Hello, Generation: 1, At: hello})
	k.tell(channel.Event{Kind: channel.Closed, Generation: 1, At: closed})
	k.tell(channel.Event{Kind: channel.NotObserved, At: refused})
	if record := k.snapshot(); record.Interval != refused || len(k.held) != 1 {
		t.Fatalf("interval %v with %d held, want the failure told at once and the close waiting", record.Interval, len(k.held))
	}
	k.mu.Lock()
	k.foldRipe(ago(0), true)
	k.mu.Unlock()
	record := k.snapshot()
	if record.Interval != closed || record.Class != channel.ClassNotObserved {
		t.Fatalf("interval %v class %q, want the close's time and the later failure's class", record.Interval, record.Class)
	}
}

// A ticket told after two later failures, while a close waits, folds in its
// place: the interval starts at the first failure after it, not at the
// latest.
func TestATicketToldAfterLaterFailuresFoldsInItsPlace(t *testing.T) {
	k := newChannelKeeper(stateDir(t), "api", "1.2.b", "codex")
	k.begin(true, "")
	k.alive = func() bool { return true }
	k.tell(channel.Event{Kind: channel.Hello, Generation: 1, At: ago(8 * heartbeat)})
	k.tell(channel.Event{Kind: channel.Closed, Generation: 1, At: ago(6 * heartbeat)})
	first, latest := ago(4*heartbeat), ago(3*heartbeat)
	k.tell(channel.Event{Kind: channel.NotObserved, At: first})
	k.tell(channel.Event{Kind: channel.NotObserved, At: latest})
	k.tell(channel.Event{Kind: channel.Validated, At: ago(5 * heartbeat), Issued: ago(5 * heartbeat).Boot})
	k.mu.Lock()
	k.foldRipe(ago(0), true)
	k.mu.Unlock()
	if record := k.snapshot(); record.Interval != first {
		t.Fatalf("interval %v, want the first failure after the ticket %v (latest %v)", record.Interval, first, latest)
	}
}

func TestACloseYoungerThanAHeartbeatWaits(t *testing.T) {
	k := newChannelKeeper(stateDir(t), "api", "1.2.b", "codex")
	k.begin(true, "")
	k.tell(channel.Event{Kind: channel.Hello, Generation: 1, At: ago(3 * heartbeat)})
	now := ago(0)
	young := channel.Stamp{Boot: now.Boot - int64(heartbeat) + int64(time.Millisecond), Wall: now.Wall}
	// Whatever the teller says of the harness, the keeper decides it.
	k.tell(channel.Event{Kind: channel.Closed, Generation: 1, At: young, Alive: true})
	k.mu.Lock()
	k.foldRipe(now, true)
	held := len(k.held)
	k.mu.Unlock()
	if record := k.snapshot(); record.Open() || held != 1 {
		t.Fatalf("a close %v old was folded: %+v, %d held", time.Duration(now.Boot-young.Boot), record, held)
	}
	// The run ends before it ripens: the close is the end's.
	k.exited()
	if record := k.snapshot(); record.Open() || !record.Frozen {
		t.Fatalf("a close within a heartbeat of the end failed the tool: %+v", record)
	}
}

// Notices fixed while the worker's mailbox cannot be written do not pile
// up: one is in flight, the rest wait for it to settle.
func TestNoticesWaitForTheOneInFlight(t *testing.T) {
	dir := stateDir(t)
	k := newChannelKeeper(dir, "api", "1.2.b", "codex")
	k.begin(true, "")
	worker := channel.Recipient{Role: channel.ToWorker, Name: "api", Epoch: k.epoch}
	planned := 0
	for i := range 8 {
		class := channel.ClassServerGone
		if i%2 == 1 {
			class = channel.ClassNotObserved
		}
		at := ago(time.Duration(8-i) * time.Second)
		k.record = channel.Record{Harness: channel.Codex, Tool: channel.ToolFailing, Class: class, Interval: at}
		if _, ok := k.notices.Plan(&k.record, worker, at); ok {
			planned++
		}
	}
	for _, p := range k.notices.Unsent() {
		k.publish(context.Background(), p)
	}
	if got := availabilityFiles(t, dir, "api"); planned != 1 || len(got) != 1 {
		t.Fatalf("planned %d, published %d", planned, len(got))
	}
}

// A notice fixed before the freeze and not yet written is never written.
func TestANoticeFixedBeforeTheExitIsNotWrittenAfterIt(t *testing.T) {
	dir := stateDir(t)
	k := failingKeeper(t, dir, "api", true)
	k.mu.Lock()
	k.foldRipe(ago(0), true)
	p, ok := k.notices.Plan(&k.record, channel.Recipient{Role: channel.ToWorker, Name: "api", Epoch: k.epoch}, ago(0))
	k.mu.Unlock()
	if !ok {
		t.Fatal("no notice fixed")
	}
	k.exited()
	if state, _ := k.attempt(context.Background(), p); state != channel.Dropped {
		t.Fatalf("an attempt after the exit: %q", state)
	}
	if got := availabilityFiles(t, dir, "api"); len(got) != 0 {
		t.Fatalf("written after the exit: %+v", got)
	}
}

// A backend gone ends the run from the wrapper's side: the record freezes
// before the harness is told to end.
func TestALostBackendFreezesTheRecordBeforeTheHarnessEnds(t *testing.T) {
	k := newChannelKeeper(stateDir(t), "api", "1.2.b", "codex")
	k.begin(true, "")
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var frozenFirst bool
	ending := func() {
		frozenFirst = proc.Alive(child.Process.Pid, start)
		k.exited()
		// Nothing to wait for after the signal: the test ends there.
		cancel()
	}
	stopWithBackend(ctx, goneBackend{}, child.Process, start, ending)
	if record := k.snapshot(); !record.Frozen || !frozenFirst {
		t.Fatalf("frozen %v, before the harness was signaled %v", record.Frozen, frozenFirst)
	}
}

// goneBackend is a backend that is already gone.
type goneBackend struct{ harness.Backend }

func (goneBackend) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

// A signal the wrapper accepts ends the run: the record freezes at once.
func TestAnAcceptedSignalFreezesTheRecord(t *testing.T) {
	k := newChannelKeeper(stateDir(t), "api", "1.2.b", "codex")
	k.begin(true, "")
	incoming := make(chan os.Signal, 1)
	incoming <- syscall.SIGTERM
	close(incoming)
	forward(incoming, &os.Process{Pid: -1}, func() bool { return false }, k.exited)
	if record := k.snapshot(); !record.Frozen {
		t.Fatalf("not frozen after SIGTERM: %+v", record)
	}
}

// A denial the keeper was told stops shell advice at once, even while a
// close waits: the publication guard under the mailbox lock sees it. Only a
// ticket issued after the latest denial lifts it.
func TestADenialToldWhileACloseWaitsStopsAdviceAtOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		// denied are the denials, ticket the issue time of a ticket that
		// validated after them, told after the advice was attempted; both
		// while the close waits.
		denied []time.Duration
		ticket time.Duration
		// lifts says the ticket lifts the block.
		lifts bool
	}{
		{name: "one denial", denied: []time.Duration{4 * heartbeat}},
		{name: "a ticket between two denials", denied: []time.Duration{5 * heartbeat, 3 * heartbeat}, ticket: 4 * heartbeat},
		{name: "a ticket issued before the denial", denied: []time.Duration{3 * heartbeat}, ticket: 4 * heartbeat},
		{name: "a ticket issued after the latest denial", denied: []time.Duration{5 * heartbeat, 4 * heartbeat}, ticket: 3 * heartbeat, lifts: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := stateDir(t)
			k := newChannelKeeper(dir, "api", "1.2.b", "codex")
			k.begin(true, "")
			k.alive = func() bool { return true }
			k.tell(channel.Event{Kind: channel.NotObserved, At: ago(9 * heartbeat)})
			advice, ok := k.notices.Plan(&k.record, channel.Recipient{Role: channel.ToWorker, Name: k.name, Epoch: k.epoch}, ago(9*heartbeat))
			if !ok || !advice.Advice {
				t.Fatal("no shell advice fixed")
			}
			k.tell(channel.Event{Kind: channel.Hello, Generation: 1, At: ago(8 * heartbeat)})
			k.tell(channel.Event{Kind: channel.Closed, Generation: 1, At: ago(heartbeat / 2)})
			for _, at := range tc.denied {
				k.tell(channel.Event{Kind: channel.Denied, At: ago(at)})
			}
			if len(k.held) == 0 {
				t.Fatal("no close waits: the test proves nothing")
			}
			if result, _ := k.attempt(context.Background(), advice); result != channel.Dropped {
				t.Fatalf("advice %q while a close waited after a told denial", result)
			}
			if letters := availabilityFiles(t, dir, "api"); len(letters) > 0 {
				t.Fatalf("%d letters written after a denial", len(letters))
			}
			if tc.ticket != 0 {
				k.tell(channel.Event{Kind: channel.Validated, At: ago(2 * heartbeat), Issued: ago(tc.ticket).Boot})
			}
			k.mu.Lock()
			k.foldRipe(ago(-heartbeat), true)
			lifted := !k.record.Blocked()
			k.mu.Unlock()
			if lifted != tc.lifts {
				t.Fatalf("block lifted %v, want %v", lifted, tc.lifts)
			}
		})
	}
}

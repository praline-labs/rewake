package endpoint

import (
	"cmp"
	"encoding/json"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/channel"
)

// The endpoint tells each connection's events from that connection's own
// goroutine: nothing orders their delivery. These tests hold one event's delivery after it was
// stamped while other events go through, and fold everything into a record
// as the keeper would (docs/mail-bridge-channel.md, rule 5).

// folding is a record fed by an endpoint's channel, with one kind of event
// held after its stamp until released.
type folding struct {
	t       *testing.T
	mu      sync.Mutex
	begun   channel.Stamp
	record  channel.Record
	arrived []channel.Event
	holds   func(channel.Event) bool
	held    chan struct{}
	release chan struct{}
	once    sync.Once
}

func newFolding(t *testing.T, holds func(channel.Event) bool) *folding {
	begun := Stamp()
	f := &folding{
		t: t, begun: begun, record: channel.New(true, "", begun), holds: holds,
		held: make(chan struct{}), release: make(chan struct{}),
	}
	t.Cleanup(f.let)
	return f
}

func (f *folding) take(e channel.Event) {
	if f.holds(e) {
		close(f.held)
		<-f.release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// The harness lives: the keeper would say so a heartbeat later.
	e.Alive = true
	f.arrived = append(f.arrived, e)
	f.record.Fold(e)
}

func (f *folding) let() { f.once.Do(func() { close(f.release) }) }

// until waits for a condition on what arrived.
func (f *folding) until(what string, done func([]channel.Event) bool) {
	f.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		ok := done(f.arrived)
		f.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.let()
	f.t.Fatalf("%s did not arrive: %+v", what, f.arrived)
}

// inOrder is the record the same events give folded in the order they
// happened: what a late delivery must not change.
func (f *folding) inOrder() channel.Record {
	events := slices.Clone(f.arrived)
	slices.SortStableFunc(events, func(a, b channel.Event) int { return cmp.Compare(a.At.Boot, b.At.Boot) })
	r := channel.New(true, "", f.begun)
	for _, e := range events {
		r.Fold(e)
	}
	return r
}

// sameRecord says whether two records show the same thing.
func sameRecord(a, b channel.Record) bool {
	left, _ := json.Marshal(a.View())
	right, _ := json.Marshal(b.View())
	return string(left) == string(right)
}

func arrivedAs(kind channel.Kind, generation uint64) func([]channel.Event) bool {
	return func(events []channel.Event) bool {
		return slices.ContainsFunc(events, func(e channel.Event) bool { return e.Kind == kind && e.Generation == generation })
	}
}

// A transport's hello delivered after its own close — the run named another
// transport meanwhile — still counts at its own time: the record is the one
// the events give in the order they happened, the newer transport live as a
// reconnection after the first one's end, and once that one ends too, nothing
// is live.
func TestAHelloDeliveredLateKeepsItsConnectionLive(t *testing.T) {
	f := newFolding(t, func(e channel.Event) bool { return e.Kind == channel.Hello && e.Generation == 1 })
	served, _ := testEndpoint(t, testTransport, func(c *Config) { c.Channel = f.take })
	go served.SetTransport(101, 1)
	select {
	case <-f.held:
	case <-time.After(3 * time.Second):
		t.Fatal("the first hello was never told")
	}
	served.SetTransport(102, 2)
	f.until("the second transport's hello", arrivedAs(channel.Hello, 2))
	f.let()
	f.until("the first hello", arrivedAs(channel.Hello, 1))

	f.mu.Lock()
	arrived, r, want := slices.Clone(f.arrived), f.record, f.inOrder()
	f.mu.Unlock()
	if arrived[0].Kind != channel.Closed || arrived[0].Generation != 1 || arrived[2].Generation != 1 || arrived[2].At.Boot >= arrived[0].At.Boot {
		t.Fatalf("the schedule did not happen, nothing is proven: %+v", arrived)
	}
	if !sameRecord(r, want) {
		t.Fatalf("the late hello changed the record:\n%+v\nin event order:\n%+v", r.View(), want.View())
	}
	if !slices.Equal(r.Live, []uint64{2}) || r.Tool != channel.ToolConnected || r.Interval != arrived[0].At || r.Reconnected != arrived[1].At {
		t.Fatalf("live %v tool %q interval %v reconnected %v, want the second transport live, reconnected after the first one's end", r.Live, r.Tool, r.Interval, r.Reconnected)
	}

	served.SetTransport(0, 0)
	f.until("the second transport's end", arrivedAs(channel.Closed, 2))
	f.mu.Lock()
	defer f.mu.Unlock()
	if r := f.record; len(r.Live) != 0 || r.Class != channel.ClassServerGone || !r.Open() {
		t.Fatalf("live %v class %q open %v, want nothing live and the channel gone", r.Live, r.Class, r.Open())
	}
}

// A refused transport hello from the harness's tree, told before the run
// names its transport and delivered after that hello, keeps its failure: no
// transport lived at its time, so the interval opens there, and the hello is
// a reconnection during it.
func TestARefusedHelloDeliveredAfterAHelloKeepsItsFailure(t *testing.T) {
	f := newFolding(t, func(e channel.Event) bool { return e.Kind == channel.HelloRefused })
	served, path := testEndpoint(t, testTransport, func(c *Config) { c.Channel = f.take })
	refused, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = refused.Close() }()
	greeting, _ := json.Marshal(hello{Role: roleTransport})
	if _, err := refused.Write(append(greeting, '\n')); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.held:
	case <-time.After(3 * time.Second):
		t.Fatal("the refused hello was never told")
	}
	served.SetTransport(101, 1)
	f.until("the hello", arrivedAs(channel.Hello, 1))
	f.let()
	f.until("the refusal", func(events []channel.Event) bool { return len(events) == 2 })

	f.mu.Lock()
	defer f.mu.Unlock()
	failed, hello, r := f.arrived[1], f.arrived[0], f.record
	if failed.Kind != channel.HelloRefused || !failed.Descendant || failed.At.Boot >= hello.At.Boot {
		t.Fatalf("the schedule did not happen, nothing is proven: %+v", f.arrived)
	}
	if r.Interval != failed.At || r.Class != channel.ClassServerRefused || r.Reconnected != hello.At {
		t.Fatalf("interval %v class %q reconnected %v, want the failure at %v and the hello after it", r.Interval, r.Class, r.Reconnected, failed.At)
	}
}

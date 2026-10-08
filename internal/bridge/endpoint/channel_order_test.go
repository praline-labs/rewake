package endpoint

import (
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
	record  channel.Record
	arrived []channel.Event
	holds   func(channel.Event) bool
	held    chan struct{}
	release chan struct{}
	once    sync.Once
}

func newFolding(t *testing.T, holds func(channel.Event) bool) *folding {
	f := &folding{
		t: t, record: channel.New(true, "", Stamp()), holds: holds,
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

func arrivedAs(kind channel.Kind, generation uint64) func([]channel.Event) bool {
	return func(events []channel.Event) bool {
		return slices.ContainsFunc(events, func(e channel.Event) bool { return e.Kind == kind && e.Generation == generation })
	}
}

// A connection's hello delivered after another connection opened and closed
// still counts at its own time: that close left a server live, so it is no
// "server gone". Once the first connection closes too, the server is gone.
func TestAHelloDeliveredLateKeepsItsConnectionLive(t *testing.T) {
	f := newFolding(t, func(e channel.Event) bool { return e.Kind == channel.Hello && e.Generation == 1 })
	_, path := testEndpoint(t, testTransport, func(c *Config) { c.Channel = f.take })
	first, err := Dial(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	select {
	case <-f.held:
	case <-time.After(3 * time.Second):
		t.Fatal("the first hello was never told")
	}
	second, err := Dial(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
	f.until("the second connection's close", arrivedAs(channel.Closed, 2))
	f.mu.Lock()
	gone := f.record.Open()
	f.mu.Unlock()
	f.let()
	f.until("the first hello", arrivedAs(channel.Hello, 1))

	f.mu.Lock()
	arrived, r := slices.Clone(f.arrived), f.record
	f.mu.Unlock()
	if arrived[0].Generation != 2 || arrived[2].Generation != 1 || arrived[2].At.Boot >= arrived[0].At.Boot {
		t.Fatalf("the schedule did not happen, nothing is proven: %+v", arrived)
	}
	if !gone {
		t.Fatal("with the first hello not yet told, the second close did not read as the server gone")
	}
	if r.Open() || !slices.Equal(r.Live, []uint64{1}) || r.Tool != channel.ToolConnected {
		t.Fatalf("open %v class %q live %v tool %q, want the first connection live and no failure", r.Open(), r.Class, r.Live, r.Tool)
	}

	first.Close()
	f.until("the first connection's close", arrivedAs(channel.Closed, 1))
	f.mu.Lock()
	defer f.mu.Unlock()
	if r := f.record; r.Class != channel.ClassServerGone || r.Interval != f.arrived[3].At {
		t.Fatalf("class %q since %v, want the server gone at the last close", r.Class, r.Interval)
	}
}

// A refused hello from the harness's tree, told before a good hello and
// delivered after it, keeps its failure: no connection lived at its time,
// so the interval opens there, and the hello is a reconnection during it.
func TestARefusedHelloDeliveredAfterAHelloKeepsItsFailure(t *testing.T) {
	f := newFolding(t, func(e channel.Event) bool { return e.Kind == channel.HelloRefused })
	_, path := testEndpoint(t, testTransport, func(c *Config) { c.Channel = f.take })
	refused, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer refused.Close()
	greeting, _ := json.Marshal(hello{Role: roleServer, Capability: "guess"})
	if _, err := refused.Write(append(greeting, '\n')); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.held:
	case <-time.After(3 * time.Second):
		t.Fatal("the refused hello was never told")
	}
	server, err := Dial(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
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

package host

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
)

// threadedHarness learns its conversation from the harness's own state, as an
// adapter.ThreadTracker; it answers only for the run's harness process.
type threadedHarness struct {
	fakeHarness
	asked adapter.RunProcess
}

var _ adapter.ThreadTracker = (*threadedHarness)(nil)

func (h *threadedHarness) Thread(run adapter.RunProcess) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asked = run
	return "selected-thread", nil
}

func TestTheWrapperPinsDeliveryToTheHarnessThread(t *testing.T) {
	dir := stateDir(t)
	fake := &threadedHarness{fakeHarness: fakeHarness{script: "sleep 1.5"}}
	sent := make(chan error, 1)
	go func() {
		end := time.Now().Add(time.Second)
		for time.Now().Before(end) {
			session, err := registry.Lookup(dir, "api-fake")
			if err == nil {
				sent <- mail.Put(dir, mail.Message{ID: mail.NewID(), From: "sender", FromEpoch: "2.2", To: "api-fake", ToEpoch: session.Epoch(), Text: "task", CreatedAt: time.Now()})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		sent <- registry.ErrNotFound
	}()
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	messages := fake.seen()
	if len(messages) != 1 || messages[0].DeliveryThread != "selected-thread" {
		t.Fatalf("wrapper lost thread context: %+v", messages)
	}
	if fake.asked.PID == 0 || fake.asked.Start == 0 {
		t.Fatalf("the tracker was not told the harness process: %+v", fake.asked)
	}
}

// threadObserver is a run's service that reports telemetry and follows the
// conversation, as a harness's collector may: an adapter.Service, an
// adapter.Telemetry and an adapter.ThreadSource.
type threadObserver struct{}

var (
	_ adapter.Service      = threadObserver{}
	_ adapter.Telemetry    = threadObserver{}
	_ adapter.ThreadSource = threadObserver{}
)

func (threadObserver) SessionState() session.Snapshot { return session.Unknown("") }
func (threadObserver) Start(context.Context, func(string)) error {
	return nil
}
func (threadObserver) Close()                  {}
func (threadObserver) Thread() (string, error) { return "observed-thread", nil }

// threadObservedHarness is a tracker too, whose answer the run's own source
// overrides.
type threadObservedHarness struct{ threadedHarness }

func (h *threadObservedHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(request)
	observer := threadObserver{}
	plan.Service, plan.Telemetry, plan.Thread = observer, observer, observer
	return plan, err
}

// A run that follows its conversation pins a delivery to the one it heard
// last, over what a tracker would say, which is how a harness's reports learn
// its conversation was cleared.
func TestTheWrapperPinsDeliveryToTheObservedThread(t *testing.T) {
	dir := stateDir(t)
	fake := &threadObservedHarness{threadedHarness: threadedHarness{fakeHarness: fakeHarness{script: "sleep 1.5"}}}
	sent := make(chan error, 1)
	go func() {
		end := time.Now().Add(time.Second)
		for time.Now().Before(end) {
			session, err := registry.Lookup(dir, "api-fake")
			if err == nil {
				sent <- mail.Put(dir, mail.Message{ID: mail.NewID(), From: "sender", FromEpoch: "2.2", To: "api-fake", ToEpoch: session.Epoch(), Text: "task", CreatedAt: time.Now()})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		sent <- registry.ErrNotFound
	}()
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	messages := fake.seen()
	if len(messages) != 1 || messages[0].DeliveryThread != "observed-thread" {
		t.Fatalf("delivery not pinned to the observed thread: %+v", messages)
	}
}

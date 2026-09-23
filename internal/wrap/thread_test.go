package wrap

import (
	"context"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

type threadedHarness struct{ fakeHarness }

func (*threadedHarness) Thread(registry.Session) (string, error) { return "selected-thread", nil }

func TestTheWrapperPinsDeliveryToTheHarnessThread(t *testing.T) {
	dir := stateDir(t)
	fake := &threadedHarness{fakeHarness: fakeHarness{script: "sleep 1.5"}}
	sent := make(chan error, 1)
	go func() {
		end := time.Now().Add(time.Second)
		for time.Now().Before(end) {
			session, err := registry.Lookup(dir, "api-fake")
			if err == nil {
				sent <- inbox.Put(dir, inbox.Message{ID: inbox.NewID(), From: "sender", FromEpoch: "2.2", To: "api-fake", ToEpoch: session.Epoch(), Text: "task", CreatedAt: time.Now()})
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
}

// threadObserver is telemetry that follows the conversation, as Claude Code's
// collector does.
type threadObserver struct{}

func (threadObserver) SessionState() sessionstate.Snapshot { return sessionstate.Unknown("") }
func (threadObserver) Start(context.Context) error         { return nil }
func (threadObserver) Close()                              {}
func (threadObserver) Thread() (string, error)             { return "observed-thread", nil }

type threadObservedHarness struct{ fakeHarness }

func (h *threadObservedHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(request)
	plan.Observer = threadObserver{}
	return plan, err
}

// A harness with no backend pins a delivery to the conversation its observer
// heard last, which is how Claude Code's reports learn of a /clear.
func TestTheWrapperPinsDeliveryToTheObservedThread(t *testing.T) {
	dir := stateDir(t)
	fake := &threadObservedHarness{fakeHarness: fakeHarness{script: "sleep 1.5"}}
	sent := make(chan error, 1)
	go func() {
		end := time.Now().Add(time.Second)
		for time.Now().Before(end) {
			session, err := registry.Lookup(dir, "api-fake")
			if err == nil {
				sent <- inbox.Put(dir, inbox.Message{ID: inbox.NewID(), From: "sender", FromEpoch: "2.2", To: "api-fake", ToEpoch: session.Epoch(), Text: "task", CreatedAt: time.Now()})
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

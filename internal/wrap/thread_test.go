package wrap

import (
	"context"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
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
			session, err := registry.Lookup(dir, "api")
			if err == nil {
				sent <- inbox.Put(dir, inbox.Message{ID: inbox.NewID(), From: "sender", FromEpoch: "2.2", To: "api", ToEpoch: session.Epoch(), Text: "task", CreatedAt: time.Now()})
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

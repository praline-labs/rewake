package wrap

import (
	"context"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// shortWindow serves the wrappers a test starts with a window a fraction of
// the real one: what these tests ask is whether the wrapper sets a window, and
// the real three seconds would only add to every run.
func shortWindow(t *testing.T) {
	t.Helper()
	previous := inbox.Coalescing
	inbox.Coalescing = inbox.Window{Quiet: time.Second, Cap: 1500 * time.Millisecond}
	t.Cleanup(func() { inbox.Coalescing = previous })
}

// The wrapper serves its mailbox with the collection window: notes that
// arrive well apart, each far outside the 150 ms first collection, still
// wake the session once. A wrapper that served without the window would
// announce each on its own.
func TestWrapperGathersNotesApartIntoOneNotice(t *testing.T) {
	shortWindow(t)
	dir := stateDir(t)
	fake := &fakeHarness{script: "sleep 3"}
	texts := []string{"first left", "second left", "first available"}

	go func() {
		deadline := time.Now().Add(3 * time.Second)
		var session registry.Session
		for {
			found, err := registry.Lookup(dir, "api-fake")
			if err == nil {
				session = found
				break
			}
			if time.Now().After(deadline) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		for index, text := range texts {
			if index > 0 {
				time.Sleep(400 * time.Millisecond)
			}
			_ = inbox.Put(dir, inbox.Message{
				ID: inbox.NewID(), From: "web", To: "api-fake", ToEpoch: session.Epoch(),
				Kind: inbox.Note, Text: text, CreatedAt: time.Now(),
			})
		}
	}()

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	delivered := fake.seen()
	if len(delivered) != 1 || len(delivered[0].Batch) != len(texts) {
		t.Fatalf("delivered %d notice(s), want one carrying all %d notes: %+v", len(delivered), len(texts), delivered)
	}
}

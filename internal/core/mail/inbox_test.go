package mail

import (
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv(state.DirEnv, dir)
	resolved, err := state.Dir()
	if err != nil {
		t.Fatalf("state.Dir: %v", err)
	}
	checkRecordKinds(t, resolved)
	return resolved
}

func message(text string) Message {
	return Message{ID: NewID(), From: "web", To: "api", Text: text, CreatedAt: time.Now()}
}

func TestAwaitReturnsTheFinalStatus(t *testing.T) {
	dir := stateDir(t)
	sent := message("hi")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}

	// A pending status is written first: Await must keep waiting for the final
	// one rather than return the first thing it sees.
	if err := WriteStatus(dir, "api", sent.ID, Result{State: Pending, Detail: "not listening yet"}); err != nil {
		t.Fatalf("status: %v", err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = WriteStatus(dir, "api", sent.ID, Result{State: Delivered, Via: "socket"})
	}()

	status, ok := Await(dir, "api", sent.ID, 3*time.Second, nil)
	if !ok || status.State != Delivered {
		t.Fatalf("Await = %+v, %v; want delivered", status, ok)
	}
}

func TestAwaitGivesUpWithoutAStatus(t *testing.T) {
	dir := stateDir(t)
	began := time.Now()
	if _, ok := Await(dir, "api", "missing", 500*time.Millisecond, nil); ok {
		t.Error("Await reported a status that was never written")
	}
	// It has to wait out the time it was given: returning at once would report
	// a healthy delivery as no answer.
	if waited := time.Since(began); waited < 400*time.Millisecond {
		t.Errorf("Await gave up after %v, want it to wait the full timeout", waited)
	}
}

func TestIDsSortByTime(t *testing.T) {
	first := NewID()
	time.Sleep(2 * time.Millisecond)
	second := NewID()
	if first >= second {
		t.Errorf("ids do not sort by time: %q then %q", first, second)
	}

	// Two senders in the same millisecond must still get different ids: equal
	// ones would have one message overwrite the other in the mailbox.
	seen := map[string]bool{}
	for range 1000 {
		id := NewID()
		if seen[id] {
			t.Fatalf("id %q was produced twice", id)
		}
		seen[id] = true
	}
}

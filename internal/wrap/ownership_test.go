package wrap

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// A wrapper whose harness has ended wakes up to a name that may already belong
// to somebody else. Cleaning up by name then deletes a live session's record —
// which is what happened before the epoch decided.
func TestCleanupLeavesTheNextOwnerAlone(t *testing.T) {
	dir := stateDir(t)
	fake := &fakeHarness{script: "sleep 0.5"}

	// While ours runs, publish what will become the next owner of the name once
	// ours is gone: same name, another epoch.
	successor := registry.Session{
		Name:         "api",
		Harness:      "claude",
		ServicePID:   os.Getpid(),
		ServiceStart: selfStart(t) + 1, // a different epoch, still this process
		PIDNamespace: proc.Namespace(),
		CWD:          dir,
		StartedAt:    time.Now(),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := registry.Lookup(dir, "api"); err == nil {
				// Replace the record the way a takeover would, while the old
				// wrapper is still running and about to clean up.
				_ = registry.Update(dir, successor)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	<-done

	held, err := registry.Load(dir, "api")
	if err != nil {
		t.Fatalf("the next owner's record was deleted by the previous wrapper: %v", err)
	}
	if held.Epoch() != successor.Epoch() {
		t.Errorf("record = %+v, want the successor's", held)
	}
}

// Mail written for the session that has ended must not be failed by whoever
// holds the name afterwards, and mail already delivered must keep its result.
func TestShutdownOnlyRefusesItsOwnMail(t *testing.T) {
	dir := stateDir(t)
	fake := &fakeHarness{script: "sleep 1"}

	foreign := inbox.Message{
		ID:        inbox.NewID(),
		From:      "web",
		To:        "api",
		ToEpoch:   "999.999",
		Text:      "for a session that came before",
		CreatedAt: time.Now(),
	}
	if err := inbox.Put(dir, foreign); err != nil {
		t.Fatalf("put: %v", err)
	}

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	status, ok := inbox.ReadStatus(dir, "api", foreign.ID)
	if !ok || status.State != inbox.Failed {
		t.Fatalf("status = %+v, want a refusal written while the session ran", status)
	}
	if status.Detail == "" || status.Detail == "the session ended before this message could be delivered" {
		t.Errorf("detail = %q, want it to name the real reason: the mail was somebody else's", status.Detail)
	}
}

// The harness runs in a process group of its own. That is what makes Ctrl+C
// reach it and nothing else, and a forwarded signal exactly one signal.
func TestHarnessRunsInItsOwnProcessGroup(t *testing.T) {
	dir := stateDir(t)
	fake := &fakeHarness{script: "sleep 1"}

	seen := make(chan int, 1)
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			session, err := registry.Lookup(dir, "api")
			if err != nil || session.HarnessPID == 0 {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			group, err := syscall.Getpgid(session.HarnessPID)
			if err != nil {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			seen <- group
			return
		}
		close(seen)
	}()

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	group, ok := <-seen
	if !ok {
		t.Fatal("the harness process was never visible")
	}
	if own, err := syscall.Getpgid(os.Getpid()); err != nil {
		t.Fatalf("getpgid: %v", err)
	} else if group == own {
		t.Error("the harness shares the wrapper's process group, so one Ctrl+C would reach it twice")
	}
}

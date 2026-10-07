package wrap

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// endableMain publishes a ready main of this build served by a process of its
// own, and answers a function that ends it.
func endableMain(t *testing.T, dir, name string) (registry.Session, func()) {
	t.Helper()
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	end := func() { once.Do(func() { _ = child.Process.Kill(); _, _ = child.Process.Wait() }) }
	t.Cleanup(end)
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(state.SessionsPath(dir)); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Minute)
	main := registry.Session{
		Name: name, Role: role.Main.ID, Harness: "stub", Room: filepath.Base(dir), CWD: "/workspace",
		ServicePID: child.Process.Pid, ServiceStart: start, Boot: thisBoot(t), PIDNamespace: proc.Namespace(), StartedAt: at, MessagingReadyAt: &at,
	}
	if err := registry.Publish(dir, main); err != nil {
		t.Fatal(err)
	}
	return main, end
}

// A notice for a main whose run ends while the keeper waits for its mailbox
// lock is dropped: the run that decides the write is read inside the
// recipient's section (docs/v2/stage3-publication.md#the-contract), and the
// keeper's look at the room's main before it is an early answer only.
func TestANoticeForAMainThatEndsAtItsLockIsDropped(t *testing.T) {
	dir := stateDir(t)
	lead, end := endableMain(t, dir, "lead")
	k := failingKeeper(t, dir, "api", true)
	notice := channel.Publication{Seq: 1, To: channel.Recipient{Role: channel.ToMain, Name: "lead", Epoch: lead.Epoch()}, Key: "k", Body: "b", Fixed: endpoint.Stamp()}
	waiting, mailbox := make(chan struct{}), state.InboxPath(dir, "lead")
	var waited sync.Once
	state.LockWait = func(path string) {
		if path == mailbox {
			waited.Do(func() { close(waiting) })
		}
	}
	t.Cleanup(func() { state.LockWait = nil })
	held, release, released := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		released <- state.WithMailboxLock(context.Background(), dir, "lead", func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	answer := make(chan string, 1)
	go func() {
		kept, _ := k.attempt(context.Background(), notice)
		answer <- kept
	}()
	select {
	case <-waiting:
	case <-time.After(10 * time.Second):
		t.Fatal("the keeper never waited for main's lock")
	}
	end()
	close(release)
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	select {
	case kept := <-answer:
		if kept != channel.Dropped {
			t.Fatalf("a notice for a main that ended at its lock: %q", kept)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the attempt did not answer")
	}
	if got := availabilityFiles(t, dir, "lead"); len(got) != 0 {
		t.Fatalf("published to a main that had ended: %+v", got)
	}
}

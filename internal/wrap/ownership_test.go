package wrap

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
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
				// Put the successor's record in place the way a takeover does,
				// while the old wrapper is still running and about to clean up.
				// Written directly: Update now refuses to write over a record
				// that belongs to somebody else, which is the neighbouring fix.
				encoded, marshalErr := json.MarshalIndent(successor, "", "  ")
				if marshalErr == nil {
					_ = state.WriteAtomic(state.SessionPath(dir, "api"), encoded)
				}
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

// The harness shares the wrapper's process group, and that is the point: the
// terminal then treats it as the program it is, so Ctrl+C, Ctrl+Z and job
// control work exactly as they would without rewake in between. Giving it a
// group of its own was tried; it left a stopped harness holding the terminal
// and let a backgrounded rewake steal the terminal from the shell.
func TestHarnessSharesTheTerminalGroup(t *testing.T) {
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
	own, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		t.Fatalf("getpgid: %v", err)
	}
	if group != own {
		t.Errorf("harness group = %d, wrapper group = %d: the terminal would stop treating the harness as the foreground program", group, own)
	}
}

// A signal the harness already got through the shared process group is not
// repeated: for many programs the second one means "stop cleaning up and die".
// Both processes are in one group, so the wrapper cannot tell a copy of a group
// signal from one aimed at itself — it asks the harness instead, and a harness
// that is already gone is not signalled again.
func TestSignalIsNotRepeatedToAHarnessThatGotIt(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "sleep 5")
	if err := command.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()

	incoming := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		forward(incoming, command.Process, func() bool { return false })
	}()

	incoming <- syscall.SIGTERM
	time.Sleep(forwardGrace + 200*time.Millisecond)
	close(incoming)
	<-done

	if err := command.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the harness was signalled although it had already acted on one: %v", err)
	}
}

// A signal aimed at the wrapper alone reaches nobody else, so it is passed on.
func TestSignalAimedAtTheWrapperIsPassedOn(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "sleep 5")
	if err := command.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()

	incoming := make(chan os.Signal, 1)
	go forward(incoming, command.Process, func() bool { return true })
	incoming <- syscall.SIGTERM

	select {
	case <-exited:
	case <-time.After(forwardGrace + 3*time.Second):
		_ = command.Process.Kill()
		t.Fatal("the harness never received the signal the wrapper was sent")
	}
	close(incoming)
}

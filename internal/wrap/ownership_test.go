package wrap

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
		Name:         "api-fake",
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
			// Wait until the wrapper has finished writing its own record — it
			// knows the harness pid then — so the takeover lands after that and
			// before the cleanup, as it would in life.
			if ours, err := registry.Load(dir, "api-fake"); err == nil && ours.HarnessPID != 0 {
				// Put the successor's record in place the way a takeover does:
				// under the name lock. Written directly, because Publish would
				// refuse a name whose holder is alive, and without the lock a
				// write between the wrapper's own read and write was lost to it,
				// which made this test fail one run in many.
				encoded, marshalErr := json.MarshalIndent(successor, "", "  ")
				if marshalErr == nil {
					_ = state.WithNameLock(dir, "api-fake", func() error {
						return state.WriteAtomic(state.SessionPath(dir, "api-fake"), encoded)
					})
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

	held, err := registry.Load(dir, "api-fake")
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
		To:        "api-fake",
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

	status, ok := inbox.ReadStatus(dir, "api-fake", foreign.ID)
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
			session, err := registry.Lookup(dir, "api-fake")
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
// that is already gone is not signaled again.
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
		t.Fatalf("the harness was signaled although it had already acted on one: %v", err)
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

// Ctrl+C reaches the wrapper as well as the harness, and dying from it left the
// agent running with nobody serving its mailbox: interrupting a turn must not
// end the session.
func TestInterruptDoesNotEndTheSession(t *testing.T) {
	dir := stateDir(t)
	// The harness ignores the interrupt and keeps running, the way an agent
	// that is merely canceling a turn does.
	fake := &fakeHarness{script: "trap '' INT; for _ in $(seq 1 20); do sleep 0.1; done"}

	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			session, err := registry.Lookup(dir, "api-fake")
			if err != nil || session.HarnessPID == 0 {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			// To the group, the way the terminal delivers Ctrl+C.
			group, err := syscall.Getpgid(session.HarnessPID)
			if err == nil && group != 0 {
				_ = syscall.Kill(session.HarnessPID, syscall.SIGINT)
				_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
			}
			return
		}
	}()

	code, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want the harness's own 0: the interrupt should not have ended anything", code)
	}
}

// A harness that stops itself leaves the shell waiting on a wrapper that is
// still running, with no prompt and no way to bring the job back. The wrapper
// stops with it — which is why the continue has to come from outside: this
// process is the wrapper, and a stopped process cannot wake itself.
func TestWrapperStopsWithTheHarness(t *testing.T) {
	dir := stateDir(t)
	fake := &fakeHarness{script: "kill -STOP $$; sleep 0.2"}

	self := strconv.Itoa(os.Getpid())
	continuer := exec.Command("/bin/sh", "-c",
		"for _ in $(seq 1 200); do "+
			"state=$(cut -d' ' -f3 /proc/"+self+"/stat 2>/dev/null); "+
			"if [ \"$state\" = T ]; then kill -CONT "+self+"; exit 0; fi; "+
			"sleep 0.05; done")
	if err := continuer.Start(); err != nil {
		t.Fatalf("start the continuer: %v", err)
	}
	defer func() {
		_ = continuer.Process.Kill()
		_ = continuer.Wait()
	}()

	began := time.Now()
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Being stopped and continued is the point: a wrapper that ignored the stop
	// would have returned without ever pausing.
	if waited := time.Since(began); waited < 100*time.Millisecond {
		t.Errorf("the run took %v: the wrapper did not follow the harness into its stop", waited)
	}
}

// bindSocket leaves a socket file at a path, as a harness that bound it does.
func bindSocket(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen %s: %v", path, err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = listener.Close()
}

func isSocket(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

// The name changed hands while this session ran. Its own socket still goes when
// it ends, and the next holder's socket — at a path of its own — stays.
func TestEachRunCleansOnlyItsOwnSocket(t *testing.T) {
	dir := stateDir(t)
	ours := registry.SocketFor(dir, "api-fake", strconv.Itoa(os.Getpid())+"."+strconv.FormatUint(selfStart(t), 10))
	theirs := registry.SocketFor(dir, "api-fake", "999.1")
	if ours == theirs {
		t.Fatalf("two runs of a name share the socket path %s", ours)
	}
	bindSocket(t, theirs)

	fake := &fakeHarness{script: "sleep 0.5", socket: true}
	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if held, err := registry.Load(dir, "api-fake"); err == nil && held.HarnessPID != 0 {
				bindSocket(t, ours)
				successor := held
				successor.ServiceStart++
				encoded, _ := json.MarshalIndent(successor, "", "  ")
				_ = state.WithNameLock(dir, "api-fake", func() error {
					return state.WriteAtomic(state.SessionPath(dir, "api-fake"), encoded)
				})
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	<-done

	if isSocket(ours) {
		t.Error("the ended run left its socket behind")
	}
	if !isSocket(theirs) {
		t.Error("the ended run removed the socket of the next holder of the name")
	}
}

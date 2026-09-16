package wrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// fakeHarness runs a shell command instead of an agent, and records what it was
// asked to deliver. The wrapper is what is under test here, not a harness.
type fakeHarness struct {
	script string
	// socket makes the launch claim the socket path it is given, as Claude
	// Code's does.
	socket bool

	mu        sync.Mutex
	delivered []inbox.Message
	result    inbox.Result
}

func (f *fakeHarness) ID() string         { return "fake" }
func (f *fakeHarness) Title() string      { return "Fake" }
func (f *fakeHarness) Summary() string    { return "A harness that is a shell script." }
func (f *fakeHarness) Examples() []string { return []string{"rewake fake"} }
func (f *fakeHarness) Notes() []string    { return nil }

func (f *fakeHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	plan := harness.LaunchPlan{
		Command: "/bin/sh",
		Args:    []string{"-c", f.script},
		Env:     harness.SessionEnv(request, nil),
	}
	if f.socket {
		plan.Socket, plan.OwnsSocket = request.Socket, true
	}
	return plan, nil
}

func (f *fakeHarness) Deliver(_ context.Context, _ registry.Session, message inbox.Message) inbox.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered = append(f.delivered, message)
	if f.result.State == "" {
		return inbox.Result{State: inbox.Delivered, Via: "fake"}
	}
	return f.result
}

func (f *fakeHarness) seen() []inbox.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]inbox.Message{}, f.delivered...)
}

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
	return resolved
}

func TestExitCodeOfTheHarnessIsReturned(t *testing.T) {
	dir := stateDir(t)
	code, err := Run(context.Background(), Request{
		Harness: &fakeHarness{script: "exit 7"},
		Dir:     dir,
		Name:    "api",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 7 {
		t.Errorf("code = %d, want the harness's own 7", code)
	}
}

// A harness killed by a signal has no exit code. Passing on what Go reports
// (-1) would reach the shell as 255 and hide the reason.
func TestSignalledHarnessReportsShellStyleCode(t *testing.T) {
	dir := stateDir(t)
	code, err := Run(context.Background(), Request{
		Harness: &fakeHarness{script: "kill -TERM $$; sleep 5"},
		Dir:     dir,
		Name:    "api",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 143 {
		t.Errorf("code = %d, want 143 (128 + SIGTERM)", code)
	}
}

func TestSessionIsPublishedWhileRunningAndRemovedAfter(t *testing.T) {
	dir := stateDir(t)
	fake := &fakeHarness{script: "sleep 2"}

	seen := make(chan registry.Session, 1)
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if session, err := registry.Lookup(dir, "api"); err == nil {
				seen <- session
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		close(seen)
	}()

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	session, ok := <-seen
	if !ok {
		t.Fatal("the session was never visible while the harness ran")
	}
	if session.HarnessPID == 0 || session.ServicePID != os.Getpid() {
		t.Errorf("record = %+v, want both the wrapper and the harness recorded", session)
	}
	if _, err := registry.Load(dir, "api"); err == nil {
		t.Error("the record outlived the session")
	}
}

func TestMailboxIsServedWhileTheHarnessRuns(t *testing.T) {
	dir := stateDir(t)
	fake := &fakeHarness{script: "sleep 2"}

	go func() {
		// Wait for the session, then write into its mailbox the way a sender does.
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			session, err := registry.Lookup(dir, "api")
			if err != nil {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			_ = inbox.Put(dir, inbox.Message{
				ID:        inbox.NewID(),
				From:      "web",
				To:        "api",
				ToEpoch:   session.Epoch(),
				Text:      "hello",
				CreatedAt: time.Now(),
			})
			return
		}
	}()

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	delivered := fake.seen()
	if len(delivered) != 1 || delivered[0].Text != "hello" {
		t.Fatalf("delivered = %v, want exactly the one message", delivered)
	}
}

// Mail addressed to a session that has ended must not be handed to the next
// session that happens to take the same name.
func TestMailOfAPreviousSessionIsRefused(t *testing.T) {
	dir := stateDir(t)
	if err := inbox.Put(dir, inbox.Message{
		ID:        inbox.NewID(),
		From:      "web",
		To:        "api",
		ToEpoch:   "999.999",
		Text:      "for whoever was called api yesterday",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	fake := &fakeHarness{script: "sleep 1"}
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if delivered := fake.seen(); len(delivered) != 0 {
		t.Fatalf("delivered = %v, want nothing: that mail belonged to an earlier session", delivered)
	}
}

func TestSessionEnvironmentReachesTheHarness(t *testing.T) {
	dir := stateDir(t)
	out := filepath.Join(t.TempDir(), "env.txt")
	fake := &fakeHarness{script: "printf '%s %s %s' \"$REWAKE_SESSION\" \"$REWAKE_DIR\" \"$REWAKE_EPOCH\" > " + out}
	// Inherited from a parent session, these must not leak into this one.
	t.Setenv("REWAKE_EPOCH", "1.1")

	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// The run is this wrapper's: its pid and start time.
	want := fmt.Sprintf("api %s %d.%d", state.RootForRoom(dir), os.Getpid(), selfStart(t))
	if got := string(content); got != want {
		t.Errorf("environment = %q, want %q", got, want)
	}
}

// An automatic name retries; an explicit one is a wrong call the caller has to
// see, because they are about to hand that address to somebody else.
func TestTakenNameIsRefusedOnlyWhenExplicit(t *testing.T) {
	dir := stateDir(t)
	blocker := registry.Session{
		Name:         "fake",
		Harness:      "fake",
		ServicePID:   os.Getpid(),
		ServiceStart: selfStart(t),
		CWD:          dir,
		StartedAt:    time.Now(),
	}
	if err := registry.Publish(dir, blocker); err != nil {
		t.Fatalf("publish: %v", err)
	}

	fake := &fakeHarness{script: "sleep 0.3"}
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir}); err != nil {
		t.Fatalf("an automatic name did not step aside: %v", err)
	}

	_, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "fake"})
	if err == nil {
		t.Fatal("an explicit taken name was accepted")
	}
	if !strings.Contains(err.Error(), "taken") {
		t.Errorf("refusal = %v, want it to say the name is taken", err)
	}
}

func selfStart(t *testing.T) uint64 {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatalf("start time: %v", err)
	}
	return start
}

func TestTheRoleIsRecorded(t *testing.T) {
	dir := stateDir(t)
	fake := &fakeHarness{script: "sleep 0.5"}
	recorded := make(chan string, 1)
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if session, err := registry.Load(dir, "lead"); err == nil {
				recorded <- session.Role
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		recorded <- "never"
	}()
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "lead", Role: role.Main}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := <-recorded; got != role.Main.ID {
		t.Errorf("recorded role = %q, want main", got)
	}
}

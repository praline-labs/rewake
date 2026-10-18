package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A decision takes its room's lease before any other lock and holds it
// through its last effect (docs/v2/stage3-decision-build.md, S17's lease): in
// a room of another build both routes refuse with nothing written — no
// decision, no lock, no temporary file, no shell note — and neither asks who
// may decide; while a launch holds the lease to switch the build, a decision
// waits for it holding no mailbox; and every write and read of a decision is
// made while no launch could switch the room, the lease going only after
// them all.
func TestDecisionLeasePrecedesAllLocks(t *testing.T) {
	t.Run("another build's room", func(t *testing.T) {
		lab := newDecideLab(t)
		t.Setenv(state.SessionEnv, "web")
		r := &route{below: true, wrapper: lab.asWrapper}
		r.install(t)
		foreign := strings.Repeat("ab", 32)
		if err := os.WriteFile(filepath.Join(lab.dir, "build"), []byte(foreign+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Held, the test's own lease would let a stray write land; released,
		// enforcement refuses it and counts the attempt.
		buildtest.Release(lab.dir)
		before, refused := snapshotTree(t, lab.dir), state.NoLeaseRefusals()
		code, _, errOut := run("decide", "api", lab.report.ID, "--arrived")
		if code != ExitFailed || !strings.Contains(errOut, "belongs to build "+build.Short(foreign)) {
			t.Fatalf("the shell: exit %d, %q", code, errOut)
		}
		if r.asked != 0 || r.scanned != 0 {
			t.Fatalf("before the lease: the wrapper asked %d, the person classified %d", r.asked, r.scanned)
		}
		if n := state.NoLeaseRefusals() - refused; n != 0 {
			t.Errorf("the refused decisions tried %d writes", n)
		}
		if after := snapshotTree(t, lab.dir); after != before {
			t.Fatalf("a refused decision changed the room:\n--- before\n%s\n--- after\n%s", before, after)
		}
	})

	t.Run("a launch switching the build", func(t *testing.T) {
		lab := newDecideLab(t)
		(&route{}).install(t)
		buildtest.Release(lab.dir)
		holder := holdExclusive(t, lab.dir)
		defer holder()
		done := make(chan int, 1)
		go func() {
			code, _, _ := run("decide", "api", lab.report.ID, "--arrived")
			done <- code
		}()
		time.Sleep(300 * time.Millisecond) //nolint:forbidigo // the decision waits for the lease meanwhile
		select {
		case code := <-done:
			t.Fatalf("the decision finished, exit %d, while a launch held the lease", code)
		default:
		}
		for _, name := range []string{"api", "web"} {
			if !lockable(t, filepath.Join(state.InboxPath(lab.dir, name), ".lock")) {
				t.Errorf("the decision held the mailbox of %s while it waited for the lease", name)
			}
		}
		if lab.decision(t) != nil {
			t.Fatal("decided before the lease")
		}
		holder()
		select {
		case code := <-done:
			if code != ExitOK || lab.decision(t) == nil {
				t.Fatalf("after the switch let go: exit %d, decision %+v", code, lab.decision(t))
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the decision never took the lease")
		}
	})

	t.Run("held through the last effect", func(t *testing.T) {
		lab := newDecideLab(t)
		(&route{}).install(t)
		buildtest.Release(lab.dir)
		lease := filepath.Join(lab.dir, "build.lock")
		var steps []string
		state.Fault = func(op, path string) error {
			switch {
			case op == state.OpStep:
			case op == state.OpNote:
				steps = append(steps, op+" "+path)
			case !strings.HasPrefix(path, lab.dir+string(filepath.Separator)) || path == lease || path == filepath.Join(lab.dir, "build"):
			default:
				steps = append(steps, fmt.Sprintf("%s %s leased=%v switchable=%v", op, strings.TrimPrefix(path, lab.dir), state.Leased(lab.dir), lockable(t, lease)))
			}
			return nil
		}
		t.Cleanup(func() { state.Fault = nil })
		code, out, errOut := run("decide", "api", lab.report.ID, "--not-arrived")
		state.Fault = nil
		if code != ExitOK || lab.decision(t) == nil {
			t.Fatalf("exit %d:\n%s%s", code, out, errOut)
		}
		decided, resent, released := false, false, false
		for _, step := range steps {
			switch {
			case step == state.OpNote+" lease release":
				released = true
			case strings.HasPrefix(step, state.OpNote+" "):
			case released:
				t.Fatalf("%q after the lease went: %v", step, steps)
			case !strings.HasSuffix(step, "leased=true switchable=false"):
				t.Fatalf("%q while a launch could switch the room: %v", step, steps)
			case strings.HasPrefix(step, state.OpPublish+" "+strings.TrimPrefix(mail.DecisionPath(lab.dir, "api", lab.report.ID), lab.dir)+" "):
				decided = true
			case strings.HasPrefix(step, state.OpWrite+" "+strings.TrimPrefix(state.InboxPath(lab.dir, "web"), lab.dir)+"/") && strings.Contains(step, ".json "):
				resent = true
			}
		}
		if !decided || !resent || !released {
			t.Fatalf("the decision written %v, its resend %v, the lease released %v: %v", decided, resent, released, steps)
		}
	})
}

// lockable answers whether the lock at path could be taken exclusive now,
// through a descriptor of its own: the room's lease as a launch switching the
// build takes it, or a mailbox's as any section does. A lock never made is
// held by nobody.
func lockable(t *testing.T, path string) bool {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil {
		t.Errorf("the lock %s: %v", path, err)
		return false
	}
	defer func() { _ = file.Close() }()
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true
	}
	if !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Errorf("the lock %s: %v", path, err)
	}
	return false
}

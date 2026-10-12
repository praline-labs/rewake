package build

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// refusal is the RefusedError an acquisition must have returned.
func refusal(t *testing.T, lease *Lease, err error, what string) *RefusedError {
	t.Helper()
	var refused *RefusedError
	if !errors.As(err, &refused) {
		lease.Release()
		t.Fatalf("%s: %v, not a refusal", what, err)
	}
	return refused
}

func holds(refused *RefusedError, pids ...int) bool {
	for _, pid := range pids {
		if !slices.ContainsFunc(refused.Holders, func(h Holder) bool { return h.Pid == pid }) {
			return false
		}
	}
	return true
}

// A missing, malformed or foreign record is replaced only by a launch and
// only exclusively: a writer of this build refuses it, a launch refuses it
// while another process holds the lease — naming that process — and leaves
// the record as it was; once nothing holds the room, the launch writes its
// own build and holds the lease shared.
func TestLaunchReplacesMissingOrInvalidBuildOnlyExclusively(t *testing.T) {
	// The holder stays: the launch beside it is refused once its retries end.
	before := switchWait
	switchWait = 100 * time.Millisecond
	t.Cleanup(func() { switchWait = before })
	foreign := strings.Repeat("ab", 32)
	own, err := ID()
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"missing": "missing", "malformed": "not an id\n", "upper case": strings.ToUpper(foreign) + "\n",
		"foreign": foreign + "\n", "empty": "",
	} {
		t.Run(name, func(t *testing.T) {
			base := newBase(t)
			room := established(t, base, "default")
			writeRecord(t, room, content)

			lease, err := Acquire(base, "default")
			_ = refusal(t, lease, err, "a writer")
			if got := record(t, room); got != content {
				t.Fatalf("a writer changed the record to %q", got)
			}

			other := startHolder(t, "shared", leaseLockPath(room))
			if !other.held() {
				t.Fatalf("the other process: %s", other.answer)
			}
			lease, err = AcquireLaunch(base, "default")
			refused := refusal(t, lease, err, "a launch beside a holder")
			if !holds(refused, other.cmd.Process.Pid) {
				t.Fatalf("the refusal names %v, not the holder %d", refused.Holders, other.cmd.Process.Pid)
			}
			if got := record(t, room); got != content {
				t.Fatalf("a refused launch changed the record to %q", got)
			}
			if state.Leased(room) {
				t.Fatal("a refused launch left the room authorized")
			}

			other.release()
			lease, err = AcquireLaunch(base, "default")
			if err != nil {
				t.Fatalf("the launch once nothing holds the room: %v", err)
			}
			if got := record(t, room); got != own+"\n" {
				t.Fatalf("the launch wrote %q", got)
			}
			if !state.Leased(room) {
				t.Fatal("the launch holds no authorization")
			}
			lease.Release()
			if state.Leased(room) {
				t.Fatal("the authorization outlived the release")
			}
		})
	}
}

// Writers of the room's build hold the lease together, and while any of
// them does, no launch of another build switches the room; when the last one
// is gone the switch goes through, and the old build's writers are refused.
func TestSharedWritersExcludeBuildSwitch(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	before := record(t, room)
	first := startHolder(t, "writer", base, "default")
	second := startHolder(t, "writer", base, "default")
	if !first.held() || !second.held() {
		t.Fatalf("two writers of the room's build: %q, %q", first.answer, second.answer)
	}

	other := actAs(t, "another build")
	lease, err := AcquireLaunch(base, "default")
	refused := refusal(t, lease, err, "another build's launch beside two writers")
	if !holds(refused, first.cmd.Process.Pid, second.cmd.Process.Pid) {
		t.Fatalf("the refusal names %v", refused.Holders)
	}
	first.release()
	lease, err = AcquireLaunch(base, "default")
	_ = refusal(t, lease, err, "another build's launch beside one writer")
	if got := record(t, room); got != before {
		t.Fatalf("a refused switch changed the record to %q", got)
	}

	second.release()
	lease, err = AcquireLaunch(base, "default")
	if err != nil {
		t.Fatalf("the switch once the writers are gone: %v", err)
	}
	defer lease.Release()
	if got := record(t, room); got != other+"\n" {
		t.Fatalf("the switch wrote %q", got)
	}
	late := startHolder(t, "writer", base, "default")
	if late.held() || !strings.Contains(late.answer, "belongs to build "+Short(other)) {
		t.Fatalf("a writer of the old build after the switch: %q", late.answer)
	}
}

// The conversion from exclusive to shared is not atomic: a launch that
// slipped in between may have written its own build, and the launch reads
// the record again and refuses rather than writing under another build.
func TestDowngradeRechecksBuild(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	actAs(t, "another build")
	slipped := strings.Repeat("cd", 32)
	before := state.Fault
	t.Cleanup(func() { state.Fault = before })
	state.Fault = func(op, path string) error {
		if op == state.OpStep && path == "lease downgrade" {
			writeRecord(t, room, slipped+"\n")
		}
		return nil
	}
	lease, err := AcquireLaunch(base, "default")
	refused := refusal(t, lease, err, "a launch whose build was replaced during the conversion")
	if refused.Theirs != slipped {
		t.Fatalf("the refusal names build %q", refused.Theirs)
	}
	if state.Leased(room) {
		t.Fatal("the refused launch left the room authorized")
	}
}

// A lock file replaced while its descriptor waited locks nothing anyone else
// opens: once taken, the lock is checked against the path and opened again.
func TestReplacedLeaseLockIsReopened(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	lock, err := state.OpenLeaseLock(room)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	replacement := filepath.Join(room, "build.lock.new")
	if err := os.WriteFile(replacement, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, leaseLockPath(room)); err != nil {
		t.Fatal(err)
	}
	if err := lock.Share(time.Second); err != nil {
		t.Fatal(err)
	}
	_, ino, err := lock.File()
	if err != nil {
		t.Fatal(err)
	}
	var named syscall.Stat_t
	if err := syscall.Stat(leaseLockPath(room), &named); err != nil {
		t.Fatal(err)
	}
	if ino != named.Ino {
		t.Fatalf("the lock holds inode %d, the path is inode %d", ino, named.Ino)
	}
}

// A launch that stalls while it holds the lease exclusive does not stall the
// room's writers: each waits a bounded time and is refused, saying why.
func TestAStalledSwitchBoundsItsWriters(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	stalled := startHolder(t, "exclusive", leaseLockPath(room))
	if !stalled.held() {
		t.Fatalf("the stalled launch: %s", stalled.answer)
	}
	before := shareWait
	shareWait = 200 * time.Millisecond
	t.Cleanup(func() { shareWait = before })
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		lease, err := Acquire(base, "default")
		lease.Release()
		done <- err
	}()
	select {
	case err := <-done:
		var refused *RefusedError
		if !errors.As(err, &refused) || !refused.Busy || !strings.Contains(err.Error(), "has not finished within") {
			t.Fatalf("a writer beside a stalled switch: %v", err)
		}
		if waited := time.Since(started); waited < shareWait {
			t.Fatalf("refused after %s, before its bound", waited)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a writer beside a stalled switch waited without a bound")
	}
	if state.Leased(room) {
		t.Fatal("a refused writer left the room authorized")
	}
}

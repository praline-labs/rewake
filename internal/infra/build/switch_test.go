package build

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// candidate is an installed build to switch to: a file and its id.
func candidate(t *testing.T, content string) (path, id string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "rewake")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return path, hex.EncodeToString(sum[:])
}

// leading takes the room's lease as a writer of its build and claims a
// switch of it to target.
func leading(t *testing.T, base, room, target string) (*Lease, *Switch) {
	t.Helper()
	lease, err := Acquire(base, room)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	sw, err := Claim(lease, target, "", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return lease, sw
}

// launched makes a room whose build is this process's, with its own copy, as
// a launch of a session does.
func launched(t *testing.T, base, room string) string {
	t.Helper()
	lease, err := Launch(base, room)
	if err != nil {
		t.Fatalf("launch into an empty base: %v", err)
	}
	lease.Release()
	return roomOf(base, room)
}

// copyID is the id of the room's own copy, or "missing".
func copyID(t *testing.T, room string) string {
	t.Helper()
	id, err := FileID(filepath.Join(room, "bin", "rewake"))
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func phaseOf(t *testing.T, room string) string {
	t.Helper()
	mark, err := state.ReadSwitchMark(room)
	if err != nil {
		return err.Error()
	}
	return mark.Phase
}

// The build changes only under the exclusive lock with no other holder: a
// switch whose drop left a writer holding the lease names it and commits
// nothing, and commits once the writer is gone, then holds the lease shared
// with no authorization, writing nothing more.
func TestASwitchCommitsOnlyWithNoOtherHolder(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	before := record(t, room)
	path, target := candidate(t, "build B")
	writer := startHolder(t, "writer", base, "default")
	if !writer.held() {
		t.Fatalf("the writer: %s", writer.answer)
	}
	_, sw := leading(t, base, "default", target)
	if err := sw.Stage(path); err != nil {
		t.Fatal(err)
	}
	if err := sw.Phase(state.PhaseDropping, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := sw.Commit(time.Now().Add(time.Minute)); err == nil {
		t.Fatal("a commit under the shared lease went through")
	}
	holders, err := sw.Exclusive(200 * time.Millisecond)
	if err != nil || len(holders) != 1 || holders[0].Pid != writer.cmd.Process.Pid {
		t.Fatalf("the exclusive lock beside a writer: %v, %v", holders, err)
	}
	if got := record(t, room); got != before {
		t.Fatalf("a switch beside a writer wrote %q", got)
	}
	writer.release()
	if holders, err := sw.Exclusive(2 * time.Second); err != nil || len(holders) != 0 {
		t.Fatalf("the exclusive lock alone: %v, %v", holders, err)
	}
	if err := sw.Commit(time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := record(t, room); got != target+"\n" {
		t.Fatalf("the commit wrote %q", got)
	}
	if got := copyID(t, room); got != target {
		t.Fatalf("the room's copy is %s after the commit", got)
	}
	if state.Leased(room) {
		t.Fatal("the leader is still authorized after its commit")
	}
	if holders := sw.Holders(); len(holders) != 0 {
		t.Fatalf("holders after the commit: %v", holders)
	}
}

// A writer that meets a dropping switch lets go of the lease and waits, so the
// switch can take it alone: within its bound, then refused as busy; and it
// goes on as soon as the phase moves on.
func TestAWriterWaitsOutADroppingSwitchWithinItsBound(t *testing.T) {
	base := newBase(t)
	established(t, base, "default")
	before := shareWait
	shareWait = 300 * time.Millisecond
	t.Cleanup(func() { shareWait = before })
	_, sw := leading(t, base, "default", strings.Repeat("b", 64))
	if err := sw.Phase(state.PhaseDropping, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	lease, err := Acquire(base, "default")
	var refused *RefusedError
	if !errors.As(err, &refused) || !refused.Busy {
		lease.Release()
		t.Fatalf("a writer beside a dropping switch: %v", err)
	}
	if waited := time.Since(started); waited < shareWait {
		t.Fatalf("refused after %s, before its bound", waited)
	}
	done := make(chan error, 1)
	go func() {
		lease, err := Acquire(base, "default")
		lease.Release()
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if err := sw.Phase(state.PhaseAborting, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("a writer once the switch stopped dropping: %v", err)
	}
}

// A mark that holds nothing — asking for a leader that is gone, or past its
// deadline — is ignored and removed by the next writer granted; one dropping
// before its deadline holds the room whatever became of its leader.
func TestAStaleSwitchMarkIsIgnoredAndRemoved(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	gone := state.SwitchMark{Version: state.SwitchVersion, Leader: os.Getpid(), LeaderStart: 1, Target: strings.Repeat("b", 64), Phase: state.PhaseAsking, Deadline: time.Now().Add(time.Minute).UnixMilli()}
	past := gone
	past.LeaderStart, past.Phase, past.Deadline = 0, state.PhaseDropping, time.Now().Add(-time.Second).UnixMilli()
	held := gone
	held.Phase = state.PhaseDropping
	for name, mark := range map[string]state.SwitchMark{"asking, its leader gone": gone, "dropping, past its deadline": past} {
		mark := mark
		if SwitchLive(mark, time.Now()) {
			t.Fatalf("%s: live", name)
		}
		lease, err := Acquire(base, "default")
		if err != nil {
			t.Fatal(err)
		}
		_, err = state.ClaimSwitch(room, mark, func(state.SwitchMark) bool { return true })
		lease.Release()
		if err != nil {
			t.Fatal(err)
		}
		lease, err = Acquire(base, "default")
		if err != nil {
			t.Fatalf("%s: a writer: %v", name, err)
		}
		lease.Release()
		if _, err := state.ReadSwitchMark(room); !errors.Is(err, state.ErrNoSwitchMark) {
			t.Fatalf("%s: the mark stays: %v", name, err)
		}
	}
	if !SwitchLive(held, time.Now()) {
		t.Fatal("a dropping mark before its deadline does not hold the room once its leader is gone")
	}
}

// A leader killed while it drops leaves its parked images to recover: a
// writer — the lease a parked image takes — is granted at once rather than
// after the mark's deadline, when a launch could take the room first; and
// until that deadline a launch of another build is still refused.
func TestADroppingMarkWithoutItsLeaderGatesNoWriter(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	before := shareWait
	shareWait = 2 * time.Second
	t.Cleanup(func() { shareWait = before })
	gone := state.SwitchMark{
		Version: state.SwitchVersion, Leader: os.Getpid(), LeaderStart: 1, Target: strings.Repeat("b", 64),
		Phase: state.PhaseDropping, Deadline: time.Now().Add(time.Minute).UnixMilli(),
	}
	lease, err := Acquire(base, "default")
	if err != nil {
		t.Fatal(err)
	}
	_, err = state.ClaimSwitch(room, gone, func(state.SwitchMark) bool { return true })
	lease.Release()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	lease, err = Acquire(base, "default")
	if err != nil {
		t.Fatalf("a writer beside a dropping mark whose leader is gone: %v", err)
	}
	lease.Release()
	if waited := time.Since(started); waited >= time.Second {
		t.Fatalf("the writer waited %s for a leader that is gone", waited)
	}
	actAs(t, "build C")
	_, err = AcquireLaunch(base, "default")
	var refused *RefusedError
	if !errors.As(err, &refused) || refused.Switching == nil {
		t.Fatalf("a launch of another build under the mark: %v", err)
	}
}

// The commit writes, in order, the committed mark, the build, then the room's
// copy, and removes the copies no mark names: a crash after any step leaves
// a live mark and one build.
func TestTheCommitWritesTheBuildThenTheCopy(t *testing.T) {
	base := newBase(t)
	room := launched(t, base, "default")
	own := strings.TrimSpace(record(t, room))
	path, target := candidate(t, "build B")
	older := state.StagedPath(room, strings.Repeat("c", 64))
	if err := os.WriteFile(older, []byte("an earlier attempt's"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, sw := leading(t, base, "default", target)
	if err := sw.Stage(path); err != nil {
		t.Fatal(err)
	}
	if err := sw.Phase(state.PhaseDropping, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := sw.Exclusive(time.Second); err != nil {
		t.Fatal(err)
	}
	name := map[string]string{own: "own", target: "target"}
	var steps []string
	was := state.Fault
	t.Cleanup(func() { state.Fault = was })
	state.Fault = func(op, step string) error {
		if op == state.OpStep && strings.HasPrefix(step, "switch ") {
			steps = append(steps, step+": "+phaseOf(t, room)+", build "+name[strings.TrimSpace(record(t, room))]+", copy "+name[copyID(t, room)])
		}
		return nil
	}
	if err := sw.Commit(time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"switch marked: committed, build own, copy own",
		"switch build: committed, build target, copy own",
		"switch linked: committed, build target, copy target",
	}
	if strings.Join(steps, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the commit's steps:\n%s", strings.Join(steps, "\n"))
	}
	if _, err := os.Stat(older); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an earlier attempt's copy stays: %v", err)
	}
	if _, err := os.Stat(state.StagedPath(room, target)); err != nil {
		t.Fatalf("the staged copy is gone: %v", err)
	}
}

package build

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A launch of another build never switches a room a live switch holds, at any
// phase — refused naming the switch, or as busy while it drops — and neither
// does one that took the lock exclusive just as the switch began to drop.
func TestALaunchNeverSwitchesUnderALiveMark(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	before := record(t, room)
	wait := shareWait
	shareWait = 300 * time.Millisecond
	t.Cleanup(func() { shareWait = wait })
	actAs(t, "another build")
	for _, phase := range []string{state.PhaseAsking, state.PhaseDropping, state.PhaseCommitted, state.PhaseAborting} {
		leader := startHolder(t, "marked", base, "default", phase)
		if !leader.held() {
			t.Fatalf("%s: the leader: %s", phase, leader.answer)
		}
		lease, err := AcquireLaunch(base, "default")
		refused := refusal(t, lease, err, "a launch under a "+phase+" switch")
		if refused.Switching == nil && (phase != state.PhaseDropping || !refused.Busy) {
			t.Fatalf("%s: refused as %v", phase, refused)
		}
		if got := record(t, room); got != before {
			t.Fatalf("%s: a launch switched the room to %q", phase, got)
		}
		leader.release()
		if err := os.Remove(state.SwitchMarkPath(room)); err != nil {
			t.Fatal(err)
		}
	}
	// The switch drops between the launch's read of the room and its
	// exclusive lock.
	was := state.Fault
	t.Cleanup(func() { state.Fault = was })
	var leader *holder
	state.Fault = func(op, step string) error {
		if op == state.OpStep && step == "lease switch" && leader == nil {
			leader = startHolder(t, "marked", base, "default", state.PhaseDropping)
		}
		return nil
	}
	lease, err := AcquireLaunch(base, "default")
	refused := refusal(t, lease, err, "a launch that took the lock exclusive as the switch dropped")
	if leader == nil || !leader.held() || refused.Switching == nil {
		t.Fatalf("refused as %v, the leader %v", refused, leader)
	}
	if got := record(t, room); got != before {
		t.Fatalf("a launch switched the room to %q", got)
	}
}

// An image that adopts a run takes the room's lease only as its build's
// writer: in a room of another build it is refused and changes nothing, where
// a launch would have switched it.
func TestAdoptionOnlyFollows(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	before, files := record(t, room), tree(t, room)
	actAs(t, "another build")
	lease, err := Follow(base, "default")
	_ = refusal(t, lease, err, "an adoption in a room of another build")
	if got := record(t, room); got != before {
		t.Fatalf("an adoption switched the room to %q", got)
	}
	if got := tree(t, room); got != files {
		t.Fatalf("an adoption changed the room:\n%s\nwas\n%s", got, files)
	}
}

// A leader killed after the build and before the copy leaves the room's copy
// another build's: the first image that follows the build puts it in line.
func TestFollowPutsTheCopyInLineWithTheBuild(t *testing.T) {
	base := newBase(t)
	room := launched(t, base, "default")
	own := strings.TrimSpace(record(t, room))
	bin := filepath.Join(room, "bin", "rewake")
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("the build before the switch"), 0o700); err != nil {
		t.Fatal(err)
	}
	lease, err := Follow(base, "default")
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	if got := copyID(t, room); got != own {
		t.Fatalf("the room's copy is %s, its build %s", got, own)
	}
}

// A second attempt is refused before it stages anything: the first attempt's
// mark and candidate stay as they were.
func TestASecondAttemptIsRefusedBeforeItStages(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	path, target := candidate(t, "build B")
	_, first := leading(t, base, "default", target)
	if err := first.Stage(path); err != nil {
		t.Fatal(err)
	}
	mark, staged := read(t, state.SwitchMarkPath(room)), read(t, state.StagedPath(room, target))
	_, other := candidate(t, "build C")
	lease, err := Acquire(base, "default")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	_, err = Claim(lease, other, "", time.Now().Add(time.Minute))
	var claimed *ClaimedError
	if !errors.As(err, &claimed) || claimed.Mark.Target != target {
		t.Fatalf("a second attempt: %v", err)
	}
	if read(t, state.SwitchMarkPath(room)) != mark || read(t, state.StagedPath(room, target)) != staged {
		t.Fatal("a refused attempt changed the first one's mark or candidate")
	}
	if _, err := os.Stat(state.StagedPath(room, other)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused attempt staged its candidate: %v", err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

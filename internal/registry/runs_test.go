package registry

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/proc"
)

const (
	thisBoot  = "0f0e0d0c-0b0a-4908-8706-050403020100"
	otherBoot = "1f0e0d0c-0b0a-4908-8706-050403020100"
)

// runWorld describes the boot, the namespace and which wrappers live: 5 at
// start 50 lives, anything else has ended, and 9 cannot be told.
func runWorld(t *testing.T) string {
	t.Helper()
	dir := stateDir(t, map[int]uint64{5: 50})
	previousBoot, previousObserve, previousNamespace := currentBoot, observe, namespace
	currentBoot = func() (string, error) { return thisBoot, nil }
	observe = func(pid int, start uint64) proc.IdentityEvidence {
		switch {
		case pid == 9:
			return proc.IdentityUnknown
		case pid == 5 && start == 50:
			return proc.IdentityAlive
		}
		return proc.IdentityEnded
	}
	namespace = func() string { return "pid:[1]" }
	t.Cleanup(func() { currentBoot, observe, namespace = previousBoot, previousObserve, previousNamespace })
	return dir
}

func recordRun(t *testing.T, dir, name string, pid int, start uint64, boot, ns string) string {
	t.Helper()
	epoch := RunEpoch(pid, start, boot)
	if err := WriteRunRecord(dir, RunRecord{Name: name, Boot: boot, Epoch: epoch, Build: BuildStamp, PIDNamespace: ns}); err != nil {
		t.Fatal(err)
	}
	return epoch
}

func successorIs(t *testing.T, dir string, want SuccessorState) {
	t.Helper()
	if got, _, err := Successor(dir, "api"); got != want {
		t.Fatalf("successor %v, want %v (%v)", got, want, err)
	}
}

// The successor is bound once for good, and a sender's barrier tells its
// states apart: none, starting between binding and publishing, ready once its
// record names it, gone only when its wrapper has ended or its boot is past
// (docs/protocol-cutover.md#the-launch).
func TestTheSuccessorGoesThroughItsStates(t *testing.T) {
	dir := runWorld(t)
	successorIs(t, dir, SuccessorNone)
	epoch := recordRun(t, dir, "api", 5, 50, thisBoot, "pid:[1]")
	if bound, err := BindSuccessor(dir, "api", epoch); err != nil || !bound {
		t.Fatalf("bind: %v %v", bound, err)
	}
	later := recordRun(t, dir, "api", 6, 60, thisBoot, "pid:[1]")
	if bound, err := BindSuccessor(dir, "api", later); err != nil || bound {
		t.Fatalf("a later run replaced the successor: %v %v", bound, err)
	}
	// Bound, alive, no session record yet: a missing record is never taken
	// for an ended run.
	successorIs(t, dir, SuccessorStarting)
	session := Session{Name: "api", ServicePID: 5, ServiceStart: 50, Boot: thisBoot, PIDNamespace: "pid:[1]"}
	if err := Publish(dir, session); err != nil {
		t.Fatal(err)
	}
	successorIs(t, dir, SuccessorReady)
	// The wrapper ends: gone for good, even once the name is taken again.
	observe = func(int, uint64) proc.IdentityEvidence { return proc.IdentityEnded }
	successorIs(t, dir, SuccessorGone)
}

// What cannot be read stays unknown: the successor record, its run record,
// its wrapper, or a namespace this process cannot judge. Another boot is
// gone without reading anything.
func TestAnUnreadableSuccessorIsUnknown(t *testing.T) {
	for _, c := range []struct {
		name  string
		pid   int
		boot  string
		ns    string
		spoil func(dir string)
		want  SuccessorState
	}{
		{"its wrapper cannot be told", 9, thisBoot, "pid:[1]", nil, SuccessorUnknown},
		{"another namespace", 5, thisBoot, "pid:[2]", nil, SuccessorUnknown},
		{"the successor record", 5, thisBoot, "pid:[1]", func(dir string) { _ = os.WriteFile(successorPath(dir, "api"), []byte("{"), 0o600) }, SuccessorUnknown},
		{"its run record", 5, thisBoot, "pid:[1]", func(dir string) { _ = os.Remove(runRecordPath(dir, "api", thisBoot, RunEpoch(5, 50, thisBoot))) }, SuccessorUnknown},
		{"another boot", 5, otherBoot, "pid:[1]", nil, SuccessorGone},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := runWorld(t)
			epoch := recordRun(t, dir, "api", c.pid, 50, c.boot, c.ns)
			if _, err := BindSuccessor(dir, "api", epoch); err != nil {
				t.Fatal(err)
			}
			if c.spoil != nil {
				c.spoil(dir)
			}
			successorIs(t, dir, c.want)
		})
	}
}

// Only a run of this build is recorded or bound, and an epoch without a boot
// is never looked up among run records, where after a restart it could match
// another run's.
func TestOnlyARunOfThisBuildIsRecorded(t *testing.T) {
	dir := runWorld(t)
	if err := WriteRunRecord(dir, RunRecord{Name: "api", Epoch: "5.50"}); err == nil {
		t.Error("a run of the earlier build was recorded")
	}
	if _, err := BindSuccessor(dir, "api", "5.50"); err == nil {
		t.Error("a run of the earlier build was bound")
	}
	if _, found, err := ReadRunRecord(dir, "api", "5.50"); found || err != nil {
		t.Errorf("an earlier-build epoch was looked up: %v %v", found, err)
	}
	if _, _, err := ReadRunRecord(dir, "api", RunEpoch(5, 50, thisBoot)); err == nil {
		t.Error("a missing run record of this build read as the earlier build's")
	}
}

// A successor record that cannot be read is not taken for a binding: a
// later run is refused rather than told the name is bound, and the record is
// left as it is, since the first successor is never replaced.
func TestAnUnreadableSuccessorRefusesTheBinding(t *testing.T) {
	dir := runWorld(t)
	epoch := recordRun(t, dir, "api", 5, 50, thisBoot, "pid:[1]")
	if err := os.MkdirAll(runsPath(dir, "api"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(successorPath(dir, "api"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if bound, err := BindSuccessor(dir, "api", epoch); err == nil || bound {
		t.Fatalf("bound over an unreadable successor: %v %v", bound, err)
	}
	if raw, _ := os.ReadFile(successorPath(dir, "api")); string(raw) != "{" {
		t.Fatalf("the record was written over: %q", raw)
	}
}

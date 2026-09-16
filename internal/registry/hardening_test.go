package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A session name becomes a file path. Without a check, "../../victim" reads —
// and, through Lookup's cleanup of dead records, deletes — a file outside the
// state directory that has nothing to do with rewake.
func TestTraversalNameTouchesNothing(t *testing.T) {
	dir := stateDir(t, map[int]uint64{})

	// Records live in <dir>/sessions, so "../victim" lands in <dir> and
	// "../../victim" one level above it. Both are written here: a test whose
	// victim sits anywhere else proves nothing, because the name it passes never
	// pointed at the file it checks.
	victims := []string{
		filepath.Join(dir, "victim.json"),
		filepath.Join(filepath.Dir(dir), "victim.json"),
	}
	for _, victim := range victims {
		if err := os.WriteFile(victim, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	for _, name := range []string{"../victim", "../../victim", "sub/dir", "../../etc/passwd"} {
		if _, err := Lookup(dir, name); err == nil {
			t.Errorf("Lookup(%q) found something", name)
		}
	}
	for _, victim := range victims {
		if _, err := os.Stat(victim); err != nil {
			t.Fatalf("a file outside the sessions directory was deleted: %v", err)
		}
	}
}

// Two claimants racing to take over the name of a session that has ended must
// not both succeed: the loser would serve the winner's mailbox and delete its
// record on the way out.
func TestTakeoverOfADeadNameHasOneWinner(t *testing.T) {
	living := map[int]uint64{}
	dir := stateDir(t, living)

	if err := Publish(dir, session("api", 10, 100)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// The owner is gone; two wrappers now try to take the name at once.
	living[11], living[12] = 110, 120

	results := make(chan error, 2)
	start := make(chan struct{})
	for _, pid := range []int{11, 12} {
		go func() {
			<-start
			results <- Publish(dir, session("api", pid, living[pid]))
		}()
	}
	close(start)

	failures := 0
	for range 2 {
		if err := <-results; err != nil {
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("%d of 2 takeovers failed, want exactly 1", failures)
	}

	// The winner's record is the one on disk, and it is intact.
	loaded, err := Lookup(dir, "api")
	if err != nil {
		t.Fatalf("lookup after the race: %v", err)
	}
	if loaded.ServicePID != 11 && loaded.ServicePID != 12 {
		t.Errorf("record = %+v, want one of the two claimants", loaded)
	}
}

// The mailbox is served by the wrapper, and the harness is what the message is
// for. If either is gone the session cannot be reached, whatever the other does.
func TestSessionNeedsBothProcesses(t *testing.T) {
	living := map[int]uint64{10: 100, 20: 200}
	dir := stateDir(t, living)

	record := session("api", 10, 100)
	record.HarnessPID, record.HarnessStart = 20, 200
	if err := Publish(dir, record); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := Lookup(dir, "api"); err != nil {
		t.Fatalf("a healthy session was not found: %v", err)
	}

	delete(living, 20) // the harness exited, the wrapper is still there
	if _, err := Lookup(dir, "api"); err == nil {
		t.Error("a session whose harness is gone was reported reachable")
	}

	// And the other way round: a wrapper that died leaves the harness with
	// nobody to deliver for it, which is just as unreachable.
	living[20] = 200
	delete(living, 10)
	record.Name = "web"
	if err := Publish(dir, record); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := Lookup(dir, "web"); err == nil {
		t.Error("a session whose wrapper is gone was reported reachable")
	}
}

func TestEpochChangesWithEveryRun(t *testing.T) {
	first := Session{ServicePID: 10, ServiceStart: 100}
	again := Session{ServicePID: 10, ServiceStart: 200}
	if first.Epoch() == again.Epoch() {
		t.Fatal("a reused pid produces the same epoch, so old mail would reach a new session")
	}

	// Two processes can start within the same clock tick, so the pid belongs in
	// the epoch as well.
	neighbour := Session{ServicePID: 11, ServiceStart: 100}
	if first.Epoch() == neighbour.Epoch() {
		t.Fatal("two processes started in the same tick share an epoch")
	}
}

func TestNameLockFileIsNotASession(t *testing.T) {
	living := map[int]uint64{10: 100}
	dir := stateDir(t, living)

	// Force the locked path: publishing over a dead record.
	if err := Publish(dir, session("api", 11, 110)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	delete(living, 11)
	if err := Publish(dir, session("api", 10, 100)); err != nil {
		t.Fatalf("takeover: %v", err)
	}

	// The lock is a file next to the records; a lock that leaves no file behind
	// is not a lock between processes at all.
	if _, err := os.Stat(filepath.Join(state.SessionsPath(dir), ".api.lock")); err != nil {
		t.Fatalf("no lock file was created: %v", err)
	}

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Name != "api" {
		t.Fatalf("list = %v, want just api: the lock file is not a session", sessions)
	}
}

func TestListIgnoresUnreadableRecords(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100})
	if err := Publish(dir, session("api", 10, 100)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := os.WriteFile(state.SessionPath(dir, "broken"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Name != "api" {
		t.Fatalf("list = %v, want the readable session only", sessions)
	}
}

func TestAgeIsMeasuredFromTheStart(t *testing.T) {
	record := Session{StartedAt: time.Now().Add(-90 * time.Second)}
	if age := record.Age(); age < 80*time.Second || age > 100*time.Second {
		t.Errorf("age = %v, want about 90s", age)
	}
}

// A sandboxed agent — Codex runs its commands in one — sees its own pid
// namespace only, where every other process is missing. Reading a session from
// there must neither report it gone nor delete its record: found by running
// `rewake list` inside the sandbox, which wiped a live session.
func TestReaderInAnotherNamespaceDoesNotJudge(t *testing.T) {
	dir := stateDir(t, map[int]uint64{})

	record := session("web", 10, 100)
	record.PIDNamespace = "pid:[4026531836]"
	if err := Publish(dir, record); err != nil {
		t.Fatalf("publish: %v", err)
	}

	previous := namespace
	namespace = func() string { return "pid:[4026533194]" }
	t.Cleanup(func() { namespace = previous })

	found, err := Lookup(dir, "web")
	if err != nil {
		t.Fatalf("Lookup from another namespace: %v", err)
	}
	if found.Name != "web" {
		t.Errorf("session = %+v, want the published one", found)
	}

	sessions, err := List(dir)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("List = %v (%v), want the session listed", sessions, err)
	}
	if _, err := os.Stat(state.SessionPath(dir, "web")); err != nil {
		t.Fatalf("the record was deleted by a reader that cannot see the processes: %v", err)
	}
}

// In the namespace the pids came from, the same record is judged as before.
func TestReaderInTheSameNamespaceStillPrunes(t *testing.T) {
	dir := stateDir(t, map[int]uint64{})

	record := session("web", 10, 100)
	record.PIDNamespace = "pid:[4026531836]"
	if err := Publish(dir, record); err != nil {
		t.Fatalf("publish: %v", err)
	}

	previous := namespace
	namespace = func() string { return "pid:[4026531836]" }
	t.Cleanup(func() { namespace = previous })

	if _, err := Lookup(dir, "web"); err == nil {
		t.Fatal("a dead session was reported alive in its own namespace")
	}
	if _, err := os.Stat(state.SessionPath(dir, "web")); !os.IsNotExist(err) {
		t.Error("the leftover record was not cleaned up")
	}
}

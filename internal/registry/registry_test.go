package registry

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// stateDir prepares an isolated state directory and describes which processes
// are alive, so no test depends on what happens to be running.
func stateDir(t *testing.T, living map[int]uint64) string {
	t.Helper()
	dir := t.TempDir()
	// The test framework creates its directories group-readable; the state
	// directory refuses that, and rightly so.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv(state.DirEnv, dir)
	resolved, err := state.Dir()
	if err != nil {
		t.Fatalf("state.Dir: %v", err)
	}

	previous := alive
	alive = func(pid int, start uint64) bool {
		recorded, ok := living[pid]
		return ok && recorded == start
	}
	t.Cleanup(func() { alive = previous })
	return resolved
}

func session(name string, pid int, start uint64) Session {
	return Session{
		Name:         name,
		Harness:      "claude",
		ServicePID:   pid,
		ServiceStart: start,
		CWD:          "/tmp",
		StartedAt:    time.Now(),
	}
}

func TestPublishClaimsNameOnce(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100})

	if err := Publish(dir, session("api", 10, 100)); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	err := Publish(dir, session("api", 11, 100))
	if err == nil {
		t.Fatal("second publish took a name held by a live session")
	}

	// Refusing is only half of it: the owner's record has to be intact
	// afterwards, or the refusal came at the price of the session it protected.
	held, loadErr := Lookup(dir, "api")
	if loadErr != nil {
		t.Fatalf("the owner's record did not survive the refusal: %v", loadErr)
	}
	if held.ServicePID != 10 {
		t.Errorf("record = %+v, want the first owner", held)
	}
}

func TestPublishReplacesDeadRecord(t *testing.T) {
	living := map[int]uint64{10: 100}
	dir := stateDir(t, living)

	if err := Publish(dir, session("api", 10, 100)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	delete(living, 10) // the wrapper died, its record is a leftover
	living[11] = 200

	if err := Publish(dir, session("api", 11, 200)); err != nil {
		t.Fatalf("publish over a dead record: %v", err)
	}
	loaded, err := Lookup(dir, "api")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if loaded.ServicePID != 11 {
		t.Errorf("pid = %d, want 11", loaded.ServicePID)
	}
}

// A reused pid is the reason records carry a start time: without it the record
// of a session that ended would describe whatever process took its number.
func TestReusedPidIsNotAlive(t *testing.T) {
	living := map[int]uint64{10: 100}
	dir := stateDir(t, living)

	if err := Publish(dir, session("api", 10, 100)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	living[10] = 999 // same pid, different process

	if _, err := Lookup(dir, "api"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lookup err = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(state.SessionPath(dir, "api")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the stale record was not removed by the reader")
	}
}

func TestConcurrentPublishHasOneWinner(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100, 11: 100})

	var wait sync.WaitGroup
	results := make([]error, 2)
	for index, pid := range []int{10, 11} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index] = Publish(dir, session("api", pid, 100))
		}()
	}
	wait.Wait()

	failures := 0
	for _, err := range results {
		if err != nil {
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("%d of 2 publishes failed, want exactly 1", failures)
	}
}

func TestListSkipsAndPrunesDead(t *testing.T) {
	living := map[int]uint64{10: 100, 12: 120}
	dir := stateDir(t, living)

	if err := Publish(dir, session("api", 10, 100)); err != nil {
		t.Fatalf("publish api: %v", err)
	}
	if err := Publish(dir, session("web", 12, 120)); err != nil {
		t.Fatalf("publish web: %v", err)
	}
	delete(living, 12)

	sessions, err := List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Name != "api" {
		t.Fatalf("list = %v, want only api", sessions)
	}
	if _, err := os.Stat(state.SessionPath(dir, "web")); !errors.Is(err, os.ErrNotExist) {
		t.Error("list left the record of a dead session behind")
	}
}

// Listing never waits for a name lock: with the lock of a dead record held,
// the listing returns at once and leaves that record to the next one.
func TestListDoesNotWaitOnAHeldNameLock(t *testing.T) {
	living := map[int]uint64{10: 100}
	dir := stateDir(t, living)
	if err := Publish(dir, session("web", 10, 100)); err != nil {
		t.Fatalf("publish web: %v", err)
	}
	delete(living, 10)

	held, release, released := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(released)
		_ = state.WithNameLock(dir, "web", func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	done := make(chan error, 1)
	go func() {
		_, err := List(dir)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("list: %v", err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("list waited on a held name lock")
	}
	if _, err := os.Stat(state.SessionPath(dir, "web")); err != nil {
		t.Errorf("the record was removed under somebody else's lock: %v", err)
	}
	close(release)
	// Released, not merely told to release: a listing that ran before the
	// holder let go would find the lock still taken.
	<-released
	if _, err := List(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state.SessionPath(dir, "web")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the next listing did not prune the record once the lock was free")
	}
}

// Lookup does not wait for a name lock either: a dead record is reported as
// no session at once, and pruned by the first lookup that finds the lock free.
func TestLookupDoesNotWaitOnAHeldNameLock(t *testing.T) {
	living := map[int]uint64{10: 100}
	dir := stateDir(t, living)
	if err := Publish(dir, session("web", 10, 100)); err != nil {
		t.Fatalf("publish web: %v", err)
	}
	delete(living, 10)

	held, release, released := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(released)
		_ = state.WithNameLock(dir, "web", func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	done := make(chan error, 1)
	go func() {
		_, err := Lookup(dir, "web")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("lookup of a dead record = %v, want ErrNotFound", err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("lookup waited on a held name lock")
	}
	if _, err := os.Stat(state.SessionPath(dir, "web")); err != nil {
		t.Errorf("the record was removed under somebody else's lock: %v", err)
	}
	close(release)
	<-released
	if _, err := Lookup(dir, "web"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second lookup = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(state.SessionPath(dir, "web")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the next lookup did not prune the record once the lock was free")
	}
}

func TestChooseName(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100})

	name, err := ChooseName(dir, "", "claude")
	if err != nil || name != "claude" {
		t.Fatalf("ChooseName = %q, %v; want claude", name, err)
	}
	if err := Publish(dir, session("claude", 10, 100)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	name, err = ChooseName(dir, "", "claude")
	if err != nil || name != "claude-2" {
		t.Fatalf("ChooseName = %q, %v; want claude-2", name, err)
	}

	// An explicit name is never silently changed: the caller is about to hand
	// that address to somebody else.
	if _, err := ChooseName(dir, "Api Session", "claude"); err == nil {
		t.Error("an unusable explicit name was accepted")
	}
}

func TestUpdateKeepsTheName(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100})
	record := session("api", 10, 100)
	if err := Publish(dir, record); err != nil {
		t.Fatalf("publish: %v", err)
	}

	record.HarnessPID = 42
	if err := Update(dir, record); err != nil {
		t.Fatalf("update: %v", err)
	}
	loaded, err := Load(dir, "api")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.HarnessPID != 42 {
		t.Errorf("harness pid = %d, want 42", loaded.HarnessPID)
	}
	if entries, _ := filepath.Glob(filepath.Join(state.SessionsPath(dir), ".tmp-*")); len(entries) != 0 {
		t.Errorf("update left temporary files behind: %v", entries)
	}
}

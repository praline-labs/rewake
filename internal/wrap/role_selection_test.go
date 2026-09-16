package wrap

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func claimRole(t *testing.T, dir, name string, part role.Role) (registry.Session, error) {
	t.Helper()
	return claimName(Request{Dir: dir, Name: name, Harness: &fakeHarness{}, Role: part}, os.Getpid(), selfStart(t), dir)
}

func TestARoomChoosesOneMainAndHonorsExplicitRoles(t *testing.T) {
	dir := stateDir(t)
	first, err := claimRole(t, dir, "lead", role.Role{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Role != "main" {
		t.Errorf("first role=%q, want main", first.Role)
	}
	second, err := claimRole(t, dir, "helper", role.Role{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Role != "worker" {
		t.Errorf("next role=%q", second.Role)
	}
	writer, err := claimRole(t, dir, "writer", role.Write)
	if err != nil || writer.Role != "write" {
		t.Errorf("writer=%+v err=%v", writer, err)
	}
	if _, err := claimRole(t, dir, "other", role.Main); err == nil || !strings.Contains(err.Error(), "lead") {
		t.Errorf("occupied main refusal=%v", err)
	}
	if _, err := registry.RemoveOwned(dir, "lead", first.Epoch()); err != nil {
		t.Fatal(err)
	}
	replacement, err := claimRole(t, dir, "replacement", role.Role{})
	if err != nil || replacement.Role != "main" {
		t.Errorf("replacement=%+v err=%v", replacement, err)
	}
}

func TestAnExplicitWorkerCanStartBeforeMain(t *testing.T) {
	dir := stateDir(t)
	first, err := claimRole(t, dir, "helper", role.Worker)
	if err != nil || first.Role != "worker" {
		t.Fatalf("worker=%+v err=%v", first, err)
	}
	next, err := claimRole(t, dir, "lead", role.Role{})
	if err != nil || next.Role != "main" {
		t.Errorf("main=%+v err=%v", next, err)
	}
}

func TestConcurrentLaunchesElectOneMain(t *testing.T) {
	dir := stateDir(t)
	start := selfStart(t)
	gate := make(chan struct{})
	results := make(chan registry.Session, 16)
	failures := make(chan error, 16)
	var group sync.WaitGroup
	for i := range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-gate
			session, err := claimName(Request{Dir: dir, Name: string(rune('a' + i)), Harness: &fakeHarness{}}, os.Getpid(), start, dir)
			if err != nil {
				failures <- err
			} else {
				results <- session
			}
		}()
	}
	close(gate)
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	mains := 0
	for session := range results {
		if session.Role == "main" {
			mains++
		}
	}
	if mains != 1 {
		t.Errorf("main count=%d, want one", mains)
	}
}

func TestRoleElectionWaitsForTheRoomLock(t *testing.T) {
	dir := stateDir(t)
	file, err := os.OpenFile(filepath.Join(dir, ".launch.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := claimRole(t, dir, "lead", role.Role{}); done <- err }()
	escaped := false
	select {
	case err := <-done:
		escaped = true
		t.Errorf("claim escaped room lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if !escaped {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("claim stayed blocked")
		}
	}
}

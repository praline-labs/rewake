package spacehost

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// A space whose host was killed stayed closing for good, listed for ever. Its
// host proven gone, main's close ends what runs of the worker, releases the
// checkout and closes the record with a reason that says the host was gone.
func TestASpaceWhoseHostIsGoneIsFinishedByTheClose(t *testing.T) {
	const why = "main closed it with rewake space close"
	dir, id, record, worker, _ := goneSpace(t, why)
	if err := FinishGone(dir, record); err != nil {
		t.Fatal(err)
	}
	closed, err := space.Load(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if closed.State != space.Closed || closed.End.Worker != space.WorkerEnded || !strings.HasPrefix(closed.End.Why, why+"; its space host was gone") {
		t.Fatalf("closed as %s with %+v", closed.State, closed.End)
	}
	if err := syscall.Kill(-worker.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("something of the worker's group is left: %v", err)
	}
}

// A worker's wrapper may be gone while a process it started still runs in its
// group: the checkout is released and the end recorded only once the whole
// group is seen gone, so that process is stopped first.
func TestAGroupLeftByADeadWrapperIsStoppedBeforeTheEndIsRecorded(t *testing.T) {
	dir, id, record, worker, reaped := goneSpace(t, "main closed it with rewake space close")
	_ = worker.Process.Kill()
	<-reaped
	if !proc.GroupAlive(worker.Process.Pid) {
		t.Fatal("setup: nothing of the group survives its wrapper")
	}
	if err := FinishGone(dir, record); err != nil {
		t.Fatal(err)
	}
	if proc.GroupAlive(worker.Process.Pid) {
		closed, _ := space.Load(dir, id)
		t.Fatalf("the space is %s with its worker's group still running", closed.State)
	}
	if closed, err := space.Load(dir, id); err != nil || closed.State != space.Closed {
		t.Fatalf("the space is %+v, %v", closed.State, err)
	}
}

// goneSpace is a space closing with a host that is over and a worker whose
// group runs, as a killed host leaves it.
func goneSpace(t *testing.T, why string) (dir, id string, record space.Record, worker *exec.Cmd, reaped chan struct{}) {
	t.Helper()
	dir = buildtest.Dir(t)
	self, err := selfRun()
	if err != nil {
		t.Fatal(err)
	}
	// The host: a process that started and is over.
	dead := exec.Command("true")
	if err := dead.Start(); err != nil {
		t.Fatal(err)
	}
	hostRun, err := childRun(dead.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := dead.Wait(); err != nil {
		t.Fatal(err)
	}
	worker, reaped = startGroup(t, `sleep 60 & echo ready; wait`)
	workerRun, err := childRun(worker.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	id, err = space.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := space.Create(dir, space.Record{
		ID: id, Name: "api",
		Main:     space.Run{Name: "main-fixture", Run: self, PIDNamespace: proc.Namespace()},
		Worker:   space.Worker{Harness: "fixture", Name: "api", Role: "write"},
		OpenedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := space.SetHost(dir, id, hostRun); err != nil {
		t.Fatal(err)
	}
	if _, err := space.Opened(dir, id, workerRun); err != nil {
		t.Fatal(err)
	}
	record, err = space.BeginClosing(dir, id, why)
	if err != nil {
		t.Fatal(err)
	}
	if got := space.HostRuns(record); got != proc.IdentityEnded {
		t.Fatalf("the dead host reads %v, not ended", got)
	}
	return dir, id, record, worker, reaped
}

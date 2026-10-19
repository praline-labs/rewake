package spacehost

import (
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// rewake space close asks by the record alone: main moves the space to
// closing with its reason, and the space host, reading the record at its
// next tick, ends the worker's process group by its close sequence and closes
// the record with that reason and the signal that took — main signals
// nothing. Main's run is this process, alive throughout, so nothing but the
// closing ends the worker.
func TestMainsCloseEndsTheWorkerThroughTheSpaceHost(t *testing.T) {
	dir := buildtest.Dir(t)
	self, err := selfRun()
	if err != nil {
		t.Fatal(err)
	}
	worker, reaped := startGroup(t, `sleep 60 & echo ready; wait`)
	workerRun, err := childRun(worker.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	id, err := space.NewID()
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
	if _, err := space.SetHost(dir, id, self); err != nil {
		t.Fatal(err)
	}
	record, err := space.Opened(dir, id, workerRun)
	if err != nil {
		t.Fatal(err)
	}

	h := &host{spec: Spec{Dir: dir, ID: id}, record: record}
	done := make(chan error, 1)
	go func() { done <- h.finish(worker.Process.Pid, reaped, h.watch(reaped), stopping) }()
	time.Sleep(1500 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("the space host ended the worker before main closed it: %v", err)
	default:
	}

	const why = "main closed it with rewake space close"
	if _, err := space.BeginClosing(dir, id, why); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the space host did not end the worker after main's close")
	}
	closed, err := space.Load(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if closed.State != space.Closed || closed.End.Worker != space.WorkerEnded || closed.End.Why != why+"; the worker's process group ended after SIGHUP" {
		t.Fatalf("closed as %s with %+v", closed.State, closed.End)
	}
	if err := syscall.Kill(-worker.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("something of the worker's group is left: %v", err)
	}
}

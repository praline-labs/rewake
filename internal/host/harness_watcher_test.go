package host

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
)

// watchingService records the harness process the wrapper names.
type watchingService struct {
	named []adapter.RunProcess
}

var _ adapter.HarnessWatcher = (*watchingService)(nil)

func (*watchingService) Start(context.Context, func(string)) error { return nil }
func (*watchingService) Close()                                    {}
func (s *watchingService) HarnessStarted(p adapter.RunProcess)     { s.named = append(s.named, p) }

type watchedHarness struct {
	fakeHarness
	run *watchingService
}

func (h *watchedHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(request)
	plan.Service = h.run
	return plan, err
}

// A service that checks its requests by their peer learns the harness's
// process from the wrapper, once, with its start time: the process the
// wrapper started, not the wrapper itself.
func TestTheWrapperNamesTheHarnessToAServiceThatWatchesIt(t *testing.T) {
	dir := stateDir(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	run := &watchingService{}
	fake := &watchedHarness{fakeHarness: fakeHarness{script: "echo $$ > " + pidFile}, run: run}
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(written)))
	if len(run.named) != 1 || run.named[0].PID != pid || run.named[0].Start == 0 {
		t.Fatalf("the service was named %+v, the harness was process %d", run.named, pid)
	}
}

// countingService admits one piece of work when it starts and ends it a while
// later, as a handler still writing at the service's close would.
type countingService struct {
	workers   adapter.Workers
	admitted  bool
	dismissed time.Time
	done      chan struct{}
}

var _ adapter.WorkerCounter = (*countingService)(nil)

func (s *countingService) CountWorkers(w adapter.Workers) { s.workers = w }
func (*countingService) Close()                           {}

func (s *countingService) Start(context.Context, func(string)) error {
	s.admitted = s.workers != nil && s.workers.Admit()
	if s.admitted {
		go func() {
			time.Sleep(300 * time.Millisecond)
			s.dismissed = time.Now()
			s.workers.Dismiss()
			close(s.done)
		}()
	}
	return nil
}

type countedHarness struct {
	fakeHarness
	run *countingService
}

func (h *countedHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(request)
	plan.Service = h.run
	return plan, err
}

// A service that counts its handlers gets the run's workers before it starts:
// what it admits holds the run's end until it is dismissed, and once the run
// has ended nothing more is admitted.
func TestTheWrapperCountsAServicesHandlersAmongTheRunsWorkers(t *testing.T) {
	dir := stateDir(t)
	run := &countingService{done: make(chan struct{})}
	fake := &countedHarness{fakeHarness: fakeHarness{script: "true"}, run: run}
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatal(err)
	}
	ended := time.Now()
	if !run.admitted {
		t.Fatal("the service was not handed the run's workers, or they admitted nothing while it served")
	}
	select {
	case <-run.done:
	default:
		t.Fatal("the run ended while a piece of work it admitted was still going")
	}
	if ended.Before(run.dismissed) {
		t.Fatalf("the run ended at %v, before its last worker at %v", ended, run.dismissed)
	}
	if run.workers.Admit() {
		run.workers.Dismiss()
		t.Fatal("the run's workers admitted more after the run ended")
	}
}

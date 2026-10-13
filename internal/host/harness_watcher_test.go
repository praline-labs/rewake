package host

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

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

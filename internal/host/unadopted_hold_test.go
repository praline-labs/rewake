package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// failingAdoption plans a service whose adoption fails once released, after
// reporting what its run proved: a service adopted in part.
type failingAdoption struct {
	*fakeHarness
	entered, release chan struct{}
	report           func([]adapter.Capability)
}

func (f *failingAdoption) Launch(adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	return adapter.LaunchPlan{Service: f, Live: f}, nil
}

func (f *failingAdoption) ReportLive(report func([]adapter.Capability)) { f.report = report }
func (f *failingAdoption) Start(context.Context, func(string)) error    { return nil }
func (f *failingAdoption) Busy() string                                 { return "" }
func (f *failingAdoption) HandOver() (adapter.HandedOver, error)        { return adapter.HandedOver{}, nil }

func (f *failingAdoption) Adopt(context.Context, func(string), adapter.HandedOver) error {
	f.report([]adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability})
	close(f.entered)
	<-f.release
	return errors.New("the service could not be adopted")
}

// Close reports once more, as a service torn down in part might.
func (f *failingAdoption) Close() { f.report([]adapter.Capability{adapter.WakeCapability}) }

// holdRun claims a worker whose record has its boundary live, as the earlier
// image left it, and runs its adoption by harness against a sleeping child,
// which the test's end kills. It answers the room, the worker and Run's end.
func holdRun(t *testing.T, harness adapter.Harness) (string, registry.Session, chan struct{}) {
	t.Helper()
	dir := stateDir(t)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	worker, err := claimName(Request{Harness: harness, Dir: dir, Name: "api", Role: role.Write}, os.Getpid(), start, mustBoot(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	worker.Live = []string{string(adapter.WakeCapability), string(adapter.TurnBoundaryCapability)}
	if err := registry.Update(dir, worker); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	childStart, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	handover := &Handover{
		Version: HandoverVersion, Harness: harness.ID(), Dir: dir, Name: worker.Name, Epoch: worker.Epoch(),
		Role: role.Write.ID, HarnessPID: child.Process.Pid, HarnessStart: childStart,
	}
	done := make(chan struct{})
	go func() {
		_, _ = Run(context.Background(), handover.Request(harness))
		close(done)
	}()
	t.Cleanup(func() {
		_ = child.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the run did not end with its harness")
		}
	})
	return dir, worker, done
}

// liveOf waits up to a few seconds for the worker's record to advertise
// nothing, and answers what it advertises then.
func liveOf(t *testing.T, dir, name string) []string {
	t.Helper()
	var got registry.Session
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		var err error
		if got, err = registry.Load(dir, name); err != nil {
			t.Fatal(err)
		}
		if len(got.Live) == 0 {
			break
		}
	}
	return got.Live
}

// A held run serves nothing, so its record advertises nothing: a sender's
// task admitted on the earlier image's boundary would wait for a report no
// turn's end can send (found in review, November 3, 2026). Both when the plan
// is refused, before anything is served, and when the service published in
// part and failed, its later report not published again.
func TestAHeldRunWithdrawsItsLiveSet(t *testing.T) {
	t.Run("the plan refused", func(t *testing.T) {
		dir, worker, done := holdRun(t, refusingAdoption{&fakeHarness{}})
		if live := liveOf(t, dir, worker.Name); len(live) > 0 {
			t.Fatalf("the held run still advertises %v", live)
		}
		assertHolding(t, done)
	})
	t.Run("the service adopted in part", func(t *testing.T) {
		harness := &failingAdoption{fakeHarness: &fakeHarness{}, entered: make(chan struct{}), release: make(chan struct{})}
		dir, worker, done := holdRun(t, harness)
		<-harness.entered
		close(harness.release)
		if live := liveOf(t, dir, worker.Name); len(live) > 0 {
			t.Fatalf("the held run still advertises %v", live)
		}
		time.Sleep(100 * time.Millisecond)
		if got, _ := registry.Load(dir, worker.Name); len(got.Live) > 0 {
			t.Fatalf("a later report of the service advertised %v again", got.Live)
		}
		assertHolding(t, done)
	})
}

// A SIGTERM the wrapper caught while its service's adoption was under way
// reaches the harness the run then holds: the run had taken the catch, and
// the hold taking a second one left it on a channel nobody read (found in
// review, November 3, 2026).
func TestAHeldRunForwardsASignalCaughtBeforeItsFailure(t *testing.T) {
	// As rewake adopt does before it reads the handover.
	CatchSignals()
	t.Cleanup(func() {
		if caught.restore != nil {
			caught.restore()
			caught.incoming, caught.restore = nil, nil
		}
	})
	harness := &failingAdoption{fakeHarness: &fakeHarness{}, entered: make(chan struct{}), release: make(chan struct{})}
	_, _, done := holdRun(t, harness)
	<-harness.entered
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	close(harness.release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the SIGTERM caught during the adoption never reached the held harness")
	}
}

// assertHolding says Run still holds its harness.
func assertHolding(t *testing.T, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
		t.Fatal("the run returned under its harness")
	default:
	}
}

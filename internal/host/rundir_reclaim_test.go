package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The reclaiming of a run's directories by its epoch: a run's own cleanup
// leaves a successor's alone, and a later launch takes what a killed run left
// only once that run is proven dead.

// A run's cleanup removes its own directories by its epoch: the next owner of
// the name, another run, keeps its run and control directories.
func TestOldRunCleanupPreservesNewOwner(t *testing.T) {
	dir := stateDir(t)
	successor := liveEpoch(t, startSleeper(t))
	var next []string
	harness := &runDirHarness{fakeHarness: &fakeHarness{script: "exit 0"}}
	_, err := Run(context.Background(), Request{
		Harness: harness, Dir: dir, Name: "api",
		OnClaimed: func(session registry.Session) error {
			// The name changes hands while the run is on: the same name,
			// another run.
			next = []string{state.RunPath(dir, session.Name, successor), state.ControlPath(dir, session.Name, successor)}
			for _, path := range next {
				if err := os.MkdirAll(path, 0o700); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Lstat(harness.given); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the old run's directory stayed: %v", err)
	}
	for _, path := range next {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the old run's cleanup removed the new owner's %s: %v", path, err)
		}
	}
}

// A wrapper killed outright leaves its directories; a later launch removes
// them once that wrapper is proven dead, and never before. A run alive, or one
// that cannot be told, keeps what it has, and so does an entry naming no run.
func TestCrashedRunIsReclaimedOnlyAfterProvenDeath(t *testing.T) {
	dir := stateDir(t)
	crashed := startCrashingWrapper(t, dir)
	crashedDirs := []string{
		state.RunPath(dir, crashed.Name, crashed.Epoch()),
		state.ControlPath(dir, crashed.Name, crashed.Epoch()),
	}
	if err := os.MkdirAll(crashedDirs[1], 0o700); err != nil {
		t.Fatal(err)
	}

	alive := liveEpoch(t, startSleeper(t))
	unknown := registry.RunEpoch(4194301, 77, mustBoot(t))
	elsewhere := registry.RunEpoch(4194302, 78, "00000000-0000-4000-8000-000000000000")
	kept := []string{
		state.RunPath(dir, "alive", alive),
		state.ControlPath(dir, "alive", alive),
		state.RunPath(dir, "unknown", unknown),
		filepath.Join(state.RunsPath(dir), "no.run.here"),
		filepath.Join(state.RunsPath(dir), crashed.Name+"."+registry.RunEpoch(1, 1, "")),
	}
	gone := append([]string{state.RunPath(dir, "elsewhere.with.dots", elsewhere)}, crashedDirs...)
	for _, path := range append(append([]string{}, kept...), gone[:1]...) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	file := state.RunPath(dir, "file", elsewhere)
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	kept = append(kept, file)
	before := observeRun
	t.Cleanup(func() { observeRun = before })
	observeRun = func(epoch string) proc.IdentityEvidence {
		if epoch == unknown {
			return proc.IdentityUnknown
		}
		return before(epoch)
	}

	launch := func() {
		t.Helper()
		if _, err := Run(context.Background(), Request{Harness: &fakeHarness{script: "exit 0"}, Dir: dir, Name: "next"}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	// While the crashed run is still running, a launch takes nothing of it.
	launch()
	for _, path := range crashedDirs {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("a launch removed %s of a run still running: %v", path, err)
		}
	}
	crashed.kill()
	for _, path := range crashedDirs {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the killed wrapper removed %s: the test killed nothing outright", path)
		}
	}
	launch()
	for _, path := range gone {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s of a run proven dead stayed: %v", path, err)
		}
	}
	for _, path := range kept {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s was removed: %v", path, err)
		}
	}
}

// liveEpoch names a run whose wrapper is the given process, this one by default.
func liveEpoch(t *testing.T, pid ...int) string {
	t.Helper()
	process := os.Getpid()
	if len(pid) > 0 {
		process = pid[0]
	}
	start, err := proc.StartTime(process)
	if err != nil {
		t.Fatal(err)
	}
	return registry.RunEpoch(process, start, mustBoot(t))
}

func mustBoot(t *testing.T) string {
	t.Helper()
	boot, err := registry.CurrentBoot()
	if err != nil {
		t.Fatal(err)
	}
	return boot
}

// startSleeper starts a process that outlives the test's launches.
func startSleeper(t *testing.T) int {
	t.Helper()
	sleeper := exec.Command("sleep", "30")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sleeper.Process.Kill(); _ = sleeper.Wait() })
	return sleeper.Process.Pid
}

const crashingWrapperEnv = "REWAKE_TEST_CRASHING_WRAPPER"

// TestCrashingWrapperProcess is the wrapper startCrashingWrapper kills: a
// separate process running a launch until it is killed. Skipped otherwise.
func TestCrashingWrapperProcess(t *testing.T) {
	if os.Getenv(crashingWrapperEnv) == "" {
		t.Skip("the wrapper a crash test starts and kills")
	}
	_, _ = Run(context.Background(), Request{Harness: &fakeHarness{script: "sleep 60"}, Dir: os.Getenv(crashingWrapperEnv), Name: "crashed"})
}

type crashingWrapper struct {
	registry.Session
	kill func()
}

// startCrashingWrapper starts a wrapper in its own process and process group,
// waits for its run directory, and returns its session with what kills it
// with SIGKILL, harness and all, and reaps it.
func startCrashingWrapper(t *testing.T, dir string) crashingWrapper {
	t.Helper()
	wrapper := exec.Command(os.Args[0], "-test.run=^TestCrashingWrapperProcess$", "-test.timeout=60s")
	wrapper.Env = append(os.Environ(), crashingWrapperEnv+"="+dir)
	wrapper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := wrapper.Start(); err != nil {
		t.Fatal(err)
	}
	killed := false
	kill := func() {
		if killed {
			return
		}
		killed = true
		_ = syscall.Kill(-wrapper.Process.Pid, syscall.SIGKILL)
		_ = wrapper.Wait()
	}
	t.Cleanup(kill)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		session, err := registry.LookupReadOnly(dir, "crashed-fake")
		if err == nil && session.ServicePID == wrapper.Process.Pid && session.HarnessPID != 0 {
			if _, err := os.Stat(state.RunPath(dir, session.Name, session.Epoch())); err == nil {
				return crashingWrapper{Session: session, kill: kill}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the wrapper never ran: pid " + strconv.Itoa(wrapper.Process.Pid))
	return crashingWrapper{}
}

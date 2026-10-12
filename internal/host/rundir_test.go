package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// runDirHarness writes into the run directory it is handed: at launch, from
// its harness process while it runs, and from its service as that closes.
type runDirHarness struct {
	*fakeHarness
	given      string
	mode       os.FileMode
	launchErr  error
	command    string
	closeSaw   []string
	closeWrote error
	// planned says Launch returned a plan; closes counts its service's Close.
	planned  bool
	closes   int
	boundary bool
}

func (r *runDirHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	r.given = request.RunDir
	if !filepath.IsAbs(request.RunDir) {
		return adapter.LaunchPlan{}, errors.New("no run directory to write into")
	}
	if info, err := os.Stat(request.RunDir); err == nil {
		r.mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Join(request.RunDir, "plugin"), 0o700); err != nil {
		return adapter.LaunchPlan{}, err
	}
	if err := os.WriteFile(filepath.Join(request.RunDir, "plugin", "manifest"), []byte("x"), 0o600); err != nil {
		return adapter.LaunchPlan{}, err
	}
	if r.launchErr != nil {
		return adapter.LaunchPlan{}, r.launchErr
	}
	plan, err := r.fakeHarness.Launch(request)
	plan.Args = []string{"-c", r.script, "sh", request.RunDir}
	if r.command != "" {
		plan.Command = r.command
	}
	plan.Service = &runDirService{harness: r, dir: request.RunDir}
	if r.boundary {
		plan.TurnBoundary = &startBoundary{}
	}
	r.planned = err == nil
	return plan, err
}

// runDirService notes what its run directory held when it closed, and writes
// there then: the directory goes only after the service has stopped.
type runDirService struct {
	harness *runDirHarness
	dir     string
}

func (s *runDirService) Start(context.Context, func(string)) error { return nil }

func (s *runDirService) Close() {
	s.harness.closes++
	if !filepath.IsAbs(s.dir) {
		return
	}
	entries, _ := os.ReadDir(s.dir)
	for _, entry := range entries {
		s.harness.closeSaw = append(s.harness.closeSaw, entry.Name())
	}
	s.harness.closeWrote = os.WriteFile(filepath.Join(s.dir, "closed"), []byte("x"), 0o600)
}

// A run's directory is made private before the adapter prepares the launch,
// holds what the adapter, the harness and the service write for the run, and
// goes with the run — here one whose harness ends by a signal.
func TestRunDirectoryDiesWithRun(t *testing.T) {
	dir := stateDir(t)
	harness := &runDirHarness{fakeHarness: &fakeHarness{script: `echo x > "$1/child"; kill -TERM $$; sleep 5`}}
	var session registry.Session
	code, err := Run(context.Background(), Request{
		Harness: harness, Dir: dir, Name: "api",
		OnClaimed: func(claimed registry.Session) error { session = claimed; return nil },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 143 {
		t.Fatalf("code = %d, want the harness's signal", code)
	}
	if want := state.RunPath(dir, session.Name, session.Epoch()); harness.given != want || harness.mode != 0o700 {
		t.Fatalf("launched with %q, mode %v; want %q, 0700", harness.given, harness.mode, want)
	}
	if strings.Join(harness.closeSaw, ",") != "child,plugin" || harness.closeWrote != nil {
		t.Fatalf("the service saw %v and wrote %v as it closed: the harness's or the launch's files were missing", harness.closeSaw, harness.closeWrote)
	}
	if _, err := os.Lstat(harness.given); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the run directory outlived the run: %v", err)
	}
}

// A launch that fails after its directory was made removes that directory and
// nothing else: another live run's, and an entry that names no run, stay. A
// plan the adapter returned is closed once, wherever the launch fails after
// it — the record's update, the read clock, the program — and before the
// directory goes.
func TestFailedLaunchRemovesOnlyItsRunDirectory(t *testing.T) {
	failures := map[string]func(*testing.T, string, *runDirHarness, *Request){
		"the adapter refuses": func(_ *testing.T, _ string, h *runDirHarness, _ *Request) { h.launchErr = errors.New("refused") },
		"the program is missing": func(t *testing.T, _ string, h *runDirHarness, _ *Request) {
			h.command = filepath.Join(t.TempDir(), "no-such-program")
		},
		"the claim's listener refuses": func(_ *testing.T, _ string, _ *runDirHarness, r *Request) {
			r.OnClaimed = func(registry.Session) error { return errors.New("refused") }
		},
		"the record cannot take the plan": func(t *testing.T, dir string, _ *runDirHarness, _ *Request) {
			previous := state.Fault
			t.Cleanup(func() { state.Fault = previous })
			state.Fault = func(op, path string) error {
				if op == state.OpWrite && path == state.SessionPath(dir, "api-fake") {
					return errors.New("the record cannot be written")
				}
				return nil
			}
		},
		"the read clock cannot open": func(_ *testing.T, dir string, h *runDirHarness, r *Request) {
			h.boundary = true
			r.OnClaimed = func(session registry.Session) error {
				if err := os.MkdirAll(state.InboxPath(dir, session.Name), 0o700); err != nil {
					return err
				}
				return os.WriteFile(state.AwaitingPath(dir, session.Name), nil, 0o600)
			}
		},
	}
	for name, fail := range failures {
		t.Run(name, func(t *testing.T) {
			dir := stateDir(t)
			other := state.RunPath(dir, "other", liveEpoch(t))
			stray := filepath.Join(state.RunsPath(dir), "stray")
			for _, keep := range []string{other, stray} {
				if err := os.MkdirAll(keep, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			harness := &runDirHarness{fakeHarness: &fakeHarness{script: "exit 0"}}
			request := Request{Harness: harness, Dir: dir, Name: "api"}
			fail(t, dir, harness, &request)
			var made string
			claimed := request.OnClaimed
			request.OnClaimed = func(session registry.Session) error {
				made = state.RunPath(dir, session.Name, session.Epoch())
				if _, err := os.Stat(made); err != nil {
					t.Errorf("no run directory when the claim was heard: %v", err)
				}
				if claimed != nil {
					return claimed(session)
				}
				return nil
			}
			if _, err := Run(context.Background(), request); err == nil {
				t.Fatal("the launch did not fail")
			}
			if made == "" {
				t.Fatal("the launch failed before its claim")
			}
			if _, err := os.Lstat(made); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the failed launch left its run directory: %v", err)
			}
			if want := map[bool]int{true: 1}[harness.planned]; harness.closes != want {
				t.Fatalf("a plan returned %v was closed %d times, want %d", harness.planned, harness.closes, want)
			}
			if harness.planned && !slices.Contains(harness.closeSaw, "plugin") {
				t.Fatalf("the service closed after its directory went: it saw %v", harness.closeSaw)
			}
			for _, keep := range []string{other, stray} {
				if _, err := os.Stat(keep); err != nil {
					t.Errorf("the failed launch removed %s: %v", keep, err)
				}
			}
		})
	}
}

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

// The sockets of a run keep their short form: at the longest name and room, an
// epoch at its widest and the default base, each fits the limit of a unix
// socket path, which the run directory's own path would not.
func TestRunSocketsFitAtMaximumNames(t *testing.T) {
	longest := strings.Repeat("n", 32)
	room := filepath.Join("/tmp/rewake-4294967294", "v2", "rooms", strings.Repeat("r", 32))
	epoch := registry.RunEpoch(4194304, 18446744073709551615, "ffffffff-ffff-4fff-bfff-ffffffffffff")
	if !state.ValidName(longest) {
		t.Fatal("the longest name is not a name")
	}
	for _, path := range []string{state.SocketPath(room, longest, epoch), state.ContextPath(room, longest, epoch)} {
		if len(path) > 103 {
			t.Errorf("%s is %d bytes, past the 103 a unix socket path allows", path, len(path))
		}
	}
	if run := filepath.Join(state.RunPath(room, longest, epoch), "x.sock"); len(run) <= 103 {
		t.Fatalf("a socket in the run directory would fit (%d bytes): the case does not test the limit", len(run))
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

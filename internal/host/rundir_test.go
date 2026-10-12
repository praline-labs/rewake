package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
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

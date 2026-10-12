//go:build rewakefixture

// Package decisionrig asks a run's wrapper for decisions from real processes
// (docs/v2/stage3-decision.md#authority): sessions of a built rewake whose
// harness is this test binary as the fixture's program, commands it starts
// below them or outside them, and pauses at the named steps of the decision's
// path, so a test orders a wrapper's death or end against a question in flight
// without sharing a process between the two sides.
package decisionrig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/grant/authority"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// binary is cmd/rewake built with the fault seam and the fixture harness;
// program is this test binary, which plays the fixture's program.
var binary, program string

func TestMain(m *testing.M) {
	switch {
	case os.Getenv(askerEnv) != "":
		os.Exit(runAsker(os.Getenv(askerEnv)))
	case os.Getenv(launcherEnv) != "":
		os.Exit(runLauncher(os.Getenv(launcherEnv)))
	case os.Getenv(programEnv) != "":
		os.Exit(runProgram(os.Getenv(programEnv), os.Args[1:]))
	}
	dir, err := os.MkdirTemp("", "decisionrig")
	if err != nil {
		panic(err)
	}
	code := func() int {
		if program, err = os.Executable(); err != nil {
			fmt.Fprintln(os.Stderr, "the test binary cannot be found:", err)
			return 1
		}
		binary = filepath.Join(dir, "rewake")
		compile := exec.Command("go", "build", "-tags", "rewakefault,rewakefixture", "-o", binary, "../../cmd/rewake")
		compile.Stdout, compile.Stderr = os.Stderr, os.Stderr
		if err := compile.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "rewake does not build:", err)
			return 1
		}
		return m.Run()
	}()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// world is one state directory and the sessions a test launches in it.
type world struct {
	t                *testing.T
	base, home, root string
	// reports counts the unknown reports made, each of a sender of its own;
	// launches the sessions, each with a control directory of its own.
	reports, launches int
}

func newWorld(t *testing.T) *world {
	t.Helper()
	// A short directory of its own: a socket path has a bound.
	base, err := os.MkdirTemp("", "dr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("the world is kept for reading at %s", base)
			return
		}
		_ = os.RemoveAll(base)
	})
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, base: base, home: filepath.Join(base, "h"), root: filepath.Join(base, "s")}
	for _, dir := range []string{w.home, w.root} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// room is the default room of the world's v2 root.
func (w *world) room() string { return filepath.Join(w.root, "v2", "rooms", "default") }

// session is one launched rewake: its wrapper, as its run names it, and the
// control directory its program's halves take commands from.
type session struct {
	w           *world
	name, epoch string
	run         authority.Run
	ctl         string
	launch      *exec.Cmd
	exited      chan struct{}
	asked       int
	// harness is the pid of the program's terminal half, the wrapper's
	// foreground.
	harness int
}

// launch starts a session of the role given, with a fault plan for the
// wrapper when one is given, and returns it once its program is up.
func (w *world) launch(name, role, fault string) *session {
	w.t.Helper()
	w.launches++
	s := &session{w: w, ctl: filepath.Join(w.base, fmt.Sprintf("c%d", w.launches)), exited: make(chan struct{})}
	for _, half := range []string{serverHalf, terminalHalf} {
		if err := os.MkdirAll(filepath.Join(s.ctl, half), 0o700); err != nil {
			w.t.Fatal(err)
		}
	}
	log, err := os.Create(s.ctl + ".log")
	if err != nil {
		w.t.Fatal(err)
	}
	s.launch = exec.Command(binary, "--name", name, role, "--command", program, "fixture")
	s.launch.Dir = w.home
	s.launch.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + w.home, "REWAKE_DIR=" + w.root, programEnv + "=" + s.ctl}
	if fault != "" {
		s.launch.Env = append(s.launch.Env, "REWAKE_FAULT="+fault)
	}
	s.launch.Stdout, s.launch.Stderr = log, log
	if err := s.launch.Start(); err != nil {
		w.t.Fatal(err)
	}
	go func() { _ = s.launch.Wait(); _ = log.Close(); close(s.exited) }()
	w.t.Cleanup(s.kill)
	var ready struct {
		Session, Epoch string
		PID            int
	}
	if !waitFor(20*time.Second, func() bool { return readJSON(filepath.Join(s.ctl, terminalHalf, "ready"), &ready) == nil }) {
		w.t.Fatalf("session %s never came up: %s", name, s.log())
	}
	s.name, s.epoch, s.harness = ready.Session, ready.Epoch, ready.PID
	var ok bool
	if s.run, ok = authority.RunOf(s.epoch); !ok {
		w.t.Fatalf("session %s runs as %q, which names no run", name, s.epoch)
	}
	return s
}

func (s *session) log() string {
	raw, _ := os.ReadFile(s.ctl + ".log")
	return string(raw)
}

// address is where the session's wrapper answers.
func (s *session) address() string { return state.AuthorityAddress(s.w.room(), s.epoch) }

// command hands one command to a half of the session's program, and returns
// the file its result will be written to.
func (s *session) command(half string, c command) string {
	s.w.t.Helper()
	s.asked++
	name := fmt.Sprintf("%03d", s.asked)
	c.Out = filepath.Join(s.ctl, half, name+".out")
	raw, err := json.Marshal(c)
	if err != nil {
		s.w.t.Fatal(err)
	}
	staged := filepath.Join(s.ctl, half, "."+name)
	if err := os.WriteFile(staged, raw, 0o600); err != nil {
		s.w.t.Fatal(err)
	}
	if err := os.Rename(staged, filepath.Join(s.ctl, half, name+".cmd")); err != nil {
		s.w.t.Fatal(err)
	}
	return c.Out
}

// do runs a command to its end and returns its result.
func (s *session) do(half string, c command) result {
	s.w.t.Helper()
	return s.w.await(s.command(half, c))
}

// await waits for a result file.
func (w *world) await(out string) result {
	w.t.Helper()
	r, answered := w.awaitWithin(out, 90*time.Second)
	if !answered {
		w.t.Fatalf("no result at %s", out)
	}
	return r
}

// awaitWithin waits for a result file at most d.
func (w *world) awaitWithin(out string, d time.Duration) (result, bool) {
	var r result
	return r, waitFor(d, func() bool { return readJSON(out, &r) == nil })
}

// stop ends the session through its harness and waits for the wrapper.
func (s *session) stop() {
	s.w.t.Helper()
	s.command(terminalHalf, command{Op: opExit})
	s.waitExit()
}

// kill ends the wrapper by SIGKILL, and its program's halves with it.
func (s *session) kill() {
	// Never pid 0 or below: that signals a whole process group.
	if s.run.PID > 0 {
		_ = syscall.Kill(s.run.PID, syscall.SIGKILL)
	}
	if s.harness > 0 {
		_ = syscall.Kill(s.harness, syscall.SIGKILL)
	}
	if s.launch.Process != nil {
		_ = s.launch.Process.Kill()
	}
	<-s.exited
}

func (s *session) waitExit() {
	s.w.t.Helper()
	select {
	case <-s.exited:
	case <-time.After(30 * time.Second):
		s.w.t.Fatalf("session %s did not end: %s", s.name, s.log())
	}
	if !waitFor(10*time.Second, func() bool { return !proc.Alive(s.run.PID, s.run.Start) }) {
		s.w.t.Fatalf("the wrapper of %s outlived its launch", s.name)
	}
}

// pause is a directory a process holds in at a named step until go is made.
type pause string

func newPause(t *testing.T) pause { return pause(t.TempDir()) }

func (p pause) held() bool {
	_, err := os.Stat(filepath.Join(string(p), "held"))
	return err == nil
}

func (p pause) waitHeld(t *testing.T, what string) {
	t.Helper()
	if !waitFor(30*time.Second, p.held) {
		t.Fatalf("%s never paused", what)
	}
}

func (p pause) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(string(p), "go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitFor(limit time.Duration, done func() bool) bool {
	for deadline := time.Now().Add(limit); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if done() {
			return true
		}
	}
	return done()
}

func readJSON(path string, into any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return errors.New("empty")
	}
	return json.Unmarshal(raw, into)
}

func writeJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	staged := path + ".tmp"
	if err := os.WriteFile(staged, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(staged, path)
}

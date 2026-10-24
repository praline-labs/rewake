// Package leaserig races the room's build lease between real processes
// (docs/rules/state.md#the-rooms-lease): two builds of rewake with the
// same version, a byte-identical copy, each paused at a named step by the
// fault seam, so a test orders one process's step against another's without
// sharing a process-global lease between goroutines.
package leaserig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// builtAt is the build time the second build is given: the same tree and
// version, another image.
const builtAt = "2026-01-02T03:04:05Z"

// image is one built rewake: its path, its build id and its version.
type image struct {
	path, id, version string
}

var (
	buildA, buildB image
	// harness is a harness the binaries launch, as their guide names it: a
	// launch here never gets past its lease, so which one does not matter.
	harness string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "leaserig")
	if err != nil {
		panic(err)
	}
	code := func() int {
		if buildA, err = compile(dir, "a"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if buildB, err = compile(dir, "b", "-ldflags", "-X github.com/praline-labs/rewake/internal/cli.built="+builtAt); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if harness, err = firstHarness(buildA.path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return m.Run()
	}()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// compile builds cmd/rewake with the fault seam into dir/<name>/rewake, so
// both builds carry the same executable name.
func compile(dir, name string, flags ...string) (image, error) {
	path := filepath.Join(dir, name, "rewake")
	args := append([]string{"build", "-tags", "rewaketest"}, flags...)
	args = append(args, "-o", path, "../../cmd/rewake")
	build := exec.Command("go", args...)
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		return image{}, fmt.Errorf("build %s does not build: %w", name, err)
	}
	out, err := exec.Command(path, "--version", "--json").Output()
	if err != nil {
		return image{}, fmt.Errorf("build %s --version --json: %w", name, err)
	}
	var version struct {
		Version string `json:"version"`
		ID      string `json:"build"`
	}
	if err := json.Unmarshal(out, &version); err != nil || version.ID == "" {
		return image{}, fmt.Errorf("build %s --version --json: %q, %v", name, out, err)
	}
	return image{path: path, id: version.ID, version: version.Version}, nil
}

func firstHarness(path string) (string, error) {
	out, err := exec.Command(path, "--json").Output()
	if err != nil {
		return "", err
	}
	var guide struct {
		Harnesses []string `json:"harnesses"`
	}
	if err := json.Unmarshal(out, &guide); err != nil || len(guide.Harnesses) == 0 {
		return "", fmt.Errorf("the guide names no harness: %v", err)
	}
	return guide.Harnesses[0], nil
}

// copyOf is a byte-identical copy of an image under another path and inode.
func copyOf(t *testing.T, of image) image {
	t.Helper()
	data, err := os.ReadFile(of.path)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "rewake")
	if err := os.WriteFile(path, data, 0o700); err != nil {
		t.Fatal(err)
	}
	return image{path: path, id: of.id, version: of.version}
}

// world is one state directory the processes of a test share.
type world struct {
	t    *testing.T
	base string
	home string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	return &world{t: t, base: filepath.Join(parent, "state"), home: t.TempDir()}
}

func (w *world) room(name string) string { return filepath.Join(w.base, "v2", "rooms", name) }

// record is the room's build record, or "missing".
func (w *world) record(room string) string {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(w.room(room), "build"))
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if err != nil {
		w.t.Fatal(err)
	}
	return strings.TrimSuffix(string(data), "\n")
}

// process is one rewake the test started.
type process struct {
	t      *testing.T
	cmd    *exec.Cmd
	pause  string
	done   chan struct{}
	stderr strings.Builder
	err    error
}

// start runs an image in the world's room with a fault plan, not waiting.
func (w *world) start(of image, room, fault string, args ...string) *process {
	w.t.Helper()
	p := &process{t: w.t, pause: w.t.TempDir(), done: make(chan struct{})}
	p.cmd = exec.Command(of.path, args...)
	p.cmd.Env = []string{
		"REWAKE_DIR=" + w.base, "REWAKE_ROOM=" + room, "HOME=" + w.home,
		"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8",
		"REWAKE_FAULT=" + strings.ReplaceAll(fault, "@PAUSE", "@"+p.pause),
	}
	p.cmd.Stdout = io.Discard
	p.cmd.Stderr = &p.stderr
	if err := p.cmd.Start(); err != nil {
		w.t.Fatal(err)
	}
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	w.t.Cleanup(p.kill)
	return p
}

// run runs an image to its end and returns its exit code and stderr.
func (w *world) run(of image, room, fault string, args ...string) (int, string) {
	w.t.Helper()
	p := w.start(of, room, fault, args...)
	select {
	case <-p.done:
	case <-time.After(30 * time.Second):
		w.t.Fatalf("%v did not end", args)
	}
	return p.code(), p.stderr.String()
}

// heldAt runs an image until it is paused at the named step and returns it
// there, holding whatever it took before the step.
func (w *world) heldAt(of image, room, step string, args ...string) *process {
	w.t.Helper()
	p := w.start(of, room, "other:holdat="+step+"@PAUSE", args...)
	if !p.waitHeld() {
		w.t.Fatalf("%v ended (%d) before %s: %s", args, p.code(), step, p.stderr.String())
	}
	return p
}

// waitHeld says whether the process paused; false when it ended first.
func (p *process) waitHeld() bool {
	p.t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(p.pause, "held")); err == nil {
			return true
		}
		select {
		case <-p.done:
			return false
		case <-deadline:
			p.t.Fatal("the process neither paused nor ended")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// let lets a paused process go on and waits for its end.
func (p *process) let() {
	p.t.Helper()
	if err := os.WriteFile(filepath.Join(p.pause, "go"), nil, 0o600); err != nil {
		p.t.Fatal(err)
	}
	select {
	case <-p.done:
	case <-time.After(30 * time.Second):
		p.t.Fatal("the process did not end after it was let go")
	}
}

// kill ends the process by SIGKILL and waits for it.
func (p *process) kill() {
	_ = p.cmd.Process.Kill()
	<-p.done
}

func (p *process) pid() int { return p.cmd.Process.Pid }

// code is the exit code, -1 for a process a signal ended.
func (p *process) code() int {
	var exit *exec.ExitError
	if errors.As(p.err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return -1
		}
		return exit.ExitCode()
	}
	if p.err != nil {
		return -2
	}
	return 0
}

// launch is the words of a launch into a room that never gets as far as its
// harness: the test pauses it at its lease or ends it before. The role is
// main, which every harness takes.
func launch(room string) []string {
	return []string{"--main", "--room", room, "--command", "/bin/true", harness}
}

// send is a change form of a session's shell: it takes the room's lease.
func send() []string { return []string{"send", "web", "hello", "--notify", "--wait", "0"} }

func short(id string) string { return id[:12] }

// Package upgraderig runs the clearing of 1.x between real processes
// (docs/v2/stage3-upgrade.md#the-tests): a build of rewake with the fault
// seam, its clearing paused, failed or killed at its steps, against the
// stand-in of 1.x in ./standin, whose sessions and commands are processes of
// their own. The bounds are built short — the clear lock 1 s, a room 300 ms —
// so a wait past them is cheap to show.
package upgraderig

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The bounds this build carries.
const (
	clearWait = time.Second
	roomWait  = 300 * time.Millisecond
)

var (
	rewake, standin string
	// harness is a harness the binary launches, as its guide names it: a
	// launch here is held at its last step before the claim, so which one
	// does not matter.
	harness string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "upgraderig")
	if err != nil {
		panic(err)
	}
	code := func() int {
		rewake, standin = filepath.Join(dir, "rewake"), filepath.Join(dir, "standin")
		const legacy = "github.com/praline-labs/rewake/internal/infra/legacy"
		for _, build := range [][]string{
			{"build", "-tags", "rewakefault", "-ldflags", fmt.Sprintf("-X %s.builtClearWait=%s -X %s.builtRoomWait=%s", legacy, clearWait, legacy, roomWait), "-o", rewake, "../../cmd/rewake"},
			{"build", "-o", standin, "./standin"},
		} {
			cmd := exec.Command("go", build...)
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "build:", err)
				return 1
			}
		}
		if harness, err = firstHarness(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return m.Run()
	}()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func firstHarness() (string, error) {
	out, err := exec.Command(rewake, "--json").Output()
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

// world is one base the processes of a test share, with files beside the
// trees that nothing may touch.
type world struct {
	t    *testing.T
	base string
	home string
	// procRoot is the /proc-shaped tree the world's launches judge wrappers
	// by instead of /proc, when the world sets one.
	procRoot string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, base: filepath.Join(parent, "state"), home: t.TempDir()}
	for _, path := range []string{"notes/kept.txt", "worktrees/x/file"} {
		if err := os.MkdirAll(filepath.Join(w.base, filepath.Dir(path)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(w.base, path), []byte("kept"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// standin runs the stand-in of 1.x on a room of a tree. An op that holds or
// runs is returned once it says it is ready, and killed when the test ends.
func (w *world) standin(op, tree, room string, args ...string) *exec.Cmd {
	w.t.Helper()
	cmd := exec.Command(standin, append([]string{op, tree, room}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	switch op {
	case "room", "letter":
		if err := cmd.Run(); err != nil {
			w.t.Fatalf("standin %s: %v: %s", op, err, stderr.String())
		}
		return cmd
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		w.t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		w.t.Fatal(err)
	}
	w.t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ready := make(chan bool, 1)
	go func() {
		line, _ := bufio.NewReader(out).ReadString('\n')
		ready <- line == "ready\n"
		_, _ = io.Copy(io.Discard, out)
	}()
	select {
	case ok := <-ready:
		if !ok {
			w.t.Fatalf("standin %s %s did not get ready", op, room)
		}
	case <-time.After(20 * time.Second):
		w.t.Fatalf("standin %s %s waits", op, room)
	}
	return cmd
}

// process is one launch of rewake the test started.
type process struct {
	t      *testing.T
	cmd    *exec.Cmd
	pause  string
	done   chan struct{}
	stderr strings.Builder
	err    error
}

// launch starts a launch into room with a fault plan for its role, not
// waiting; "@PAUSE" in the plan names its pause directory.
func (w *world) launch(room, fault string) *process {
	w.t.Helper()
	p := &process{t: w.t, pause: w.t.TempDir(), done: make(chan struct{})}
	p.cmd = exec.Command(rewake, "--main", "--room", room, "--command", "/bin/true", harness)
	p.cmd.Env = []string{
		"REWAKE_DIR=" + w.base, "HOME=" + w.home, "PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8",
		"REWAKE_FAULT=" + strings.ReplaceAll(fault, "@PAUSE", "@"+p.pause),
	}
	if w.procRoot != "" {
		p.cmd.Env = append(p.cmd.Env, "REWAKE_FAULT_PROC="+w.procRoot)
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

// heldAt waits until the process is held at a named step, and answers the
// step; "" when it ended first. Every hold the rig plans is named, so an
// empty held is no answer.
func (p *process) heldAt() string {
	p.t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		if step, err := os.ReadFile(filepath.Join(p.pause, "held")); err == nil && len(step) > 0 {
			return string(step)
		}
		select {
		case <-p.done:
			return ""
		case <-deadline:
			p.t.Fatal("the launch neither paused nor ended")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// resume lets a held process go on and waits until it has left the step.
func (p *process) resume() {
	p.t.Helper()
	gone := filepath.Join(p.pause, "go")
	if err := os.WriteFile(gone, nil, 0o600); err != nil {
		p.t.Fatal(err)
	}
	deadline := time.After(30 * time.Second)
	for {
		if _, err := os.Stat(gone); errors.Is(err, os.ErrNotExist) {
			return
		}
		select {
		case <-p.done:
			return
		case <-deadline:
			p.t.Fatal("the launch did not go on")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// kill ends the process by SIGKILL and waits for it.
func (p *process) kill() {
	_ = p.cmd.Process.Kill()
	<-p.done
}

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

// The ends of a launch, as a test reads them.
const (
	went    = "went"
	refused = "refused"
	killed  = "killed"
)

// end follows a launch to its end, resuming it at every held step but the
// last, "launch cleared", where it is killed: there it went on. It answers
// how it ended and what it said.
func (p *process) end() (string, string) {
	p.t.Helper()
	for {
		step := p.heldAt()
		if step == "launch cleared" {
			p.kill()
			return went, p.stderr.String()
		}
		if step == "" {
			break
		}
		p.resume()
	}
	switch code := p.code(); code {
	case 1:
		return refused, p.stderr.String()
	case -1:
		return killed, p.stderr.String()
	default:
		p.t.Fatalf("the launch ended with %d: %s", code, p.stderr.String())
	}
	return "", ""
}

// trees names the trees of 1.x in the base, in order.
func (w *world) trees() []string {
	w.t.Helper()
	entries, err := os.ReadDir(w.base)
	if err != nil {
		w.t.Fatal(err)
	}
	var kinds []string
	for _, entry := range entries {
		switch name := entry.Name(); {
		case name == "rooms":
			kinds = append(kinds, "rooms")
		case strings.HasPrefix(name, ".rooms-1x-candidate-"):
			kinds = append(kinds, "candidate")
		case strings.HasPrefix(name, ".rooms-1x-validated-"):
			kinds = append(kinds, "validated")
		}
	}
	sort.Strings(kinds)
	return kinds
}

// unrelated hashes what lies beside the trees and v2/: every path, its mode,
// and a file's bytes or a link's target.
func (w *world) unrelated() string {
	w.t.Helper()
	sum := sha256.New()
	err := filepath.WalkDir(w.base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(w.base, path)
		if top, _, _ := strings.Cut(rel, string(filepath.Separator)); top == "rooms" || top == "v2" || strings.HasPrefix(top, ".rooms-1x-") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(sum, "%s %v\n", rel, info.Mode())
		switch {
		case info.Mode().Type() == fs.ModeSymlink:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(sum, "-> %s\n", target)
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum.Write(data)
		}
		return nil
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

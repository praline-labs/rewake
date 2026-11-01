package mainhost

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// Closing the window ends main even when main ignores its hangup and
// SIGTERM: the shell, gone with the window, does not take the host with it
// before the host's escalation reached SIGKILL.
func TestClosingTheWindowEndsAMainThatIgnoresItsHangup(t *testing.T) {
	h := startHost(t, caseClose)
	pid := h.mainPID(t)
	time.Sleep(200 * time.Millisecond)
	_ = h.window.Close()
	h.awaitEnd(t, pid)
}

// Main alone, after a restart whose new shell failed, ends with the window
// too: no shell is there to say the terminal went, and the window need not
// be the host's controlling terminal, so no hangup comes either.
func TestMainAloneEndsWithTheWindow(t *testing.T) {
	h := startHost(t, caseFallback)
	pid := h.mainPID(t)
	if !h.drew("rewake restart shell tries again", 15*time.Second) {
		t.Fatalf("main was never left alone; the window shows %q", h.shown())
	}
	_ = h.window.Close()
	h.awaitEnd(t, pid)
}

// What the old shell hands on reaches the new one whole, though the old one
// exits the moment it has written it: a megabyte of draft, under the line's
// bound, as a person's pasted drafts may make it.
func TestAReleasedStateOfAMegabyteReachesTheNextShell(t *testing.T) {
	for run := range 3 {
		t.Run(strconv.Itoa(run), func(t *testing.T) {
			h := startHost(t, caseHandoff)
			handed := filepath.Join(h.dir, "handed")
			if !waitPath(handed, 15*time.Second) {
				t.Fatalf("the next shell was never given the terminal; the window shows %q", h.shown())
			}
			raw, _ := os.ReadFile(handed)
			if got := string(raw); got != strconv.Itoa(handedSize) {
				t.Fatalf("the next shell was handed %s bytes of draft, want %d", got, handedSize)
			}
		})
	}
}

// testHost is the host run as the person's pane: this binary in the host's
// role, on a terminal the test holds as the window.
type testHost struct {
	dir    string
	window *terminal.Plain
	done   chan struct{}

	mu  sync.Mutex
	out bytes.Buffer
}

// startHost starts the host with the shell's case kase. The window is not
// the host's controlling terminal, as in the workflow scenario: its end
// reaches the host only through its input and output.
func startHost(t *testing.T, kase string) *testHost {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	h := &testHost{dir: t.TempDir(), done: make(chan struct{})}
	h.window, err = terminal.OpenPlain(terminal.Size{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self)
	cmd.Env = append(childEnv(), roleEnv+"=host", caseEnv+"="+kase, dirEnv+"="+h.dir)
	if err := h.window.Prepare(cmd); err != nil {
		t.Fatal(err)
	}
	cmd.SysProcAttr.Setctty = false
	err = cmd.Start()
	_ = h.window.ReleaseSide()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := h.window.Master.Read(buf)
			h.mu.Lock()
			h.out.Write(buf[:n])
			h.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	go func() {
		_ = cmd.Wait()
		close(h.done)
	}()
	t.Cleanup(func() {
		if raw, err := os.ReadFile(filepath.Join(h.dir, "main.pid")); err == nil {
			if pid, err := strconv.Atoi(string(raw)); err == nil {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
		}
		_ = h.window.Close()
	})
	return h
}

// childEnv is the test's environment less every variable naming a session
// or a multiplexer pane, so nothing the host starts reaches a live one.
func childEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "REWAKE_") && !strings.HasPrefix(kv, "HERDR_") && !strings.HasPrefix(kv, "MAINHOST_TEST_") {
			env = append(env, kv)
		}
	}
	return env
}

// mainPID is main's pid, once main runs.
func (h *testHost) mainPID(t *testing.T) int {
	t.Helper()
	path := filepath.Join(h.dir, "main.pid")
	if !waitPath(path, 15*time.Second) {
		t.Fatalf("main never started; the window shows %q", h.shown())
	}
	raw, _ := os.ReadFile(path)
	pid, err := strconv.Atoi(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

// awaitEnd says the host ended within a bound, and main before it.
func (h *testHost) awaitEnd(t *testing.T, pid int) {
	t.Helper()
	select {
	case <-h.done:
	case <-time.After(10 * time.Second):
		t.Fatalf("the host still runs 10 s after its window closed (main alive: %v)", alive(pid))
	}
	if alive(pid) {
		t.Fatalf("main %d outlived its host", pid)
	}
}

// drew waits up to d for the window to show text.
func (h *testHost) drew(text string, d time.Duration) bool {
	for until := time.Now().Add(d); time.Now().Before(until); time.Sleep(20 * time.Millisecond) {
		if strings.Contains(h.shown(), text) {
			return true
		}
	}
	return false
}

func (h *testHost) shown() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.out.String()
}

// alive says pid runs, a zombie not counted.
func alive(pid int) bool {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	_, rest, _ := strings.Cut(string(raw), ") ")
	return !strings.HasPrefix(rest, "Z")
}

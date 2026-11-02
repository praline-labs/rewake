package workflow

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeShellEnv makes this test binary rewake's shell, as main's host runs it
// (docs/shell.md, "The host's channel"), and names the file it records into.
// Set only by the shell's script, so main, which inherits the host's
// environment, never sees it.
const fakeShellEnv = "RW_FAKE_SHELL"

// runFakeShell speaks the host's channel as the real shell does, without a
// screen: hello, ready on go, an attach to main's stream once main is known
// and again a second after that stream ends, as a switch of the room's build
// ends it, released on release, and lost when its terminal's input ends. It
// records each step as "<generation> <step>" for the scenario to read.
func runFakeShell() int {
	record := os.Getenv(fakeShellEnv)
	var mu sync.Mutex
	note := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		if file, err := os.OpenFile(record, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			_, _ = fmt.Fprintf(file, format+"\n", args...)
			_ = file.Close()
		}
	}
	fd, err := strconv.Atoi(os.Getenv("REWAKE_HOST_FD"))
	if err != nil {
		note("0 no channel: %v", err)
		return 2
	}
	conn, err := net.FileConn(os.NewFile(uintptr(fd), "host"))
	if err != nil {
		note("0 no channel: %v", err)
		return 2
	}
	send := func(v any) {
		mu.Lock()
		defer mu.Unlock()
		line, _ := json.Marshal(v)
		_, _ = conn.Write(append(line, '\n'))
	}
	var generation atomic.Int64
	send(map[string]any{"hello": "shell", "v": 1})
	go func() {
		// The window: its input ends when the person closes it.
		_, _ = io.Copy(io.Discard, os.Stdin)
		send(map[string]string{"t": "lost"})
		note("%d lost", generation.Load())
		os.Exit(1)
	}()
	type ref struct {
		PID    int    `json:"pid"`
		Socket string `json:"socket"`
	}
	var main *ref
	// again attaches to the stream of to once more, a second after it ended.
	var again func(to ref) func()
	again = func(to ref) func() {
		return func() {
			for range 20 {
				time.Sleep(time.Second)
				if attachStream(to.Socket, again(to)) == nil {
					note("%d reattached main %d", generation.Load(), to.PID)
					return
				}
			}
			note("%d never reattached main", generation.Load())
		}
	}
	attach := func() {
		if main == nil {
			return
		}
		if err := attachStream(main.Socket, again(*main)); err != nil {
			note("%d attach failed: %v", generation.Load(), err)
			return
		}
		note("%d attached main %d", generation.Load(), main.PID)
	}
	lines := bufio.NewScanner(conn)
	lines.Buffer(make([]byte, 64<<10), 4<<20)
	for lines.Scan() {
		var m struct {
			T          string          `json:"t"`
			Generation int64           `json:"generation"`
			Why        string          `json:"why"`
			Main       *ref            `json:"main"`
			State      json.RawMessage `json:"state"`
			Code       *int            `json:"code"`
		}
		if json.Unmarshal(lines.Bytes(), &m) != nil {
			note("%d unreadable %q", generation.Load(), lines.Text())
			continue
		}
		switch m.T {
		case "welcome":
			generation.Store(m.Generation)
			main = m.Main
			note("%d welcome %s", m.Generation, m.Why)
		case "go":
			note("%d go %s", generation.Load(), m.State)
			send(map[string]any{"t": "ready", "colors": "", "cols": 100, "rows": 30})
			attach()
		case "main":
			main = m.Main
			attach()
		case "release":
			send(map[string]any{"t": "released", "state": map[string]int64{"generation": generation.Load()}})
			note("%d released", generation.Load())
			return 0
		case "closing":
			note("%d closing", generation.Load())
		case "main-ended":
			note("%d main-ended", generation.Load())
			return 0
		default:
			note("%d %s %s", generation.Load(), m.T, m.Why)
		}
	}
	note("%d channel ended", generation.Load())
	return 1
}

// attachStream opens main's raw stream as the shell does — the mode byte,
// the host's acceptance, the preamble — and keeps reading it; ended runs
// once it ends.
func attachStream(socket string, ended func()) error {
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte{'b'}); err != nil {
		return err
	}
	accepted := make([]byte, 1)
	if _, err := io.ReadFull(conn, accepted); err != nil || accepted[0] != 'A' {
		_ = conn.Close()
		return fmt.Errorf("the host answered %q, %v", accepted, err)
	}
	_ = conn.SetDeadline(time.Time{})
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		ended()
	}()
	return nil
}

// fakeShell is the script a launch takes as REWAKE_SHELL to run this test
// binary as the shell, and said, whether the shell's record holds a line.
func fakeShell(t *testing.T, iso *Isolation) (string, func(line string) bool) {
	t.Helper()
	record := filepath.Join(iso.Home, "shell.record")
	shell := filepath.Join(iso.ShimDir, "fake-shell")
	script := "#!/bin/sh\nexec env " + fakeShellEnv + "=" + record + " \"$RW_SHIM_TEST_EXE\" -test.run=TestShimHelper -- \"$@\"\n"
	if err := replaceScript(shell, script); err != nil {
		t.Fatal(err)
	}
	return shell, func(line string) bool {
		raw, _ := os.ReadFile(record)
		return strings.Contains(string(raw), line)
	}
}

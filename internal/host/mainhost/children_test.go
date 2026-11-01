package mainhost

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The tests of the host's run start this binary three times over: as the
// host on a terminal the test holds, as the shell the host starts, and as
// main. roleEnv says which; caseEnv says how the shell behaves, and dirEnv
// where the children leave what the test reads.
const (
	roleEnv = "MAINHOST_TEST_ROLE"
	caseEnv = "MAINHOST_TEST_CASE"
	dirEnv  = "MAINHOST_TEST_DIR"
)

// The shell's cases.
const (
	// caseClose: the shell gets ready and waits; its terminal's end is lost.
	caseClose = "close"
	// caseFallback: the first shell asks a restart, and the next exits once
	// it has the terminal, which leaves main alone.
	caseFallback = "fallback"
	// caseHandoff: the first shell asks a restart and hands handedSize bytes
	// of draft on, exiting at once, as the real shell does; the next writes
	// how much of it came to the file handed.
	caseHandoff = "handoff"
)

const handedSize = 1_000_000

func TestMain(m *testing.M) {
	switch os.Getenv(roleEnv) {
	case "host":
		os.Exit(runTestHost())
	case "shell":
		os.Exit(runTestShell())
	case "main":
		runTestMain()
	}
	os.Exit(m.Run())
}

// runTestHost is the host itself, with its closing's bounds short.
func runTestHost() int {
	hangupWait, termWait = 300*time.Millisecond, 300*time.Millisecond
	self, err := os.Executable()
	if err != nil {
		return 2
	}
	state := filepath.Join(os.Getenv(dirEnv), "state")
	return Run(Config{
		Shell:    self,
		ShellEnv: []string{roleEnv + "=shell"},
		Main:     []string{self},
		MainEnv:  []string{roleEnv + "=main"},
		Base:     state,
		Room:     "test",
		Dir:      filepath.Join(state, "v2", "rooms", "test"),
	})
}

// runTestMain is a main that ignores its hangup and SIGTERM: only SIGKILL,
// the end of the host's escalation, ends it.
func runTestMain() {
	signal.Ignore(syscall.SIGHUP, syscall.SIGTERM)
	writeAtomic(filepath.Join(os.Getenv(dirEnv), "main.pid"), strconv.Itoa(os.Getpid()))
	_, _ = os.Stdout.WriteString("main ready\r\n")
	select {}
}

// runTestShell speaks the host's channel as the real shell does, without a
// screen: hello, ready on go, released on release, and lost once its
// terminal's input ends.
func runTestShell() int {
	dir, kase := os.Getenv(dirEnv), os.Getenv(caseEnv)
	conn, err := net.FileConn(os.NewFile(3, "host"))
	if err != nil {
		return 2
	}
	var mu sync.Mutex
	send := func(v any) {
		mu.Lock()
		defer mu.Unlock()
		line, _ := json.Marshal(v)
		_, _ = conn.Write(append(line, '\n'))
	}
	send(Hello{Hello: clientShell, V: Version})
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		send(Message{T: kindLost})
		os.Exit(1)
	}()
	gen := 0
	l := newLines(conn)
	for {
		line, err := l.next()
		if err != nil {
			return 1
		}
		var m Message
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		switch m.T {
		case kindWelcome:
			gen = m.Generation
		case kindGo:
			if kase == caseFallback && gen > 1 {
				return 1
			}
			if kase == caseHandoff && gen > 1 {
				var state struct {
					Draft string `json:"draft"`
				}
				_ = json.Unmarshal(m.State, &state)
				writeAtomic(filepath.Join(dir, "handed"), strconv.Itoa(len(state.Draft)))
			}
			send(Message{T: kindReady, Cols: 80, Rows: 24})
			if kase != caseClose && gen == 1 {
				go func() {
					waitPath(filepath.Join(dir, "main.pid"), 10*time.Second)
					send(Message{T: kindRestart})
				}()
			}
		case kindRelease:
			state := map[string]string{}
			if kase == caseHandoff {
				state["draft"] = strings.Repeat("x", handedSize)
			}
			raw, _ := json.Marshal(state)
			send(Message{T: kindReleased, State: raw})
			return 0
		case kindMainEnded:
			return 0
		}
	}
}

// writeAtomic writes a file whole, so a reader never sees half of it.
func writeAtomic(path, content string) {
	if os.WriteFile(path+".tmp", []byte(content), 0o600) == nil {
		_ = os.Rename(path+".tmp", path)
	}
}

// waitPath waits up to d for path to exist.
func waitPath(path string, d time.Duration) bool {
	for until := time.Now().Add(d); time.Now().Before(until); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

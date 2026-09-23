package telemetry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// socketDir is short on purpose: a unix socket path has a small limit, and a
// test's temporary directory can exceed it.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "rwtel")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func waitFor(t *testing.T, what string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// An event sent to the collector's path shows up in its snapshot, and closing
// removes the socket.
func TestTheCollectorFoldsWhatIsSent(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	collector := NewCollector(path)
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket %v %v, want mode 0600", info, err)
	}
	Send(path, Event{Kind: SessionStart, Source: "startup", Model: "m"})
	Send(path, Event{Kind: UserPromptSubmit})
	waitFor(t, "the turn", func() bool {
		snapshot := collector.SessionState()
		return snapshot.Activity != nil && *snapshot.Activity == "working"
	})
	if snapshot := collector.SessionState(); *snapshot.Model != "m" || *snapshot.Compactions != 0 {
		t.Errorf("snapshot = %+v", snapshot)
	}
	collector.Close()
	collector.Close()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("the socket survived Close: %v", err)
	}
}

// Garbage on the socket is dropped without stopping the reader.
func TestTheCollectorSurvivesGarbage(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	collector := NewCollector(path)
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	sendRaw(t, path, []byte("not json"))
	sendRaw(t, path, make([]byte, maxDatagram+100))
	Send(path, Event{Kind: Stop})
	waitFor(t, "the event after the garbage", func() bool { return collector.SessionState().Activity != nil })
}

// A socket file left by a wrapper that died is replaced; anything else at the
// path is refused rather than removed.
func TestTheCollectorReplacesOnlyAStaleSocket(t *testing.T) {
	dir := socketDir(t)
	path := filepath.Join(dir, "s.obs")
	first := NewCollector(path)
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Abandoned without Close, as a killed wrapper leaves it.
	second := NewCollector(path)
	if err := second.Start(context.Background()); err != nil {
		t.Fatalf("a stale socket blocked the next run: %v", err)
	}
	second.Close()
	first.Close()

	file := filepath.Join(dir, "plain")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewCollector(file).Start(context.Background()); err == nil {
		t.Error("a regular file at the socket path was replaced")
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("the regular file is gone: %v", err)
	}
}

// Sending never waits: to a path with nobody behind it, and to a collector
// that is not reading, it returns at once.
func TestSendNeverWaits(t *testing.T) {
	dir := socketDir(t)
	start := time.Now()
	Send(filepath.Join(dir, "missing.obs"), Event{Kind: Stop})
	Send("", Event{Kind: Stop})

	path := filepath.Join(dir, "s.obs")
	collector := NewCollector(path)
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	collector.mu.Lock() // the reader stalls on the lock once it has an event
	for range 5000 {
		Send(path, Event{Kind: Stop})
	}
	collector.mu.Unlock()
	collector.Close()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("sending took %v with nobody reading", elapsed)
	}
}

// A canceled context stops the collector like Close does.
func TestTheCollectorStopsWithItsContext(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	ctx, cancel := context.WithCancel(context.Background())
	if err := NewCollector(path).Start(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	waitFor(t, "the socket to go", func() bool {
		_, err := os.Lstat(path)
		return os.IsNotExist(err)
	})
}

func sendRaw(t *testing.T, path string, raw []byte) {
	t.Helper()
	fd, err := socketFor(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSocket(fd)
	_ = sendTo(fd, raw, path)
}

// The boot clock is one clock for every process and only moves forward: a
// process started later reads a larger value than this one did.
func TestTheBootClockOrdersProcesses(t *testing.T) {
	if processStarted == 0 {
		t.Fatal("CLOCK_BOOTTIME could not be read")
	}
	if later := bootClock(); later <= processStarted {
		t.Errorf("boot clock went from %d to %d", processStarted, later)
	}
	out, err := exec.Command(os.Args[0], "-test.run=^TestPrintBootClock$").Output()
	if err != nil {
		t.Fatal(err)
	}
	var child int64
	if _, err := fmt.Sscanf(strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), "%d", &child); err != nil {
		t.Fatalf("the child printed %q", out)
	}
	if child <= processStarted {
		t.Errorf("a process started later read %d, earlier than this one's %d", child, processStarted)
	}
}

// TestPrintBootClock is the child of the test above: it prints the reading
// its process took at start.
func TestPrintBootClock(_ *testing.T) {
	fmt.Println(processStarted)
}

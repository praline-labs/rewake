package terminal

import (
	"errors"
	"os"
	"os/exec"
	"runtime/debug"
	"syscall"
	"testing"
)

// openFailEnv runs the test below in a child, whose descriptor limit it may
// lower without touching the test binary that runs every other test.
const openFailEnv = "REWAKE_TERMINAL_OPEN_FAIL"

// A failed open closes what it opened at once: with one descriptor free, the
// master opens and the side cannot, and the master's slot is free again
// after, not left to a finalizer. The garbage collector is off in the child,
// so nothing but the open itself can return the slot.
func TestAFailedOpenClosesWhatItOpened(t *testing.T) {
	if os.Getenv(openFailEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestAFailedOpenClosesWhatItOpened$")
		cmd.Env = append(os.Environ(), openFailEnv+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	// An open first, so the runtime's poller holds its descriptors before
	// the limit is reached.
	master, side, err := openPair(Size{Cols: 10, Rows: 2})
	if err != nil {
		t.Fatal(err)
	}
	_ = side.Close()
	_ = master.Close()
	debug.SetGCPercent(-1)
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &old); err != nil {
		t.Fatal(err)
	}
	limit := old
	limit.Cur = min(limit.Cur, 128)
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &old) }()
	var held []int
	defer func() {
		for _, fd := range held {
			_ = syscall.Close(fd)
		}
	}()
	for {
		fd, err := syscall.Open("/dev/null", syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
		if errors.Is(err, syscall.EMFILE) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, fd)
	}
	if len(held) == 0 {
		t.Fatal("no descriptor could be reserved")
	}
	_ = syscall.Close(held[len(held)-1])
	held = held[:len(held)-1]
	master, side, err = openPair(Size{Cols: 10, Rows: 2})
	if master != nil || side != nil {
		t.Fatalf("a failed open returned files: %v %v", master, side)
	}
	if !errors.Is(err, syscall.EMFILE) {
		t.Fatalf("the side's open: %v", err)
	}
	fd, err := syscall.Open("/dev/null", syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("the failed open kept its master: no descriptor is free: %v", err)
	}
	_ = syscall.Close(fd)
}

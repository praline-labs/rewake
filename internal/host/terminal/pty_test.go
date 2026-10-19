package terminal

import (
	"errors"
	"os"
	"os/exec"
	"runtime/debug"
	"slices"
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

// A locale with no UTF-8 gets LC_CTYPE=C.UTF-8 in place of its own; a UTF-8
// one, or an LC_ALL the person set, stays as it is.
func TestTheLocaleIsMadeUTF8(t *testing.T) {
	for _, c := range []struct{ env, want []string }{
		{[]string{"LANG=C", "LC_CTYPE=POSIX"}, []string{"LANG=C", "LC_CTYPE=C.UTF-8"}},
		{[]string{"PATH=/bin"}, []string{"PATH=/bin", "LC_CTYPE=C.UTF-8"}},
		{[]string{"LANG=en_US.UTF-8"}, []string{"LANG=en_US.UTF-8"}},
		{[]string{"LANG=C", "LC_CTYPE=ru_RU.utf8"}, []string{"LANG=C", "LC_CTYPE=ru_RU.utf8"}},
		{[]string{"LC_ALL=C"}, []string{"LC_ALL=C"}},
	} {
		if got := UTF8Locale(c.env); !slices.Equal(got, c.want) {
			t.Errorf("%q became %q, want %q", c.env, got, c.want)
		}
	}
}

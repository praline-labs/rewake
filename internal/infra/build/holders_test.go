package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Holders are matched in the lock table by device and inode — an inode alone
// repeats across devices — among held flocks only, never this process, and
// named by pid and executable, never by command line.
func TestHolderDiagnosticsUseDeviceAndInode(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	lock, err := state.OpenLeaseLock(room)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	// The kernel's own table: a holder of the file is found by the key the
	// lock table writes, whatever the device.
	other := startHolder(t, "shared", leaseLockPath(room))
	if !other.held() {
		t.Fatalf("the holder: %s", other.answer)
	}
	if found := holdersOf(lock); len(found) != 1 || found[0].Pid != other.cmd.Process.Pid {
		t.Fatalf("in /proc/locks: %+v, want pid %d", found, other.cmd.Process.Pid)
	}
	other.release()

	sleeper := exec.Command("sleep", "30")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sleeper.Process.Kill(); _ = sleeper.Wait() }()
	dev, ino, err := lock.File()
	if err != nil {
		t.Fatal(err)
	}
	key, elsewhere := lockKey(dev, ino), lockKey(dev+1, ino)
	if key == elsewhere {
		t.Fatal("two devices share a key")
	}
	table := strings.Join([]string{
		fmt.Sprintf("1: FLOCK  ADVISORY  READ  %d %s 0 EOF", sleeper.Process.Pid, key),
		fmt.Sprintf("2: FLOCK  ADVISORY  READ  %d %s 0 EOF", 4000001, elsewhere),
		fmt.Sprintf("2: -> FLOCK  ADVISORY  WRITE %d %s 0 EOF", 4000002, key),
		fmt.Sprintf("3: POSIX  ADVISORY  WRITE %d %s 0 EOF", 4000003, key),
		fmt.Sprintf("4: FLOCK  ADVISORY  READ  %d %s 0 EOF", os.Getpid(), key),
		fmt.Sprintf("5: FLOCK  ADVISORY  READ  %d %s 0 EOF", sleeper.Process.Pid, key),
		"",
	}, "\n")
	fake := filepath.Join(t.TempDir(), "locks")
	if err := os.WriteFile(fake, []byte(table), 0o600); err != nil {
		t.Fatal(err)
	}
	before := locksTable
	locksTable = fake
	defer func() { locksTable = before }()
	found := holdersOf(lock)
	if len(found) != 1 || found[0].Pid != sleeper.Process.Pid {
		t.Fatalf("holders %+v, want only pid %d", found, sleeper.Process.Pid)
	}
	if want := "pid " + strconv.Itoa(sleeper.Process.Pid) + " (sleep)"; found[0].String() != want {
		t.Fatalf("named %q, want %q: the executable, not its arguments", found[0].String(), want)
	}
	found[0].Session = "api"
	if want := "pid " + strconv.Itoa(sleeper.Process.Pid) + " (session api, sleep)"; found[0].String() != want {
		t.Fatalf("named %q, want %q", found[0].String(), want)
	}
}

// The lease's descriptor is close-on-exec: a child takes its own lease and
// never keeps its parent's room locked after the parent is gone.
func TestLeaseDescriptorsAreNotInherited(t *testing.T) {
	base := newBase(t)
	room := established(t, base, "default")
	lease, err := Acquire(base, "default")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range fds {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err != nil || target != leaseLockPath(room) {
			continue
		}
		found = true
		fd, _ := strconv.Atoi(entry.Name())
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_GETFD, 0)
		if errno != 0 || flags&syscall.FD_CLOEXEC == 0 {
			t.Fatalf("descriptor %d of the lease: flags %#x, %v", fd, flags, errno)
		}
	}
	if !found {
		t.Fatal("no descriptor of the lease is open")
	}
	out, err := exec.Command("sh", "-c", "ls -l /proc/$$/fd").CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	if strings.Contains(string(out), "build.lock") {
		t.Fatalf("a child inherited the lease:\n%s", out)
	}
}

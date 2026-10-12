package build

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// locksTable is the kernel's table of file locks; a test points it at a
// table of its own.
var locksTable = "/proc/locks"

// Holder is a process holding a room's lease, named by what is safe to
// print: its pid, the base name of its executable and, when the caller knows
// it, the session it serves. Never its command line, which may carry what a
// diagnostic must not show (L2).
type Holder struct {
	Pid   int
	Label string
	// Session is the name of the session the process serves, filled by a
	// caller that can read the room's records.
	Session string
}

func (h Holder) String() string {
	text := "pid " + strconv.Itoa(h.Pid)
	var about []string
	if h.Session != "" {
		about = append(about, "session "+h.Session)
	}
	if h.Label != "" {
		about = append(about, h.Label)
	}
	if len(about) > 0 {
		text += " (" + strings.Join(about, ", ") + ")"
	}
	return text
}

// holdersOf names the processes other than this one holding the lock's file.
// They are matched by device and inode, since an inode alone repeats across
// devices. Whatever cannot be read leaves the list shorter; it never makes the
// lock look free — the caller has already been refused it.
func holdersOf(lock *state.LeaseLock) []Holder {
	dev, ino, err := lock.File()
	if err != nil {
		return nil
	}
	file, err := os.Open(locksTable)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()
	want := lockKey(dev, ino)
	self := os.Getpid()
	seen := map[int]bool{}
	var holders []Holder
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		pid, key, ok := parseLockLine(scanner.Text())
		if !ok || key != want || pid == self || seen[pid] {
			continue
		}
		seen[pid] = true
		holders = append(holders, Holder{Pid: pid, Label: label(pid)})
	}
	return holders
}

// parseLockLine reads one line of the lock table: a held flock, not a waiter
// (a "->" line), with its pid and its file as major:minor:inode.
func parseLockLine(line string) (int, string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 6 || fields[1] != "FLOCK" {
		return 0, "", false
	}
	pid, err := strconv.Atoi(fields[4])
	if err != nil || pid <= 0 {
		return 0, "", false
	}
	return pid, fields[5], true
}

// lockKey is how the lock table writes a file: the device's major and minor
// in hex, at least two digits each, and the inode in decimal. The device is
// split the way glibc's major and minor macros split st_dev.
func lockKey(dev, ino uint64) string {
	major := (dev>>8)&0xfff | (dev>>32)&^uint64(0xfff)
	minor := dev&0xff | (dev>>12)&^uint64(0xff)
	return fmt.Sprintf("%02x:%02x:%d", major, minor, ino)
}

// label is the base name of a process's executable, empty when it cannot be
// read: the process may have gone, or be another user's.
func label(pid int) string {
	target, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(target), " (deleted)")
}

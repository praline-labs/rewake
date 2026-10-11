package proc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNotDescendant says a process is not below the one it was checked against.
var ErrNotDescendant = errors.New("not a descendant")

// Parent returns the parent of a process and the process's own start time,
// read from one stat line so both describe the same process.
func (r Reader) Parent(pid int) (int, uint64, error) {
	fields, err := r.statFields(pid)
	if err != nil {
		return 0, 0, err
	}
	const parentIndex, startTimeIndex = 1, 19
	if len(fields) <= startTimeIndex {
		return 0, 0, fmt.Errorf("stat line for pid %d has %d fields after the name", pid, len(fields))
	}
	parent, err := strconv.Atoi(fields[parentIndex])
	if err != nil {
		return 0, 0, fmt.Errorf("unreadable parent for pid %d: %w", pid, err)
	}
	start, err := strconv.ParseUint(fields[startTimeIndex], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("unreadable start time for pid %d: %w", pid, err)
	}
	return parent, start, nil
}

// Descends says whether pid is ancestor itself or runs below it.
//
// The chain is walked through /proc, which a pid reused on the way could
// bend: a process is never older than its parent, so a link whose parent
// started after it is a pid that was taken again, and the walk refuses it
// rather than follow a stranger.
func (r Reader) Descends(pid, ancestor int) error {
	child, childStart := pid, uint64(0)
	for range 4096 {
		parent, start, err := r.Parent(child)
		if err != nil {
			return fmt.Errorf("%w: cannot read process %d: %v", ErrNotDescendant, child, err)
		}
		if childStart != 0 && start > childStart {
			return fmt.Errorf("%w: process %d started after its child", ErrNotDescendant, child)
		}
		if child == ancestor {
			return nil
		}
		if parent <= 0 || parent == child {
			return fmt.Errorf("%w: process %d does not run below %d", ErrNotDescendant, pid, ancestor)
		}
		child, childStart = parent, start
	}
	return fmt.Errorf("%w: the chain above process %d does not end", ErrNotDescendant, pid)
}

// Namespaces names the mount, user and pid namespaces of a process. A
// sandbox that confines a process with namespaces gives it other ones than
// the processes outside it, and nothing it runs can take them off.
func (r Reader) Namespaces(pid string) (string, error) {
	var names []string
	for _, kind := range []string{"mnt", "user", "pid"} {
		link, err := os.Readlink(filepath.Join(r.Root, pid, "ns", kind))
		if err != nil {
			return "", err
		}
		names = append(names, link)
	}
	return strings.Join(names, " "), nil
}

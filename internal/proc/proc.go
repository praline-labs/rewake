// Package proc checks process identity, liveness and job-control state. A pid
// alone is not an identity: records also carry its start time and namespace.
package proc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Reader reads a /proc-shaped tree. The root is a field so tests can point it
// at a fixture instead of the running machine.
type Reader struct {
	Root string
}

// Default reads the real /proc.
var Default = Reader{Root: "/proc"}

// StartTime returns the start time of a process in clock ticks since boot.
func StartTime(pid int) (uint64, error) { return Default.StartTime(pid) }

// Namespace identifies the pid namespace this process can see.
func Namespace() string { return Default.Namespace() }

// Alive reports whether the process is the one that was started.
func Alive(pid int, startTime uint64) bool { return Default.Alive(pid, startTime) }

// State returns the one-letter state of a process in the default /proc.
func State(pid int) (string, error) { return Default.State(pid) }

// StartTime returns field 22 of /proc/<pid>/stat: the moment the process
// started, in clock ticks since boot.
func (r Reader) StartTime(pid int) (uint64, error) {
	fields, err := r.statFields(pid)
	if err != nil {
		return 0, err
	}
	// Field 3 of the whole line is the first one after the name, so field 22 is
	// index 19 here.
	const startTimeIndex = 19
	if len(fields) <= startTimeIndex {
		return 0, fmt.Errorf("stat line for pid %d has %d fields after the name", pid, len(fields))
	}
	value, err := strconv.ParseUint(fields[startTimeIndex], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unreadable start time for pid %d: %w", pid, err)
	}
	return value, nil
}

// State returns the single-letter process state: R, S, D, Z and the rest.
func (r Reader) State(pid int) (string, error) {
	fields, err := r.statFields(pid)
	if err != nil {
		return "", err
	}
	if len(fields) == 0 {
		return "", fmt.Errorf("stat line for pid %d has no state field", pid)
	}
	return fields[0], nil
}

// statFields returns the fields of /proc/<pid>/stat that follow the command
// name, which is the only part that can be split on spaces safely.
func (r Reader) statFields(pid int) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(r.Root, strconv.Itoa(pid), "stat"))
	if err != nil {
		return nil, err
	}
	// The second field is the executable name in parentheses and may itself
	// contain spaces and parentheses, so fields are counted after the last one.
	closeParen := strings.LastIndex(string(raw), ")")
	if closeParen < 0 {
		return nil, fmt.Errorf("unreadable stat line for pid %d", pid)
	}
	return strings.Fields(string(raw)[closeParen+1:]), nil
}

// Alive reports whether pid is running and started at startTime. A start time of
// zero means it was never recorded, and then the pid alone has to do.
//
// A zombie does not count. Its entry in /proc survives with the same start time
// until the parent reaps it, so treating it as alive keeps a session listed and
// accepting messages that nothing will ever deliver.
func (r Reader) Alive(pid int, startTime uint64) bool {
	if pid <= 0 {
		return false
	}
	current, err := r.StartTime(pid)
	if err != nil {
		return false
	}
	if startTime != 0 && current != startTime {
		return false
	}
	if state, err := r.State(pid); err == nil && state == "Z" {
		return false
	}
	return true
}

// Namespace identifies the pid namespace of the reading process.
//
// It matters because a pid means nothing outside the namespace it came from. An
// agent whose harness runs its commands in a sandbox sees only its own
// processes, so every other pid looks dead to it. Comparing namespaces is how a
// reader knows it cannot judge rather than concluding the session has ended.
func (r Reader) Namespace() string {
	link, err := os.Readlink(filepath.Join(r.Root, "self", "ns", "pid"))
	if err != nil {
		return ""
	}
	return link
}

// Signal sends a signal to a process, reporting whether it was still there.
func Signal(pid int, signal syscall.Signal) error {
	return syscall.Kill(pid, signal)
}

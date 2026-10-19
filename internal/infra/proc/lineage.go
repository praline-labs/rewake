package proc

import (
	"errors"
	"fmt"
	"strconv"
)

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

// ErrDescendant says a process runs below the one it was checked against.
var ErrDescendant = errors.New("a descendant")

// Outside shows that pid does not run below ancestor, from its chain of
// parents read whole up to the first process: nil only when every link reads,
// none is ancestor and none is a reused pid. ErrDescendant is a pid below
// ancestor; any other error is a chain that could not be shown.
func (r Reader) Outside(pid, ancestor int) error {
	child, childStart := pid, uint64(0)
	for range 4096 {
		if child == ancestor {
			return fmt.Errorf("%w: process %d runs below %d", ErrDescendant, pid, ancestor)
		}
		parent, start, err := r.Parent(child)
		if err != nil {
			return fmt.Errorf("cannot read process %d above %d: %w", child, pid, err)
		}
		if childStart != 0 && start > childStart {
			return fmt.Errorf("process %d started after its child, so the chain above %d is not whole", child, pid)
		}
		if parent <= 0 {
			return nil
		}
		if parent == child {
			return fmt.Errorf("process %d is its own parent", child)
		}
		child, childStart = parent, start
	}
	return fmt.Errorf("the chain above process %d does not end", pid)
}

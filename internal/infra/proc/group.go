package proc

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// GroupAlive says whether any process of a group is still there in the
// default /proc, zombies that wait to be reaped aside: a group whose leader
// was reaped may still hold a child that outlived it. A process whose leader
// thread is a zombie is one only once its other threads have ended.
func GroupAlive(pgid int) bool {
	if syscall.Kill(-pgid, 0) != nil {
		return false
	}
	return Default.GroupAlive(pgid)
}

// GroupAlive reads the tree for a process of the group that is not a zombie.
// A tree it cannot list counts as holding one: an unknown answer is never an
// empty one.
func (r Reader) GroupAlive(pgid int) bool {
	entries, err := os.ReadDir(r.Root)
	if err != nil {
		return true
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		// Fields after the name: state, parent, group.
		fields, err := r.statFields(pid)
		if err != nil || len(fields) < 3 || fields[2] != strconv.Itoa(pgid) {
			continue
		}
		if fields[0] != "Z" || r.threadAlive(pid) {
			return true
		}
	}
	return false
}

// threadAlive says whether a process whose leader thread is a zombie still
// runs another thread: a leader that ended with pthread_exit is shown as one
// while the rest of the program goes on. A task list it cannot read counts as
// holding one.
func (r Reader) threadAlive(pid int) bool {
	dir := filepath.Join(r.Root, strconv.Itoa(pid), "task")
	tasks, err := os.ReadDir(dir)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	for _, task := range tasks {
		fields, err := fieldsAt(filepath.Join(dir, task.Name(), "stat"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || len(fields) == 0 || (fields[0] != "Z" && fields[0] != "X") {
			return true
		}
	}
	return false
}

package proc

import (
	"os"
	"strconv"
	"syscall"
)

// GroupAlive says whether any process of a group is still there in the
// default /proc, zombies that wait to be reaped aside: a group whose leader
// was reaped may still hold a child that outlived it.
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
		if fields[0] != "Z" {
			return true
		}
	}
	return false
}

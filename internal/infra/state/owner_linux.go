package state

import (
	"io/fs"
	"syscall"
)

// ownerUID reads the owning uid of a file where the platform reports one.
func ownerUID(info fs.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Uid), true
}

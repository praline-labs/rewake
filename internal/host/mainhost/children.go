package mainhost

import (
	"syscall"
	"unsafe"
)

// The host's children — main's wrapper and each shell — are waited without
// being reaped: a waiter learns of the exit and posts it, and the loop reaps
// the child when it takes that event. An exit a switch's exec cut off between
// the two is still there, unreaped, for the next image's waiter to find
// (docs/shell.md#a-switch-of-the-rooms-build), and a pid the loop has not
// reaped is never another process's.

// waitid's flags and idtype, which the syscall package does not name.
const (
	pPID    = 1
	wEXITED = 0x4
	wNOHANG = 0x1
	wNOWAIT = 0x1000000
)

// waitid waits for pid's exit with the given flags, leaving it unreaped; true
// when it has exited.
func waitid(pid int, flags uintptr) (bool, error) {
	// siginfo_t: 128 bytes, si_pid at offset 16, zero when WNOHANG found no
	// exit.
	var info [128]byte
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, pPID, uintptr(pid), uintptr(unsafe.Pointer(&info[0])), flags, 0, 0)
		switch errno {
		case 0:
			return *(*int32)(unsafe.Pointer(&info[16])) != 0, nil
		case syscall.EINTR:
			continue
		}
		return false, errno
	}
}

// waitExit returns once pid has exited, or cannot be waited, leaving it for
// the loop to reap.
func waitExit(pid int) { _, _ = waitid(pid, wEXITED|wNOWAIT) }

// hasExited says pid has exited and waits to be reaped.
func hasExited(pid int) bool {
	gone, err := waitid(pid, wEXITED|wNOWAIT|wNOHANG)
	return gone || err != nil
}

// reap takes pid's exit as its code: a signal's end is 1.
func reap(pid int) int {
	var status syscall.WaitStatus
	for {
		_, err := syscall.Wait4(pid, &status, 0, nil)
		switch {
		case err == syscall.EINTR:
			continue
		case err != nil:
			return 1
		case status.Exited():
			return status.ExitStatus()
		}
		return 1
	}
}

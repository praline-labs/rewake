/*
Package term covers the one piece of terminal control rewake needs: which
process group the terminal talks to.

The harness runs in a process group of its own, and that group has to be the
foreground one — otherwise Ctrl+C would reach the wrapper instead of the agent,
and reading from the terminal would stop the harness on SIGTTIN. This is what a
shell does for a job; rewake does the same for the program it starts.
*/
package term

import (
	"os"
	"syscall"
	"unsafe"
)

// IsTerminal reports whether the file is a terminal.
func IsTerminal(file *os.File) bool {
	var termios [64]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), tcgets, uintptr(unsafe.Pointer(&termios[0])))
	return errno == 0
}

// ForegroundGroup returns the process group the terminal sends its signals to.
func ForegroundGroup(file *os.File) (int, error) {
	var group int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), tiocgpgrp, uintptr(unsafe.Pointer(&group)))
	if errno != 0 {
		return 0, errno
	}
	return int(group), nil
}

// SetForegroundGroup hands the terminal to a process group. The caller must be
// ignoring SIGTTOU: a process outside the foreground group is stopped for
// trying this, and the wrapper is exactly that for the moment in between.
func SetForegroundGroup(file *os.File, group int) error {
	value := int32(group)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), tiocspgrp, uintptr(unsafe.Pointer(&value)))
	if errno != 0 {
		return errno
	}
	return nil
}

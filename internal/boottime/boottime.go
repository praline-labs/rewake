// Package boottime reads the machine's boot clock, CLOCK_BOOTTIME: nanoseconds
// since the machine booted, counting time spent suspended.
//
// It is one clock for every process on the machine, so readings taken by
// different processes compare, and unlike the wall clock it is never set back —
// not by a person, and not by a time sync after the machine wakes, which is
// when a WSL guest's wall clock jumps. rewake uses it where the order of events
// seen by different processes decides something: which turn a mark belongs to,
// which of two late hooks is newer.
package boottime

import (
	"syscall"
	"unsafe"
)

const clockBoottime = 7

// ProcessStarted is this process's reading, taken while the package
// initializes: before main runs, and so before anything it was handed — a
// hook's stdin, say — has been read.
var ProcessStarted = Now()

// Now reads the clock, or answers 0 when it cannot. Zero is never a real
// reading, so a caller can take it for "unknown".
func Now() int64 {
	var now syscall.Timespec
	if _, _, errno := syscall.RawSyscall(syscall.SYS_CLOCK_GETTIME, clockBoottime, uintptr(unsafe.Pointer(&now)), 0); errno != 0 {
		return 0
	}
	return now.Nano()
}

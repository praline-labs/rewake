// Package boottime reads the machine's boot clock, CLOCK_BOOTTIME: nanoseconds
// since the machine booted, counting time spent suspended.
//
// It is one clock for every process on the machine, so readings taken by
// different processes compare, and unlike the wall clock it is never set back —
// not by a person, and not by a time sync after the machine wakes, which is
// when a WSL guest's wall clock jumps. rewake uses it where the order of events
// seen by different processes decides something: which turn a mark belongs to,
// which of two late hooks is newer.
//
// And where a duration of seconds is measured between two processes: the
// coalescing window counts from a letter's reading, a waiting send's answer mark
// holds one, and so does a telemetry snapshot. The wall clock can be stepped by
// seconds at any moment — on some virtual machines a time sync steps it every
// half a minute — and every such duration grew or shrank by the step.
//
// Terms of minutes and more stay on the wall clock: a letter's thirty-minute
// lifetime, the day finished mail is kept, the five minutes main waits for the
// end of a compaction, and the order ids sort in. A virtual machine paused while
// its host sleeps does not advance this clock, so a letter left for a night would
// still look fresh by it, and an id must still sort after a reboot, which starts
// this clock again from zero.
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

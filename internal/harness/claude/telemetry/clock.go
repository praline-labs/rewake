package telemetry

import (
	"syscall"
	"unsafe"
)

// clockBoottime is CLOCK_BOOTTIME: nanoseconds since the machine booted,
// counting time spent suspended. It is one clock for every process on the
// machine, so two senders' readings compare, and unlike the wall clock it is
// never set back — not by a person, not by a time sync after the machine
// wakes, which is when a WSL guest's wall clock jumps.
const clockBoottime = 7

// processStarted is this process's reading of that clock, taken while the
// package initializes: before main runs, and so before a hook's stdin is read.
// The order in which two hooks were started is what the collector needs, and
// the time of reading their payloads would put a slow start after a fast one.
var processStarted = bootClock()

// bootClock reads CLOCK_BOOTTIME, or answers 0 when it cannot. A zero is the
// earliest possible time, so the collector drops such an event once it has
// applied one with a real reading (state.go); only among zeros does arrival
// order decide. The clock is there on every Linux this runs on, so a zero is
// not expected at all.
func bootClock() int64 {
	var now syscall.Timespec
	if _, _, errno := syscall.RawSyscall(syscall.SYS_CLOCK_GETTIME, clockBoottime, uintptr(unsafe.Pointer(&now)), 0); errno != 0 {
		return 0
	}
	return now.Nano()
}

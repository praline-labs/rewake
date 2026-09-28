package telemetry

import "github.com/praline-labs/rewake/internal/boottime"

// processStarted is when this sending process started, on the boot clock: the
// order in which two hooks were started is what the collector needs, and the
// time of reading their payloads would put a slow start after a fast one.
var processStarted = boottime.ProcessStarted

// bootClock reads the boot clock now.
func bootClock() int64 { return boottime.Now() }

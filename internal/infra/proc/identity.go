package proc

import (
	"errors"
	"os"
	"strconv"
	"syscall"
)

// IdentityEvidence separates an observed exit from inability to inspect a PID.
// Notice decisions must not turn permission or malformed-stat errors into death.
type IdentityEvidence uint8

// IdentityUnknown defers a notice decision until identity can be inspected again.
const (
	IdentityUnknown IdentityEvidence = iota
	IdentityAlive
	IdentityEnded
)

// ObserveIdentity preserves uncertainty when the process cannot be inspected.
func ObserveIdentity(pid int, startTime uint64) IdentityEvidence {
	return Default.ObserveIdentity(pid, startTime)
}

// ObserveIdentity uses one stat read so state and start time describe the same
// observation. Existing boolean reachability and registry cleanup are unchanged.
func (r Reader) ObserveIdentity(pid int, startTime uint64) IdentityEvidence {
	if pid <= 0 {
		return IdentityUnknown
	}
	fields, err := r.statFields(pid)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			return IdentityEnded
		}
		return IdentityUnknown
	}
	const startTimeIndex = 19
	if len(fields) <= startTimeIndex || len(fields[0]) != 1 {
		return IdentityUnknown
	}
	switch fields[0] {
	case "R", "S", "D", "T", "t", "K", "W", "P", "I", "Z":
	default:
		return IdentityUnknown
	}
	current, err := strconv.ParseUint(fields[startTimeIndex], 10, 64)
	if err != nil {
		return IdentityUnknown
	}
	if startTime != 0 && current != startTime {
		return IdentityEnded
	}
	if fields[0] == "Z" {
		return IdentityEnded
	}
	return IdentityAlive
}

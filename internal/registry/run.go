package registry

import (
	"strconv"
	"strings"
	"sync"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/proc"
)

// currentBoot reads the id of the machine's boot once: it does not change for
// the life of a process. Replaceable, so tests can describe another boot.
var currentBoot = sync.OnceValues(boottime.ID)

// CurrentBoot is the id of the boot this process runs in.
func CurrentBoot() (string, error) { return currentBoot() }

func isCurrentBoot(boot string) bool {
	current, err := currentBoot()
	return err == nil && current == boot
}

// Epoch identifies this run of a session name. A message carries the epoch of
// the session it was written for, so a later session that happens to take the
// same name does not receive somebody else's mail. A run names the boot as
// well, since a pid and its start recur after a restart of the machine; an
// epoch without one was written by a build before the turn journal
// (docs/archive-1.x/protocol-cutover.md).
func (s Session) Epoch() string {
	return RunEpoch(s.ServicePID, s.ServiceStart, s.Boot)
}

// RunEpoch names the run of the wrapper pid, started at start, in boot.
func RunEpoch(pid int, start uint64, boot string) string {
	epoch := strconv.Itoa(pid) + "." + strconv.FormatUint(start, 10)
	if boot != "" {
		epoch += "." + boot
	}
	return epoch
}

// ParseEpoch reads back the wrapper an epoch names: its pid and start time.
func ParseEpoch(epoch string) (int, uint64, bool) {
	pid, start, _, ok := ParseRun(epoch)
	return pid, start, ok
}

// ParseRun reads back every part of an epoch: the wrapper's pid and start,
// and the boot, "" for an epoch that names none.
func ParseRun(epoch string) (int, uint64, string, bool) {
	pid, rest, found := strings.Cut(epoch, ".")
	if !found {
		return 0, 0, "", false
	}
	start, boot, _ := strings.Cut(rest, ".")
	number, err := strconv.Atoi(pid)
	if err != nil || number <= 0 {
		return 0, 0, "", false
	}
	ticks, err := strconv.ParseUint(start, 10, 64)
	if err != nil {
		return 0, 0, "", false
	}
	if boot != "" && !boottime.ValidID(boot) {
		return 0, 0, "", false
	}
	return number, ticks, boot, true
}

// ObserveRun says whether the wrapper of a run is running, has ended, or
// cannot be told, as proc.ObserveIdentity does for a process. A run of
// another boot has ended; a pid and start are compared only in this boot. The
// caller decides whether the pid means anything in this namespace.
func ObserveRun(epoch string) proc.IdentityEvidence {
	pid, start, boot, ok := ParseRun(epoch)
	if !ok {
		return proc.IdentityUnknown
	}
	if boot != "" {
		current, err := currentBoot()
		if err != nil {
			return proc.IdentityUnknown
		}
		if current != boot {
			return proc.IdentityEnded
		}
	}
	return observe(pid, start)
}

// observe is the three-valued liveness check, replaceable in tests.
var observe = proc.ObserveIdentity

// EpochAlive says the run's wrapper is running now: ObserveRun proved it
// alive. An answer that cannot be told is no, as proc.Alive says no.
func EpochAlive(epoch string) bool { return ObserveRun(epoch) == proc.IdentityAlive }

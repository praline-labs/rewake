package registry

import (
	"time"

	"github.com/praline-labs/rewake/internal/proc"
)

// Whether a published session is still the run it names, and how long it
// has been published.

// alive is the liveness check, replaceable so tests can describe a machine
// instead of running processes on the real one.
var alive = proc.Alive

// namespace reports the pid namespace of this process, replaceable in tests.
var namespace = proc.Namespace

// Reachable reports whether a session may still be sent to. It is not the same
// question as Alive: a reader that cannot see the processes says yes, because
// the wrapper that can see them is the one that will deliver.
func (s Session) Reachable() bool { return s.Alive() }

// Judgeable reports whether this reader can tell if the session is running.
//
// Only a reader in the same pid namespace can. A sandboxed agent runs its
// commands in a namespace of its own, where every pid but its own is missing —
// and a reader that mistook that for death reported live sessions as gone and
// deleted their records. Found by running it.
func (s Session) Judgeable() bool {
	here := namespace()
	if here == "" {
		// This reader cannot even tell which namespace it is in, so it cannot
		// tell whether a pid means anything here. Guessing the other way
		// deleted the record of a session that was running.
		return false
	}
	// A record without a namespace comes from a version that did not record
	// one; the reader's own is the best it has.
	// legacy(rewake <2026-09-16): records of earlier builds carry no pidNamespace; remove when no session started by such a build is registered
	return s.PIDNamespace == "" || s.PIDNamespace == here
}

// Alive reports whether this session can still be reached: both the process
// serving the mailbox and the harness itself have to be running. A wrapper that
// outlives its harness has nothing to deliver to, and a harness whose wrapper is
// gone has nobody to deliver for it.
func (s Session) Alive() bool {
	if s.Boot != "" && !isCurrentBoot(s.Boot) {
		// A run of another boot of the machine has ended, whatever runs now
		// under its pid and start.
		return false
	}
	if !s.Judgeable() {
		// Cannot see those processes from here. Saying "alive" leaves delivery
		// to the wrapper, which can see them; saying "dead" would drop mail and
		// remove a record belonging to a session that is running.
		return true
	}
	if !alive(s.ServicePID, s.ServiceStart) {
		return false
	}
	if s.HarnessPID != 0 && !alive(s.HarnessPID, s.HarnessStart) {
		return false
	}
	return true
}

// Age is how long the session has been published.
func (s Session) Age() time.Duration { return time.Since(s.StartedAt) }

package workflow

import (
	"fmt"
	"syscall"
	"time"
)

// How long cleanup is allowed to take, and the loop that does it. Kept apart
// from process ownership because the two answer different questions: that file
// says what the case is responsible for, this one says how it lets go.

// groupTerminationBudget is the whole time a group gets to disappear after it
// is asked to stop — SIGTERM, then SIGKILL, then gone. It is one limit, not
// one per signal: a stuck group would otherwise hold the run for twice as
// long as anything here claims.
//
// It is the sum of the stages an ordinary rewake shutdown runs, read from the
// code:
//
//   - 5 s for the completion publisher to drain after cancellation
//     (internal/harness/codex/server_events.go);
//   - 2 s for the server session to stop its own child group, SIGTERM then
//     SIGKILL (internal/harness/codex/server.go);
//   - 2 s for the last mailbox writes to get the lock
//     (internal/inbox/outcome.go, shutdownLockWait);
//   - 300 ms of signal-forwarding grace (internal/wrap/wrap.go).
//
// That is 9.3 s of stages that can follow one another, rounded up to 12 for a
// loaded machine. The wrapper's own 2 s child stop (internal/wrap/backend.go)
// is deliberately not in the sum: it is the refusal branch, not a stage every
// ordinary shutdown passes through. Killing earlier would cut off exactly the
// teardown this suite exists to observe.
const groupTerminationBudget = 12 * time.Second

// terminationBudget is what the loop actually waits for. It is the real budget
// above, and stays that way outside a test that shortens it on purpose: a
// regression about SIGKILL after an ignored SIGTERM has to be in the ordinary
// gate, and waiting the full twelve seconds for it in every run buys nothing.
// Only the loop's own pacing changes; the budget above, and the shutdown
// stages it is built from, are untouched.
//
// A package variable is safe here because the tests that shorten it do not
// call t.Parallel: Go resumes the parallel tests only once every serial one
// has finished, so no case is cleaning up while the budget is changed — see
// the conventions in outcome_test.go.
var terminationBudget = groupTerminationBudget

// killGrace is how much of that budget is reserved for SIGKILL to take effect
// once SIGTERM has clearly not worked.
const killGrace = time.Second

// blindBudget is how long the cleanup keeps trying when every look at the
// process tree fails. Long enough to ride out a momentary failure, short
// enough that a genuinely unreadable /proc does not cost the whole budget. A
// successful look resets it, so it only ends a run of failures.
//
// It is a bail-out, not a clean finish, and it does not promise the cleanup
// happened: with the process tree unreadable, this returns with descendants
// possibly still running. The case is red — and its litter is still there.
const blindBudget = time.Second

// terminate ends everything the case is responsible for and names whatever had
// to be forced: the process's own group, when it has one, and any descendant
// that escaped into a group of its own.
//
// One loop, one budget, for both halves. They used to be two — a loop for the
// group and another for the strays — with the same three constants and the
// same shape, which meant a change to either had to be made twice and nothing
// in the code said so.
//
// The sweeping happens *while* the wait for the process is still blocked,
// which is the point. A descendant that left the group through setsid
// inherits the pipe, so waiting for the parent first means waiting on exactly
// the process that nothing has yet cleaned up: with a 100 ms deadline that
// took 14 seconds and an outside rescue.
//
// own may be nil, for a pass that only looks for escaped descendants. Only
// descendants carrying the label are this call's to end; an empty label takes
// everything, which only the final sweep may.
func terminate(own *owned, label string, known map[int]bool) []string {
	started := time.Now()
	deadline := started.Add(terminationBudget)
	// Nothing is called clean before this: /proc answers about an instant,
	// and a descendant about to be re-parented here would otherwise be missed
	// by a single glance.
	settled := started.Add(straySettle)
	// waitingFor is the process half of the job; it becomes nil once that half
	// is done and the loop moves on to waiting for a quiet tree.
	waitingFor := own
	if own != nil {
		own.signalGroup(syscall.SIGTERM)
	}
	// Every pass signals, not only the first: a descendant can appear after
	// the sweep that was supposed to catch it — that is the whole reason the
	// loop keeps looking — and one that never gets asked to stop would sit
	// there until the budget ran out.
	signal := syscall.SIGTERM
	killed := false
	var left []string
	// blindSince is when the current run of unreadable passes began. Waiting
	// out the whole budget while every look fails buys nothing: the case is
	// already red, and the condition being waited for cannot be observed.
	var blindSince time.Time
	described := map[int]string{}
	for {
		found, err := sweepOnce(label, known, signal, described)
		if err != nil {
			left = append(left, err.Error())
			if blindSince.IsZero() {
				blindSince = time.Now()
			}
		} else {
			blindSince = time.Time{}
		}
		left = append(left, found...)
		// A pass that could not look proves nothing, so it must not be the
		// pass that ends the loop: the error makes the case red, and the
		// cleanup carries on rather than leaving on a failed reading.

		ownDone := true
		if waitingFor != nil {
			waitingFor.reapGroup()
			ownDone = waitingFor.finished() && !waitingFor.groupAlive()
		}
		now := time.Now()
		// Two halves, finished at different moments, and the second one only
		// begins when the first is done: a case that owns a process waits for
		// that process and its group, and *then* waits for a quiet tree. The
		// order matters — leaving as soon as the process is gone abandons a
		// descendant that ignored SIGTERM, which never gets the SIGKILL the
		// budget was reserving for it. A red result is not a substitute:
		// the case fails either way, but the process keeps running.
		if waitingFor != nil && ownDone {
			waitingFor = nil
			// The sweep has not had its settling time yet; give it now, from
			// here, without extending the overall budget.
			settled = now.Add(straySettle)
		}
		if waitingFor == nil && err == nil && len(found) == 0 && now.After(settled) {
			return dedup(left)
		}
		// Blind for long enough, with our own half done: report rather than
		// spend the budget on looks that cannot succeed.
		if waitingFor == nil && !blindSince.IsZero() && now.Sub(blindSince) > blindBudget {
			return dedup(left)
		}
		if !killed && now.After(deadline.Add(-killGrace)) {
			if own != nil {
				own.signalGroup(syscall.SIGKILL)
			}
			signal = syscall.SIGKILL
			killed = true
		}

		if now.After(deadline) {
			if own != nil && !ownDone {
				left = append(left, fmt.Sprintf("group %d of %s would not end", own.pgid, own.name))
			}
			return dedup(left)
		}
		time.Sleep(groupPoll)
	}
}

// dedup keeps each problem once: a sweep that runs every few milliseconds
// would otherwise report the same survivor dozens of times.
func dedup(problems []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, problem := range problems {
		if seen[problem] {
			continue
		}
		seen[problem] = true
		unique = append(unique, problem)
	}
	return unique
}

package wrap

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
)

// followStop stops the wrapper with a harness that stopped on its own, and
// brings the harness back when the wrapper is continued.
//
// The report of a stop can be old news. Ctrl+Z stops the whole job, wrapper
// included, and the report is read only after "fg" has continued both — stopping
// again then handed the shell a stopped job while the harness ran on its own.
// So the wrapper follows only a harness that is stopped right now.
func followStop(pid int, stopped func(int) bool, stop func()) {
	if !stopped(pid) {
		return
	}
	stop()
	_ = syscall.Kill(pid, syscall.SIGCONT)
}

// stillStopped reports whether a process is in a job-control stop now.
func stillStopped(pid int) bool {
	state, err := proc.State(pid)
	return err == nil && state == "T"
}

// stopSelf stops the wrapper the way a job is stopped, and returns only once it
// has been continued.
//
// The stop is aimed at the calling thread, not at the process. A stop sent to
// the process is taken by whichever thread the kernel picks — the main one, as a
// rule — while the caller returns from kill and runs on: followStop then
// continued the harness before the wrapper had stopped, leaving the harness
// running and the wrapper stopped. A stop aimed at the calling thread is taken on
// its way out of the syscall, before the next line runs. The thread is locked
// for the pair of calls so the goroutine cannot move between naming the thread
// and signaling it.
func stopSelf() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	_ = syscall.Tgkill(os.Getpid(), syscall.Gettid(), syscall.SIGSTOP)
}

// catchSignals starts listening before there is a child to forward to.
func catchSignals() (chan os.Signal, func()) {
	incoming := make(chan os.Signal, 8)
	signal.Notify(incoming, syscall.SIGTERM, syscall.SIGHUP)
	// The keyboard signals are caught and dropped rather than left to their
	// default. The wrapper shares the harness's group, so Ctrl+C reaches it too,
	// and dying from it left the agent running with nobody serving its mailbox:
	// interrupting a turn must not end the session.
	//
	// Caught, not ignored. An ignored signal stays ignored across exec, so the
	// harness and every command it ran would have lost Ctrl+C too; a caught one
	// is reset to its default in the child.
	keyboard := make(chan os.Signal, 8)
	signal.Notify(keyboard, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		for {
			// Keyboard signals belong to the harness; consume until the channel closes.
			if _, open := <-keyboard; !open {
				return
			}
		}
	}()
	return incoming, func() {
		signal.Stop(incoming)
		// Stop guarantees nothing more is sent, so closing ends the drain.
		signal.Stop(keyboard)
		close(keyboard)
	}
}

// forwardGrace is how long the harness is given to act on a signal it may have
// received directly before the wrapper repeats it.
//
// The workflow suite's termination budget (test/workflow) is the sum of this and
// the other shutdown stages, so a change here has to be reflected there.
const forwardGrace = 300 * time.Millisecond

// forward passes a termination request on to the harness — once, and only if it
// has not already acted on one.
//
// Both processes are in the same group, so a signal sent to the group reaches
// the harness by itself; repeating it turns one request into two, and for many
// programs the second one means "stop cleaning up and die". A signal sent to the
// wrapper alone reaches nobody else, and that is the case this covers. The
// difference between them is not visible in the signal, so the answer comes from
// the harness: if it is still running a moment later, it did not get one.
func forward(incoming chan os.Signal, process *os.Process, alive func() bool) {
	for received := range incoming {
		signalValue, ok := received.(syscall.Signal)
		if !ok {
			continue
		}
		time.Sleep(forwardGrace)
		if !alive() {
			continue
		}
		_ = process.Signal(signalValue)
	}
}

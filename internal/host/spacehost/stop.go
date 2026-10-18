package spacehost

import (
	"errors"
	"syscall"
	"time"
)

// stopTimes are how long each signal of the close sequence is given before
// the next: the hangup a terminal's close would send, then a request to end,
// then the kill no process can refuse.
type stopTimes struct {
	hangup, term, kill time.Duration
}

var stopping = stopTimes{hangup: time.Second, term: 5 * time.Second, kill: 2 * time.Second}

// stopGroup ends the worker's process group: SIGHUP, then SIGTERM, then
// SIGKILL, each sent only while something of the group is left, and says
// which signal it took. The worker's wrapper is the group's leader, in a
// session of its own; its harness runs in the same group. reaped is closed
// once the wrapper is waited for: until then it is a zombie the group still
// counts.
func stopGroup(pgid int, reaped <-chan struct{}, times stopTimes) string {
	steps := []struct {
		signal syscall.Signal
		name   string
		wait   time.Duration
	}{
		{syscall.SIGHUP, "SIGHUP", times.hangup},
		{syscall.SIGTERM, "SIGTERM", times.term},
		{syscall.SIGKILL, "SIGKILL", times.kill},
	}
	if groupGone(pgid, reaped) {
		return "nothing"
	}
	for _, step := range steps {
		_ = syscall.Kill(-pgid, step.signal)
		if waitGone(pgid, reaped, step.wait) {
			return step.name
		}
	}
	return ""
}

func waitGone(pgid int, reaped <-chan struct{}, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for {
		if groupGone(pgid, reaped) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// groupGone says whether no process of the group is left.
func groupGone(pgid int, reaped <-chan struct{}) bool {
	select {
	case <-reaped:
	default:
		return false
	}
	return errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH)
}

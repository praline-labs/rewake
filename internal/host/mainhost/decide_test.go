package mainhost

import "testing"

// Restart, crash and close told apart: a shell's exit is a crash only while
// it has the terminal, the window is there and the one recovery is unspent;
// a failed restart leaves main alone rather than end it; a close or a lost
// terminal ends everything whatever is under way.
func TestTheHostTellsRestartCrashAndCloseApart(t *testing.T) {
	ok := situation{phase: running, window: true}
	with := func(f func(*situation)) situation { s := ok; f(&s); return s }
	cases := []struct {
		name string
		e    event
		s    situation
		want action
	}{
		{"a restart asked for while a shell runs", evRestart, ok, doSwap},
		{"a restart from main alone", evRestart, with(func(s *situation) { s.phase = alone }), doSwap},
		{"a restart during a swap", evRestart, with(func(s *situation) { s.phase = swapping }), doRefuse},
		{"a restart while the first shell starts", evRestart, with(func(s *situation) { s.phase = starting }), doRefuse},
		{"a restart once main is ending", evRestart, with(func(s *situation) { s.ending = true }), doRefuse},
		{"a restart while closing", evRestart, with(func(s *situation) { s.phase = closing }), doRefuse},

		{"the first crash with the window there", evCrash, ok, doRecover},
		{"a second crash", evCrash, with(func(s *situation) { s.crashed = true }), doClose},
		{"a crash with the window gone", evCrash, with(func(s *situation) { s.window = false }), doCloseLost},
		{"a crash once main is ending", evCrash, with(func(s *situation) { s.ending = true }), doClose},
		{"a crash of the first shell", evCrash, with(func(s *situation) { s.phase = starting }), doClose},
		{"a crash while closing", evCrash, with(func(s *situation) { s.phase = closing }), doNothing},

		{"a new shell not ready after a restart", evNotReady, with(func(s *situation) { s.phase = swapping }), doAlone},
		{"a new shell not ready after a crash", evNotReady, with(func(s *situation) { s.phase = recovering; s.crashed = true }), doClose},
		{"the first shell not ready", evNotReady, with(func(s *situation) { s.phase = starting }), doClose},
		{"not ready after a restart, the window gone", evNotReady, with(func(s *situation) { s.phase = swapping; s.window = false }), doCloseLost},
		{"not ready after a restart, main ending", evNotReady, with(func(s *situation) { s.phase = swapping; s.ending = true }), doClose},

		{"a close while a shell runs", evClose, ok, doClose},
		{"a close during a swap", evClose, with(func(s *situation) { s.phase = swapping }), doClose},
		{"a close from main alone", evClose, with(func(s *situation) { s.phase = alone }), doClose},
		{"a second close", evClose, with(func(s *situation) { s.phase = closing }), doNothing},
		{"the terminal lost while a shell runs", evLost, ok, doCloseLost},
		{"the terminal lost during a swap", evLost, with(func(s *situation) { s.phase = swapping }), doCloseLost},
		{"the terminal lost while closing", evLost, with(func(s *situation) { s.phase = closing }), doCloseLost},

		{"main's end", evMainEnded, ok, doEnd},
		{"main's end while alone", evMainEnded, with(func(s *situation) { s.phase = alone }), doEnd},
	}
	for _, c := range cases {
		if got := decide(c.e, c.s); got != c.want {
			t.Errorf("%s (%+v): %s, want %s", c.name, c.s, got, c.want)
		}
	}
}

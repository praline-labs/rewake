package mainhost

// What the host does on each event (docs/shell.md#restart-crash-and-close),
// decided in one place from what it knows: a shell's exit says nothing by
// itself — what the shell said before it, and whether the window is still
// there, do.

// phase is where the host stands.
type phase int

const (
	// starting: the first shell is not ready yet.
	starting phase = iota
	// running: a shell has the person's terminal.
	running
	// swapping: a restart asked for is under way.
	swapping
	// recovering: a shell is starting after a crash.
	recovering
	// alone: no shell; main has the person's terminal.
	alone
	// closing: main is being ended, or has ended.
	closing
)

func (p phase) String() string {
	return [...]string{"starting", "running", "swapping", "recovering", "alone", "closing"}[p]
}

// event is what happened.
type event int

const (
	// evRestart: a restart was asked for, by the shell's key or at the door.
	evRestart event = iota
	// evCrash: the shell that has the terminal exited with no word before.
	evCrash
	// evNotReady: a shell started for the terminal exited, or was not
	// ready within its bound.
	evNotReady
	// evClose: the shell said close, or the host had SIGTERM, SIGINT or
	// SIGQUIT.
	evClose
	// evLost: the terminal is gone — the shell said lost, the host had
	// SIGHUP, or its parent left.
	evLost
	// evMainEnded: main's wrapper exited.
	evMainEnded
)

// situation is what the host knows when an event comes.
type situation struct {
	phase phase
	// crashed: the one restart after a crash of this run is spent.
	crashed bool
	// window: the person's terminal is there, checked positively.
	window bool
	// ending: main has begun to end — its closing mark, or its exit.
	ending bool
}

// action is what the host does.
type action int

const (
	doNothing action = iota
	// doRefuse answers a restart that it is not taken now.
	doRefuse
	// doSwap starts a new shell beside the one there and swaps them.
	doSwap
	// doRecover starts a new shell after a crash.
	doRecover
	// doAlone gives the terminal to main alone.
	doAlone
	// doClose ends main and its workers, the closing drawn.
	doClose
	// doCloseLost ends them with nothing to draw on.
	doCloseLost
	// doEnd lets the shell finish its closing, then exits with main's code.
	doEnd
)

func (a action) String() string {
	return [...]string{"nothing", "refuse", "swap", "recover", "alone", "close", "close-lost", "end"}[a]
}

func decide(e event, s situation) action {
	switch e {
	case evRestart:
		if s.ending || (s.phase != running && s.phase != alone) {
			return doRefuse
		}
		return doSwap
	case evCrash:
		switch {
		case s.phase == closing:
			return doNothing
		case !s.window:
			return doCloseLost
		case s.ending, s.crashed, s.phase != running:
			return doClose
		}
		return doRecover
	case evNotReady:
		switch {
		case s.phase == closing:
			return doNothing
		case !s.window:
			return doCloseLost
		case s.phase == swapping && !s.ending:
			// A failed restart never ends main.
			return doAlone
		}
		return doClose
	case evClose:
		if s.phase == closing {
			return doNothing
		}
		return doClose
	case evLost:
		return doCloseLost
	case evMainEnded:
		return doEnd
	}
	return doNothing
}

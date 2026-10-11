package channel

// Kind names an event the wrapper folds into the record.
type Kind string

// The events, by the source that observes them
// (docs/mail-bridge-channel.md#the-tool-observation).
const (
	// Hello: the endpoint accepted a transport's hello.
	Hello Kind = "hello"
	// Closed: a transport's connection closed; Alive says the harness
	// lives and is not ending.
	Closed Kind = "closed"
	// HelloRefused: a hello the endpoint refused; Descendant says it came
	// from the harness's tree.
	HelloRefused Kind = "hello refused"
	// Validated: a call met its binding, its ticket issued at Issued.
	Validated Kind = "validated"
	// TimerPassed: the hello timer's bound passed.
	TimerPassed Kind = "timer passed"
	// Denied: the harness denied our tool for permission. The adapter
	// names the signal; until one does nothing sends this.
	Denied Kind = "denied"
	// ShellObserved: a shell observation the CLI wrote.
	ShellObserved Kind = "shell observed"
	// Exited: the harness exited or its turn-end shutdown began.
	Exited Kind = "exited"
)

// Event is one observation with the time it happened.
type Event struct {
	Kind Kind
	At   Stamp
	// Generation is a connection's (Hello, Closed).
	Generation uint64
	// Alive: the harness lives and is not ending (Closed).
	Alive bool
	// Descendant: the refused hello came from the harness's tree.
	Descendant bool
	// Issued is when the validated ticket was issued, on the boot clock.
	Issued int64
	// OK and Class are a shell observation's outcome.
	OK    bool
	Class string
}

// Fold applies one event. Events are ordered by when they happened, which
// may not be the order they arrive in: the policy block and the shell keep
// the latest by event time, and the tool observation is derived again from
// the transport's history on every fold (history.go).
func (r *Record) Fold(e Event) {
	if r.Frozen {
		return
	}
	switch e.Kind {
	case Denied:
		// The boundary is the latest denial's, moved only forward; one
		// before the newest validated ticket was issued is lifted already.
		if r.Tool != ToolNone && e.At.Boot > r.Issued && e.At.Boot > r.Block.Boot {
			r.Block = e.At
		}
	case ShellObserved:
		r.shell(e)
	case Exited:
		r.Frozen, r.Timer = true, 0
	case Validated:
		r.validated(e)
	default:
		if r.Tool != ToolNone && r.h.add(e, r.prune()) {
			r.derive()
		}
	}
}

// prune is the time before which the history drops what a ticket ended:
// the last ticket's (history.go).
func (r *Record) prune() int64 { return r.Worked.Boot }

// validated is the tool's one evidence: it ends every failure up to it,
// whenever it is folded, and clears the block only with a ticket issued
// after the latest denial.
func (r *Record) validated(e Event) {
	if r.Tool == ToolNone {
		return
	}
	r.Issued = max(r.Issued, e.Issued)
	if r.Blocked() && e.Issued > r.Block.Boot {
		r.Block = Stamp{}
	}
	if e.At.Boot > r.Worked.Boot {
		r.Worked = e.At
		r.h.worked(e.At.Boot)
		r.derive()
	}
}

// shell keeps the newest observation by its event time, whenever it is
// folded. One from before the open interval is kept too, and confirms
// nothing about it: the display compares the two times, so an interval that
// a later-folded failure moves back still judges the shell by event time.
func (r *Record) shell(e Event) {
	if r.Shell != nil && e.At.Boot < r.Shell.At.Boot {
		return
	}
	class := e.Class
	if e.OK {
		class = ""
	}
	r.Shell = &Shell{OK: e.OK, Class: class, At: e.At}
}

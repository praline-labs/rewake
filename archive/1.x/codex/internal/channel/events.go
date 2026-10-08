package channel

// Kind names an event the wrapper folds into the record.
type Kind string

// The events, by the source that observes them
// (docs/mail-bridge-channel.md#the-tool-observation).
const (
	// SessionStarted: Claude Code's SessionStart, which comes with the start
	// of its servers once every startup dialog is answered; the run's first
	// opens the hello timer, a later one (/clear, /resume) nothing.
	SessionStarted Kind = "session started"
	// ThreadAdmitted: the gateway admitted a thread request that selects no
	// conversation, which starts Codex's servers all the same.
	ThreadAdmitted Kind = "thread admitted"
	// SelectionAdmitted: the gateway admitted a request that selects a
	// conversation (docs/mail-bridge-channel-codex.md#selecting-a-conversation);
	// Thread is the target it names, "" for a thread/start.
	SelectionAdmitted Kind = "selection admitted"
	// Selected: the answer selected Thread as the conversation.
	Selected Kind = "selected"
	// SelectionFailed: the answer was refused, unknown or read-only, or a
	// lifecycle request left the primary empty.
	SelectionFailed Kind = "selection failed"
	// Bound: the first request over a connection named Thread, which binds
	// the connection's generation to it from its hello on.
	Bound Kind = "bound"
	// CallSeen: a PreToolUse of our tool on Claude Code, which starts its
	// server on demand.
	CallSeen Kind = "call seen"
	// Hello: the endpoint accepted a server's hello.
	Hello Kind = "hello"
	// Closed: a server's connection closed; Alive says the harness lives
	// and is not ending.
	Closed Kind = "closed"
	// HelloRefused: a hello the endpoint refused; Descendant says it came
	// from the harness's tree.
	HelloRefused Kind = "hello refused"
	// CannotStart: the server of Generation reported that its command
	// cannot start.
	CannotStart Kind = "cannot start"
	// StartupFailed: Codex reported our server's startup failed, for Thread
	// when the status names one.
	StartupFailed Kind = "startup failed"
	// NotObserved: a call refused before its ticket because no hook
	// observation of it came.
	NotObserved Kind = "not observed"
	// Validated: a child validated its ticket, issued at Issued.
	Validated Kind = "validated"
	// TimerPassed: the hello timer's bound passed.
	TimerPassed Kind = "timer passed"
	// Denied: the harness denied our tool for permission. Gate L4 names
	// the signal; until it closes nothing sends this.
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
	// Generation is a connection's (Hello, Closed, Bound, CannotStart).
	Generation uint64
	// Thread is the thread an event names on Codex (the Selection kinds,
	// Bound, StartupFailed); "" for none.
	Thread string
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
// the last ticket's on Claude Code, none on Codex (history.go).
func (r *Record) prune() int64 {
	if r.Harness == Codex {
		return 0
	}
	return r.Worked.Boot
}

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
	// On Codex the ticket is its connection's conversation's proof, kept
	// in the history; Worked stays the run's fact either way.
	changed := r.Harness == Codex && r.h.ticket(e)
	if e.At.Boot > r.Worked.Boot {
		r.Worked = e.At
		if r.prune() != 0 {
			r.h.worked(e.At.Boot)
		}
		changed = true
	}
	if changed {
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

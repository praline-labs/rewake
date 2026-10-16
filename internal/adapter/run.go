package adapter

import (
	"context"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/session"
)

// The capabilities of one run, as the plan of a launch carries them
// (LaunchPlan). Each is optional, and each is a small interface over narrow
// values: none hands an adapter the session record.

// Service is what of the run's own serves the session beside the harness: a
// process that speaks for it, a listener for its answers. The host starts it
// before the harness and closes it after, in that order whatever happened
// between. A start that fails ends the launch; a service that can do without
// a part of itself says so through note and returns nil.
type Service interface {
	Start(ctx context.Context, note func(string)) error
	// Close is called whether Start succeeded or not, and cleans up what the
	// launch prepared for the service.
	Close()
}

// ProcessService is a service that runs a process of its own, which speaks
// for the harness: the run is available once that process names its
// conversation, the harness is ended when the process is gone, and the tool's
// calls may come from below it.
type ProcessService interface {
	Service
	// ProcessID is the process's pid, zero before it started.
	ProcessID() int
	// Done is closed when the process is gone.
	Done() <-chan struct{}
}

// HarnessWatcher is a service that serves the harness process itself and
// checks each request by its peer: the host names the process it started,
// with its start time, as soon as it is running. A request that comes before
// is held until then or refused; a peer that is not that process is refused
// (docs/v2/design-api.md#the-harness-process).
type HarnessWatcher interface {
	HarnessStarted(RunProcess)
}

// Workers counts work a service does on a request of its own among the run's
// workers, so the room's lease and the run's directory outlast its last
// write (docs/v2/stage3-rundir.md). Admit says whether the run still takes
// work; each admitted piece ends with one Dismiss.
type Workers interface {
	Admit() bool
	Dismiss()
}

// WorkerCounter is a service whose request handlers write the room: the host
// hands it the run's workers before it starts. A handler that outlasts the
// service's bounded close stays counted, and the run's end waits for it.
type WorkerCounter interface {
	CountWorkers(Workers)
}

// Wake brings a notice to the running session and says what happened: it
// delivers, or answers held with the harness's reason, or failed
// (docs/v2/design-api.md#wake). The host's delivery loop decides what to send
// and when; the adapter only carries it.
type Wake interface {
	Deliver(context.Context, mail.Message) mail.Result
}

// Reserver is a Wake that holds its destination through the inbox's
// publication of a notice, so what is published is what is delivered.
type Reserver interface {
	Reserve(context.Context, mail.Message) (mail.Reservation, error)
}

// Receiver is a Wake whose word on a notice it answered held comes later: the
// inbox server settles the message when it arrives.
type Receiver interface {
	Receipts() <-chan mail.Receipt
	// Opened is closed once the session can take its first notice.
	Opened() <-chan struct{}
}

// Telemetry is a run's report of its state, folded into the session-state
// snapshot.
type Telemetry interface {
	SessionState() session.Snapshot
}

// ToolTransport is a run that registers the mail tools with its harness and
// names the process that asks the host's endpoint to run their calls
// (docs/v2/design-api.md#tooltransport). D is the tool layer's descriptor,
// which this package names as a parameter because it imports no tool layer:
// the host asks for ToolTransport[tool.Descriptor].
type ToolTransport[D any] interface {
	// OfferTools hands the run the tools and the endpoint, before its
	// service starts; serve tells the endpoint which process asks.
	OfferTools(tools []D, endpoint string, serve func(pid int, start uint64))
	// ToolReadsOff says why a read through the tool cannot count, empty
	// when it can (T6).
	ToolReadsOff() string
}

// LiveReporter is a run that says which of its adapter's capabilities it has
// proven: the live set, which the host records in the session record and every
// other session decides by (docs/v2/design-api.md#two-levels-offered-and-live).
// A run without one has live what its plan carries, from its start.
type LiveReporter interface {
	// ReportLive takes where the live set goes, before the service starts.
	// The run calls it once its readiness exchange has run, with what the
	// exchange proved — nothing, if nothing — and again with the whole set
	// each time it changes: a capability lost is withdrawn at once. The calls
	// come one at a time, in the order of the changes, and never under a
	// lock a call into the run takes.
	ReportLive(live func([]Capability))
}

// ReadinessBound is how long a run's readiness exchange may take before what
// it has not proven counts as absent: a launch that waits for a capability to
// become live waits this long at most.
const ReadinessBound = 8 * time.Second

// AfterStartBound is how long a run's availability waits, from its harness's
// start, for a boundary proven only after the start (TurnBoundaryOffer's
// AfterStart): past it the run is available with its boundary not live, and
// a later proof makes it live. A first guess, until a live run measures how
// long the proof takes.
const AfterStartBound = 30 * time.Second

// Withholder is a service that knows, once it started and before the harness
// starts, a capability its adapter offers that this run will not prove, and
// why. A launch of a role that owes reports is refused on a withheld turn
// boundary, naming the cause; any other launch goes on, the cause said as a
// note (docs/v2/stage4-turns.md#readiness-and-admission).
type Withholder interface {
	Withheld() []Withheld
}

// Withheld is one capability a run will not prove, and the cause, in words
// that say where it lies and how to start the run so it does.
type Withheld struct {
	Capability Capability
	Cause      string
}

// Resumer is a harness whose launch arguments can name the conversation a
// launch resumes, so that the grants it had can be confirmed again and given
// at launch (docs/grants.md#after-a-cold-resume): for one that also offers
// directory grants, which no adapter does from S9 until stage 6 designs
// Permissions. Empty when they name none — a new conversation, a picker, a
// fork.
type Resumer interface {
	ResumedConversation(args []string) string
}

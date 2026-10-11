package adapter

import (
	"context"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// GitGrantHarness supports explicit per-message repository metadata grants.
type GitGrantHarness interface {
	SupportsGitGrant() bool
}

// DirGrantHarness takes a directory into a running session's writable roots
// for one task, and takes it back once the task is reported on
// (docs/grants.md). A harness that is not one refuses --grant-dir.
type DirGrantHarness interface {
	SupportsDirGrant() bool
}

// Resumer is a harness whose launch arguments can name the conversation a
// launch resumes, so that the grants it had can be confirmed again and given
// at launch (docs/grants.md#after-a-cold-resume): for one that is also a
// DirGrantHarness, which no adapter is from S9 until stage 6 designs
// Permissions. Empty when they name none —
// a new conversation, a picker, a fork.
type Resumer interface {
	ResumedConversation(args []string) string
}

// TurnReportingHarness is a harness whose launch plan reports every turn's
// end: a Backend, or an Observer that is a TurnReporter. Only on one can a
// session owe a report at its turn's end, so a reporting role refuses to
// launch on any other, and a task or a question to a session of another fails
// (docs/v2/stage3-steps-adapters.md, S9). A harness that is not one reports
// no turns. S10 turns this into a query of the TurnBoundary capability, and
// S11 decides by whether that capability is live.
type TurnReportingHarness interface {
	ReportsTurns() bool
}

// GrantIssuer is a harness whose session, as main, can register a grant with
// its wrapper: the commands it runs reach the wrapper's socket. A grant is
// confirmed only by the wrapper of the main that sent it
// (docs/grants.md#who-can-grant), so a main whose commands cannot reach it
// cannot grant at all.
type GrantIssuer interface {
	ReachesWrapper() bool
}

// LaunchRefuser refuses launch arguments rewake has a way of its own for, and
// names that way. The refusal is a call to change: exit 2. No adapter in the
// catalog offers one; S10 turns the launch's assertion into a capability
// query and removes this interface if none offers the refusal then.
type LaunchRefuser interface {
	RefuseLaunch(args []string) error
}

// Completion is a terminal turn outcome, independent of its transport.
type Completion struct {
	Boundary *inbox.ReadBoundary
	ID       string
	Thread   string
	Kind     inbox.Kind
	Text     string
	// Started and Ended bound the turn on the boot clock (internal/boottime),
	// zero where unknown; a pending mark counts for this turn only if it was
	// made between them.
	Started, Ended int64
}

// CompletionHandler separates nonblocking read-boundary capture from publication.
type CompletionHandler struct {
	Capture func() *inbox.ReadBoundary
	Publish func(context.Context, Completion) error
	// Confirm runs an end that the adapter can hold open, in place of
	// Publish (docs/v2/design-api.md#turnboundary): it answers "" when the
	// end was published, or the reason to continue the turn with when it was
	// held and its answer kept. The adapter continues the turn with that
	// reason and confirms its next end again. The end names its event, and
	// the same end confirmed again gets the same answer. Nil where the
	// wrapper was given none.
	Confirm func(context.Context, Completion) (string, error)
	// EndCapture captures the boundary of a turn's end through the gate
	// the mail tool's acknowledgments enter
	// (docs/mail-bridge-turns.md#a-turns-end-meets-its-calls), with the
	// moment it noted the end on the boot clock. Nil where the run has no
	// gate; Capture is then the end's boundary, as before.
	EndCapture func() (*inbox.ReadBoundary, int64)
	// Tool takes what the harness reports of the mail tool's calls in no
	// harness's terms: turns, calls seen, their results, a failed start.
	// It never waits. Nil where the run has no tool.
	Tool ToolInput
}

// Backend is an optional session-owned transport. Its implementation owns any
// helper processes and protocol state; the wrapper only manages its lifetime.
type Backend interface {
	Start(context.Context, CompletionHandler, func(string)) error
	Deliver(context.Context, inbox.Message) inbox.Result
	Thread() (string, error)
	Done() <-chan struct{}
	Close()
}

// ProcessBackend is a backend running a harness process of its own, below
// which the harness starts its tool servers.
type ProcessBackend interface {
	ProcessID() int
}

// ReservingBackend holds its transport's destination through inbox publication.
type ReservingBackend interface {
	Reserve(context.Context, inbox.Message) (inbox.Reservation, error)
}

// Lane is a session-owned delivery path for a harness without a Backend. The
// wrapper starts it before the harness and hands it every notice; what the
// harness says about a notice after Deliver returned Held comes back through
// Receipts, and the inbox server settles the message then.
type Lane interface {
	// Start prepares the lane. An error costs the receipts, never delivery:
	// Deliver then writes as a lane that hears nothing back.
	Start(context.Context) error
	Deliver(context.Context, registry.Session, inbox.Message) inbox.Result
	Receipts() <-chan inbox.Receipt
	// Opened is closed once the session can take its first notice.
	Opened() <-chan struct{}
	Close()
}

// ObservedBackend exposes optional primary state without filesystem work.
type ObservedBackend interface {
	SessionState() sessionstate.Snapshot
}

// TurnReporter is an Observer that also hears a turn's outcome, for a harness
// whose turns end where no backend sees them. The wrapper hands it, before
// Start, the handler a Backend gets.
type TurnReporter interface {
	ReportTurns(CompletionHandler)
}

// Observer is telemetry without a transport: something the harness reports to
// on its own, which the wrapper listens to and publishes like a backend's.
//
// No adapter sets one since Claude Code's telemetry hooks left in S9: the
// wrapper's use of it is reached only by its tests until S10 recasts it on
// the adapter API and stage 4 gives Claude Code a turn boundary again.
type Observer interface {
	ObservedBackend
	Start(context.Context) error
	// Close is called whether Start succeeded or not, and cleans up what the
	// launch prepared for it.
	Close()
}

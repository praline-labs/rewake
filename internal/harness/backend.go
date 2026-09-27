package harness

import (
	"context"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
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

// LaunchRefuser refuses launch arguments rewake has a way of its own for, and
// names that way. The refusal is a call to change: exit 2.
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

// TurnReporter is an Observer that also hears a turn outcome no end-of-turn
// hook reports: Claude Code runs none for a turn a person interrupted. The
// wrapper hands it, before Start, the handler a Backend gets.
type TurnReporter interface {
	ReportTurns(CompletionHandler)
}

// Observer is telemetry without a transport: something the harness reports to
// on its own, which the wrapper listens to and publishes like a backend's.
type Observer interface {
	ObservedBackend
	Start(context.Context) error
	// Close is called whether Start succeeded or not, and cleans up what the
	// launch prepared for it.
	Close()
}

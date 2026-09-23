package harness

import (
	"context"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

// GitGrantHarness supports explicit per-message repository metadata grants.
type GitGrantHarness interface {
	SupportsGitGrant() bool
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

// ObservedBackend exposes optional primary state without filesystem work.
type ObservedBackend interface {
	SessionState() sessionstate.Snapshot
}

// Observer is telemetry without a transport: something the harness reports to
// on its own, which the wrapper listens to and publishes like a backend's.
type Observer interface {
	ObservedBackend
	Start(context.Context) error
	Close()
}

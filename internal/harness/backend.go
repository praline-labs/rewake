package harness

import (
	"context"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// Completion is a terminal turn outcome, independent of its transport.
type Completion struct {
	Boundary *inbox.ReadBoundary
	ID       string
	Thread   string
	Kind     inbox.Kind
	Text     string
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

package harness

import (
	"context"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// Completion is a terminal turn outcome, independent of its transport.
type Completion struct {
	ID     string
	Thread string
	Kind   inbox.Kind
	Text   string
}

// Backend is an optional session-owned transport. Its implementation owns any
// helper processes and protocol state; the wrapper only manages its lifetime.
type Backend interface {
	Start(context.Context, func(Completion) error, func(string)) error
	Deliver(context.Context, inbox.Message) inbox.Result
	Thread() (string, error)
	Done() <-chan struct{}
	Close()
}

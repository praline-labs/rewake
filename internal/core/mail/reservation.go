package mail

import (
	"context"
)

// Reservation pins a harness destination without exposing its transport protocol.
type Reservation interface {
	Prepare(func(thread string) error) error
	Deliver(context.Context, Message) Result
	Close()
}

// Reserver binds one shared admission before its members become readable.
// No reservation and no error is a destination that holds nothing for this
// notice: the letter takes the path of a server without a Reserver.
type Reserver func(context.Context, Message) (Reservation, error)

// CheckedAnnouncer revalidates membership after slow permission preparation,
// immediately before committing a native input.
type CheckedAnnouncer interface {
	DeliverChecked(context.Context, Message, func() bool) Result
}

package inbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Reservation pins a harness destination without exposing its transport protocol.
type Reservation interface {
	Prepare(func(thread string) error) error
	Deliver(context.Context, Message) Result
	Close()
}

// Reserver binds one shared admission before its members become readable.
type Reserver func(context.Context, Message) (Reservation, error)

// prepareDelivery checks leases before reserving a destination, then rechecks under
// the mailbox lock. Waiting for native readiness never holds the mailbox lock.
func (s *Server) prepareDelivery(ctx context.Context, message *Message) (read, answered, expired bool, reservation Reservation, release func(), err error) {
	release = func() {}
	check := func() error {
		if status, ok := ReadStatus(s.Dir, s.Name, message.ID); ok && status.State == Read {
			read = true
			return nil
		}
		answered = awaitedHere(s.Dir, s.Name, *message)
		expired, err = s.answerExpired(*message, answered)
		if err == nil && answered && !expired {
			err = linkUnread(s.Dir, s.Name, message.ID)
		}
		return err
	}
	refuse := func(cause error) error {
		if IsReport(*message) {
			if err := s.lock(func() error {
				if !s.owned() {
					return ErrThreadUnavailable
				}
				if err := check(); err != nil || read || answered || expired {
					return err
				}
				return linkUnread(s.Dir, s.Name, message.ID)
			}); err != nil {
				return err
			}
			if read || answered || expired {
				return nil
			}
		}
		return fmt.Errorf("%w: %v", ErrThreadUnavailable, cause)
	}
	if err = s.lock(check); err != nil || read || answered || expired {
		return
	}
	deliveryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	release = cancel
	if s.Reserve != nil {
		reservation, err = s.Reserve(deliveryCtx, *message)
		if err != nil {
			if !errors.Is(err, ErrNotYet) {
				err = refuse(err)
			}
			return
		}

		release = sync.OnceFunc(func() { reservation.Close(); cancel() })
	}
	makeReadable := func(thread string) error {
		if thread != "" && Owed(*message) {
			if err := recordDeliveryThread(s.Dir, s.Name, message.ID, thread); err != nil {
				return err
			}
			message.DeliveryThread = thread
		}
		return linkUnread(s.Dir, s.Name, message.ID)
	}
	lock := s.lock
	if reservation != nil {
		lock = func(fn func() error) error { return s.lockWithContext(deliveryCtx, fn) }
	}
	err = lock(func() error {
		if err := check(); err != nil || read || answered || expired {
			return err
		}
		if !s.owned() {
			return fmt.Errorf("%w: session no longer owns its name", ErrThreadUnavailable)
		}
		if reservation != nil {
			return reservation.Prepare(makeReadable)
		}
		var thread string
		if s.Thread != nil && Owed(*message) {
			var err error
			thread, err = s.Thread()
			if err != nil {
				return err
			}
		}
		return makeReadable(thread)
	})
	if errors.Is(err, ErrThreadUnavailable) {
		release()
		err = refuse(err)
	}
	return
}

// CheckedAnnouncer revalidates membership after slow permission preparation,
// immediately before committing a native input.
type CheckedAnnouncer interface {
	DeliverChecked(context.Context, Message, func() bool) Result
}

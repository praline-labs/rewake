package mail

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// prepareDelivery checks leases before reserving a destination, then rechecks under
// the mailbox lock. Waiting for native readiness never holds the mailbox lock.
func (s *Server) prepareDelivery(ctx context.Context, message *Message) (read, answered, expired bool, reservation Reservation, release func(), err error) {
	release = func() {}
	check := func() error {
		// A status or a mark that cannot be read stops the pass with nothing
		// made readable: the message may be read, withdrawn, or awaited by a
		// send, and it stays pending until they can be told.
		status, known, err := ReadStatus(s.Dir, s.Name, message.ID)
		if err != nil {
			return err
		}
		if known && status.Final() {
			// Withdrawn goes the way of read: nothing is made readable, and
			// recording the read keeps the withdrawal as it is.
			read = true
			return nil
		}
		if answered, err = AwaitedHere(s.Dir, s.Name, *message); err != nil {
			return err
		}
		expired, err = s.answerExpired(*message, answered)
		if err == nil && answered && !expired {
			err = LinkUnread(s.Dir, s.Name, message.ID)
		}
		return err
	}
	refuse := func(cause error) error {
		if IsReport(*message) {
			if err := s.lockOrAlone(s.lockCtx(), func() error {
				if !s.owned() {
					return ErrThreadUnavailable
				}
				if err := check(); err != nil || read || answered || expired {
					return err
				}
				return LinkUnread(s.Dir, s.Name, message.ID)
			}); err != nil {
				return err
			}
			if read || answered || expired {
				return nil
			}
		}
		return fmt.Errorf("%w: %v", ErrThreadUnavailable, cause)
	}
	if err = s.lockOrAlone(s.lockCtx(), check); err != nil || read || answered || expired {
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
		if reservation != nil {
			release = sync.OnceFunc(func() { reservation.Close(); cancel() })
		}
	}
	makeReadable := func(thread string) error {
		if thread != "" && Owed(*message) {
			if err := recordDeliveryThread(s.Dir, s.Name, message.ID, thread); err != nil {
				return err
			}
			message.DeliveryThread = thread
			if CarriesGrant(*message) && s.PinGrant != nil {
				s.PinGrant(*message, thread)
			}
		}
		return LinkUnread(s.Dir, s.Name, message.ID)
	}
	lockCtx := s.lockCtx()
	if reservation != nil {
		lockCtx = deliveryCtx
	}
	lock := func(fn func() error) error { return s.lockOrAlone(lockCtx, fn) }
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

// recordDeliveryThread runs under the mailbox lock before the message becomes
// readable. Recording only after delivery would lose a fast read and report.
func recordDeliveryThread(dir, name, id, thread string) error {
	if err := state.EnsureSubdir(ThreadPath(dir, name)); err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(ThreadPath(dir, name), id), []byte(thread))
}

package delivery

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// prepareDelivery checks leases, then rechecks them under the mailbox lock and
// makes the message readable.
func (s *Server) prepareDelivery(message *mail.Message) (read, answered, expired bool, err error) {
	check := func() (err error) {
		read, answered, expired, err = s.admit(*message)
		return err
	}
	refuse := func(cause error) error {
		if mail.IsReport(*message) {
			if err := s.lockOrAlone(s.lockCtx(), func() (err error) {
				if !s.owned() {
					return mail.ErrThreadUnavailable
				}
				read, answered, expired, err = s.keepReport(*message)
				return err
			}); err != nil {
				return err
			}
			if read || answered || expired {
				return nil
			}
		}
		return fmt.Errorf("%w: %v", mail.ErrThreadUnavailable, cause)
	}
	if err = s.lockOrAlone(s.lockCtx(), check); err != nil || read || answered || expired {
		return
	}
	makeReadable := func(thread string) error {
		if thread != "" && mail.Owed(*message) {
			if err := recordDeliveryThread(s.Dir, s.Name, message.ID, thread); err != nil {
				return err
			}
			message.DeliveryThread = thread
		}
		return mail.LinkUnread(s.Dir, s.Name, message.ID)
	}
	err = s.lockOrAlone(s.lockCtx(), func() error {
		if err := check(); err != nil || read || answered || expired {
			return err
		}
		if !s.owned() {
			return fmt.Errorf("%w: session no longer owns its name", mail.ErrThreadUnavailable)
		}
		var thread string
		if s.Thread != nil && mail.Owed(*message) {
			var err error
			thread, err = s.Thread()
			if err != nil {
				return err
			}
		}
		return makeReadable(thread)
	})
	if errors.Is(err, mail.ErrThreadUnavailable) {
		err = refuse(err)
	}
	return
}

// admit looks at a letter before it is made readable, in a mailbox section or
// without one. A status or a mark that cannot be read stops the pass with
// nothing made readable: the message may be read, withdrawn, or awaited by a
// send, and it stays pending until they can be told. Withdrawn goes the way of
// read: nothing is made readable, and recording the read keeps the withdrawal
// as it is. An answer awaited by a send is made readable for it, unless it
// has expired.
func (s *Server) admit(message mail.Message) (read, answered, expired bool, err error) {
	status, known, err := mail.ReadStatus(s.Dir, s.Name, message.ID)
	if err != nil || known && status.Final() {
		return err == nil, false, false, err
	}
	if answered, err = mail.AwaitedHere(s.Dir, s.Name, message); err != nil {
		return false, answered, false, err
	}
	expired, err = s.answerExpired(message, answered)
	if err == nil && answered && !expired {
		err = mail.LinkUnread(s.Dir, s.Name, message.ID)
	}
	return false, answered, expired, err
}

// keepReport makes a report readable though its notice will not go out, so the
// failure that follows leaves it available to its reader, unless admit stops
// it or has made it readable for its answer already.
func (s *Server) keepReport(message mail.Message) (read, answered, expired bool, err error) {
	if read, answered, expired, err = s.admit(message); err != nil || read || answered || expired {
		return read, answered, expired, err
	}
	return false, false, false, mail.LinkUnread(s.Dir, s.Name, message.ID)
}

// recordDeliveryThread runs under the mailbox lock before the message becomes
// readable. Recording only after delivery would lose a fast read and report.
func recordDeliveryThread(dir, name, id, thread string) error {
	if err := state.EnsureSubdir(mail.ThreadPath(dir, name)); err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(mail.ThreadPath(dir, name), id), []byte(thread))
}

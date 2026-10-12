package delivery

import (
	"context"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// Answers may be consumed while native permission metadata is read. An obsolete
// snapshot must not send another input or confer grants through another member.
func (s *Server) validAnnouncement(ctx context.Context, members []mail.Message) bool {
	valid := true
	err := s.lockOrAlone(ctx, func() error {
		if !s.owned() {
			valid = false
			return nil
		}
		for _, message := range members {
			// What cannot be read is no proof the member still stands.
			status, known, err := mail.ReadStatus(s.Dir, s.Name, message.ID)
			if err != nil {
				return err
			}
			reserved, err := mail.AwaitedHere(s.Dir, s.Name, message)
			if err != nil {
				return err
			}
			expired, err := s.answerExpired(message, reserved)
			if err != nil {
				return err
			}
			if known && status.Final() || reserved || expired {
				valid = false
				return nil
			}
		}
		return nil
	})
	return err == nil && valid
}

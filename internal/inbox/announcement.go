package inbox

import "context"

// Answers may be consumed while native permission metadata is read. An obsolete
// snapshot must not send another input or confer grants through another member.
func (s *Server) validAnnouncement(ctx context.Context, members []Message) bool {
	valid := true
	err := s.lockWithContext(ctx, func() error {
		if !s.owned() {
			valid = false
			return nil
		}
		for _, message := range members {
			status, known := ReadStatus(s.Dir, s.Name, message.ID)
			reserved := awaitedHere(s.Dir, s.Name, message)
			expired, err := s.answerExpired(message, reserved)
			if err != nil {
				return err
			}
			if known && status.final() || reserved || expired {
				valid = false
				return nil
			}
		}
		return nil
	})
	return err == nil && valid
}

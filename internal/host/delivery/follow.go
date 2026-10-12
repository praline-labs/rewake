package delivery

import (
	"context"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// followEarlierRun takes over, once this run's conversation is known, what the
// earlier runs of the name owe in it, and then forgets their waits. A session
// whose harness names no conversation forgets them at once: nothing it runs
// could continue one.
func (s *Server) followEarlierRun(ctx context.Context) {
	if s.followed || s.Epoch == "" {
		return
	}
	thread := ""
	if s.Thread != nil {
		var err error
		if thread, err = s.Thread(); err != nil || thread == "" {
			return
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.lockWithContext(ctx, func() error {
		// Not followed yet: the next pass tries again, and the earlier runs'
		// waits stay until what they owe here has been taken over.
		if _, err := mail.AdoptWaits(s.Dir, s.Name, s.Epoch, thread); err != nil {
			return err
		}
		mail.SweepAwaitingLocked(s.Dir, s.Name, s.Epoch)
		return nil
	}) == nil {
		s.followed = true
	}
}

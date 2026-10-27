package delivery

import (
	"context"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// followEarlierRun forgets, once, what the earlier runs of the name were
// waited on for: this run owes none of it, and a task they read is lost to its
// sender.
func (s *Server) followEarlierRun(ctx context.Context) {
	if s.followed || s.Epoch == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.box().WithLock(ctx, func(l mail.Locked) error {
		l.SweepAwaiting()
		return nil
	}) == nil {
		s.followed = true
	}
}

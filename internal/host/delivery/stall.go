package delivery

import (
	"context"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/buildtime"
)

// StallBound is how long a worker may stay idle under a pending mark, with a
// sender still waiting and no turn since, before its wrapper wakes it once
// (mail.Box.Stalled, docs/turn-outcomes.md#a-stalled-pending). Ten minutes:
// as long as Claude Code lets one command run in the foreground, so what a
// turn could have waited out in place is not what it went idle on. A worker
// whose own background work runs past it is woken once for nothing and
// marks pending again. A build may set it with builtStall (see package buildtime) for a
// check that cannot wait ten minutes.
var StallBound = buildtime.Duration("builtStall", builtStall, 10*time.Minute)

var builtStall string

// remindStalled wakes the session once when it has stalled for Stall: the
// reminder is a letter in its own mailbox, which the next pass announces as
// any other mail. Only a session its telemetry calls idle right now is
// reminded: one working on a turn whose start nobody timed is not stalled.
// Once a stall is reminded of, this server looks no further at it; the record
// the letter leaves (mail.Box.RemindStall) keeps another image from writing a
// second.
func (s *Server) remindStalled(ctx context.Context) {
	if s.Stall <= 0 || s.Idle == nil || ctx.Err() != nil || !s.owned() {
		return
	}
	box := mail.Open(s.Dir, s.Name, s.Epoch)
	stall, ok, err := box.Stalled()
	if err != nil || !ok || stall.Op == s.reminded || time.Duration(boottime.Now()-stall.Ended) < s.Stall || !s.Idle() {
		return
	}
	if s.lockOrAlone(s.lockCtx(), func() error { return box.RemindStall(stall, time.Now()) }) == nil {
		s.reminded = stall.Op
	}
}

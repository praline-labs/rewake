package inbox

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// One reservation fences every member's readability and the group's native ACK.
// Per-member preparation borrows it; releasing one member cannot drop that fence.
type borrowedReservation struct{ Reservation }

func (borrowedReservation) Close() {}

func announcement(messages []Message) Message {
	message := messages[0]
	message.Unread, message.Latest = len(messages), nil
	if len(messages) > 1 {
		hash := sha256.New()
		for _, member := range messages {
			_, _ = fmt.Fprintf(hash, "%s\x00", member.ID)
		}
		message.ID = fmt.Sprintf("group-%x", hash.Sum(nil))
		message.Batch = messages
	}
	return message
}

func (s *Server) deliverGroup(ctx context.Context, pending []Message) {
	if len(pending) == 0 {
		return
	}
	groupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var shared Reservation
	defer func() {
		if shared != nil {
			shared.Close()
		}
	}()
	preparer := *s
	preparer.lockContext = groupCtx
	// Readiness may wait while more letters arrive. Refresh once when the shared
	// destination is acquired; after admission the request is immutable.
	refresh := func() {
		seen := make(map[string]bool, len(pending))
		for _, member := range pending {
			seen[member.ID] = true
		}
		for _, member := range preparer.pendingMessages(groupCtx) {
			if !seen[member.ID] {
				pending = append(pending, member)
			}
		}
	}
	if s.Reserve != nil {
		attempted := false
		var reserveErr error
		preparer.Reserve = func(context.Context, Message) (Reservation, error) {
			if !attempted {
				attempted = true
				shared, reserveErr = s.Reserve(groupCtx, announcement(pending))
				if reserveErr == nil {
					refresh()
				}
			}
			if reserveErr != nil {
				return nil, reserveErr
			}
			return borrowedReservation{shared}, nil
		}
	}
	var ready []Message
	for index := 0; index < len(pending); index++ {
		message := pending[index]
		if ctx.Err() != nil || !s.owned() {
			return
		}
		read, answered, expired, _, release, err := preparer.prepareDelivery(groupCtx, &message)
		release()
		if !s.owned() {
			return
		}
		if preparer.prepared(message, read, answered, expired, err) {
			ready = append(ready, message)
		}
		if index == 0 && s.Reserve == nil {
			refresh()
		}
	}
	if len(ready) == 0 || !s.owned() {
		return
	}
	message := announcement(ready)
	var result Result

	switch {
	case ctx.Err() != nil:
		result = Result{State: Failed, Detail: "session ended before grouped announcement"}
	case !s.validAnnouncement(groupCtx, ready):
		result = Result{State: Pending, Detail: "announcement membership changed before send"}
	case shared != nil:
		if checked, ok := shared.(CheckedAnnouncer); ok {
			result = checked.DeliverChecked(ctx, message, func() bool { return s.validAnnouncement(groupCtx, ready) })
		} else {
			result = shared.Deliver(ctx, message)
		}
	default:
		result = s.Deliver(ctx, message)
	}
	if shared != nil {
		shared.Close()
		shared = nil
	}
	// Remember every irreversible result before any filesystem operation can fail.
	if result.State != Pending {
		for _, member := range ready {
			outcome := result
			outcome.ReportAvailable = outcome.State == Failed && IsReport(member)
			s.outcomes[member.ID] = outcome
		}
	}
	for _, member := range ready {
		outcome := result
		outcome.ReportAvailable = outcome.State == Failed && IsReport(member)
		s.attempts[member.ID] = time.Now()
		if outcome.State == Pending {
			if s.record(member.ID, outcome) == Read {
				s.finish(member, Result{State: Read})
			}
		} else {
			s.finish(member, outcome)
		}
	}
}

func (s *Server) prepared(message Message, read, answered, expired bool, err error) bool {
	if read {
		s.finish(message, Result{State: Read})
		return false
	}
	if errors.Is(err, state.ErrMailboxBusy) {
		s.attempts[message.ID] = time.Now()
		return false
	}
	if errors.Is(err, ErrThreadUnavailable) {
		_, readableErr := os.Stat(filepath.Join(state.UnreadPath(s.Dir, s.Name), message.ID+".json"))
		s.finish(message, Result{State: Failed, Detail: err.Error(), ReportAvailable: IsReport(message) && readableErr == nil && !expired})
		return false
	}
	if err != nil {
		s.attempts[message.ID] = time.Now()
		s.record(message.ID, Result{State: Pending, Detail: "the message could not be made readable yet: " + err.Error()})
		return false
	}
	if answered {
		return false
	}
	if expired {
		s.finish(message, Result{State: Failed, Detail: "expired before the session could take it"})
		return false
	}
	return true
}

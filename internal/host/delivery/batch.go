package delivery

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func announcement(messages []mail.Message) mail.Message {
	message := messages[0]
	message.Unread = len(messages)
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

func (s *Server) deliverGroup(ctx context.Context, pending []mail.Message) {
	if len(pending) == 0 {
		return
	}
	groupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	preparer := *s
	preparer.lockContext = groupCtx
	// Letters may arrive while the first member is prepared. Refresh once after
	// it; after admission the request is immutable.
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
	var ready []mail.Message
	for index := 0; index < len(pending); index++ {
		message := pending[index]
		if ctx.Err() != nil || !s.owned() {
			return
		}
		read, answered, expired, err := preparer.prepareDelivery(&message)
		if !s.owned() {
			return
		}
		if preparer.prepared(message, read, answered, expired, err) {
			ready = append(ready, message)
		}
		if index == 0 {
			refresh()
		}
	}
	if len(ready) == 0 || !s.owned() {
		return
	}
	message := announcement(ready)
	var result mail.Result
	handed := boottime.Now()

	switch {
	case ctx.Err() != nil:
		result = mail.Result{State: mail.Failed, Detail: "session ended before grouped announcement"}
	case !s.validAnnouncement(groupCtx, ready):
		result = mail.Result{State: mail.Pending, Detail: "announcement membership changed before send"}
	default:
		result = s.Deliver(ctx, message)
	}
	s.track(message.ID, result, ready)
	if result.State == mail.Delivered {
		for _, member := range ready {
			s.announce(member.ID, handed)
		}
	}
	// Remember every irreversible result before any filesystem operation can fail.
	if result.State != mail.Pending {
		for _, member := range ready {
			outcome := result
			outcome.ReportAvailable = outcome.State == mail.Failed && mail.IsReport(member)
			s.outcomes[member.ID] = outcome
		}
	}
	for _, member := range ready {
		outcome := result
		outcome.ReportAvailable = outcome.State == mail.Failed && mail.IsReport(member)
		s.attempts[member.ID] = time.Now()
		if outcome.State == mail.Pending {
			if s.record(member.ID, outcome) {
				s.finish(member, s.outcomes[member.ID])
			}
		} else {
			s.finish(member, outcome)
		}
	}
}

func (s *Server) prepared(message mail.Message, read, answered, expired bool, err error) bool {
	if read {
		s.finish(message, mail.Result{State: mail.Read})
		return false
	}
	if errors.Is(err, state.ErrMailboxBusy) {
		s.attempts[message.ID] = time.Now()
		return false
	}
	if errors.Is(err, mail.ErrThreadUnavailable) {
		// Whether a report stays readable decides whether the failure
		// archives it away from its reader; unknown, it is not failed yet.
		readable, lookupErr := mail.IsIn(state.UnreadPath(s.Dir, s.Name), message.ID)
		if lookupErr != nil {
			s.attempts[message.ID] = time.Now()
			return false
		}
		s.finish(message, mail.Result{State: mail.Failed, Detail: err.Error(), ReportAvailable: mail.IsReport(message) && readable && !expired})
		return false
	}
	if err != nil {
		s.attempts[message.ID] = time.Now()
		s.record(message.ID, mail.Result{State: mail.Pending, Detail: "the message could not be made readable yet: " + err.Error()})
		return false
	}
	if answered {
		return false
	}
	if expired {
		s.finish(message, mail.Result{State: mail.Failed, Detail: "expired before the session could take it"})
		return false
	}
	return true
}

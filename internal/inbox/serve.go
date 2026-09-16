package inbox

import (
	"context"
	"time"
)

const (
	// pollInterval is how often a mailbox is looked at. A quarter of a second is
	// below what a person notices and far below what a model turn costs.
	pollInterval = 250 * time.Millisecond
	// retryInterval is how long a pending message waits before the next attempt.
	// Pending means the receiver cannot take it yet — a Codex session with no
	// conversation, a socket not created yet — so retrying fast buys nothing.
	retryInterval = 2 * time.Second
	// defaultTTL is how long a message may stay undelivered before it is called
	// failed. A message older than this describes a situation that has passed.
	defaultTTL = 30 * time.Minute
)

// Deliverer hands one message to the harness of this session.
type Deliverer func(ctx context.Context, message Message) Result

// Server drains one mailbox for as long as its session lives.
type Server struct {
	// Dir is the state directory.
	Dir string
	// Name is the session whose mailbox this is.
	Name string
	// Deliver hands a message to the harness.
	Deliver Deliverer
	// TTL overrides how long a message may stay pending.
	TTL time.Duration
	// Epoch identifies this run of the session name. Mail addressed to an
	// earlier run is refused rather than handed to the current one.
	Epoch string

	// attempts remembers when each pending message was last tried.
	attempts map[string]time.Time
	// settled remembers messages that already have a final status, so a failure
	// to archive one does not turn into delivering it a second time.
	settled map[string]bool
}

// Serve drains the mailbox until the context is cancelled, then refuses whatever
// is still waiting: once the session is gone, nothing will ever deliver it, and
// a sender waiting on a status deserves to hear that rather than time out.
func (s *Server) Serve(ctx context.Context) {
	s.attempts = map[string]time.Time{}
	s.settled = map[string]bool{}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.refuseWaiting("the session ended before this message could be delivered")
			return
		case <-ticker.C:
			s.drain(ctx)
		}
	}
}

// drain makes one pass over the mailbox.
func (s *Server) drain(ctx context.Context) {
	messages, err := list(s.Dir, s.Name)
	if err != nil {
		return
	}
	for _, message := range messages {
		if ctx.Err() != nil {
			return
		}
		if s.alreadySettled(message) {
			continue
		}
		if message.ToEpoch != "" && s.Epoch != "" && message.ToEpoch != s.Epoch {
			s.finish(message, Result{
				State:  Failed,
				Detail: "addressed to an earlier session that used this name",
			})
			continue
		}
		if last, tried := s.attempts[message.ID]; tried && time.Since(last) < retryInterval {
			continue
		}
		if s.expired(message) {
			s.finish(message, Result{
				State:  Failed,
				Detail: "expired before the session could take it",
			})
			continue
		}

		result := s.Deliver(ctx, message)
		s.attempts[message.ID] = time.Now()
		if result.State == Pending {
			// A pending result is written too: a sender that is waiting should
			// learn the reason now, not when the message finally lands.
			_ = writeStatus(s.Dir, s.Name, message.ID, result)
			continue
		}
		s.finish(message, result)
	}
}

// finish records the outcome and takes the message out of the waiting set.
//
// The order matters and so does remembering the outcome. Archiving can fail —
// a full disk, a done/ directory somebody replaced with a file — and a message
// left in the mailbox with nothing remembered about it is delivered again on
// the next pass, four times a second, long after its sender was told it landed.
func (s *Server) finish(message Message, result Result) {
	if err := writeStatus(s.Dir, s.Name, message.ID, result); err != nil {
		// Without a status the sender learns nothing, so the message stays in
		// the mailbox and the next pass tries the whole step again.
		return
	}
	s.settled[message.ID] = true
	delete(s.attempts, message.ID)
	_ = archive(s.Dir, s.Name, message.ID)
}

// alreadySettled reports whether this message has a final status, either from
// this run or from a previous one whose archiving did not complete.
func (s *Server) alreadySettled(message Message) bool {
	if s.settled[message.ID] {
		// The archive step is retried until the mailbox is clear again.
		_ = archive(s.Dir, s.Name, message.ID)
		return true
	}
	status, ok := ReadStatus(s.Dir, s.Name, message.ID)
	if !ok || status.State == Pending {
		return false
	}
	s.settled[message.ID] = true
	_ = archive(s.Dir, s.Name, message.ID)
	return true
}

// refuseWaiting marks everything still in the mailbox as failed.
func (s *Server) refuseWaiting(reason string) {
	messages, err := list(s.Dir, s.Name)
	if err != nil {
		return
	}
	for _, message := range messages {
		s.finish(message, Result{State: Failed, Detail: reason})
	}
}

func (s *Server) expired(message Message) bool {
	ttl := s.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}
	return time.Since(message.CreatedAt) > ttl
}

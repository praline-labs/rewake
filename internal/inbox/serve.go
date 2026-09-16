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

	// attempts remembers when each pending message was last tried.
	attempts map[string]time.Time
}

// Serve drains the mailbox until the context is cancelled, then refuses whatever
// is still waiting: once the session is gone, nothing will ever deliver it, and
// a sender waiting on a status deserves to hear that rather than time out.
func (s *Server) Serve(ctx context.Context) {
	s.attempts = map[string]time.Time{}
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
func (s *Server) finish(message Message, result Result) {
	_ = writeStatus(s.Dir, s.Name, message.ID, result)
	_ = archive(s.Dir, s.Name, message.ID)
	delete(s.attempts, message.ID)
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

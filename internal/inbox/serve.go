package inbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

const (
	// pollInterval is how often a mailbox is looked at when nothing has woken
	// the server. It is the safety net behind the watch — it retries pending
	// messages and covers a watch that could not be set up — so it can be slow.
	pollInterval = time.Second
	// retryInterval is how long a pending message waits before the next attempt.
	// Pending means the receiver cannot take it yet — a Codex session with no
	// conversation, a socket not created yet — so retrying fast buys nothing.
	retryInterval = 2 * time.Second
	// defaultTTL is how long a message may stay undelivered before it is called
	// failed. A message older than this describes a situation that has passed.
	defaultTTL = 30 * time.Minute
	// keepFinished is how long a delivered or refused message and its status are
	// kept. Long enough for a sender that came back late to read the answer,
	// short enough that a machine running for weeks does not collect a mailbox
	// full of last month's conversations.
	keepFinished = 24 * time.Hour
	// sweepInterval is how often the finished ones are looked over.
	sweepInterval = 10 * time.Minute
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
	// outcomes remembers what happened to a message the moment it happened, not
	// once the status file was written. Delivery is the part that cannot be
	// undone: if writing the status fails, the outcome has to survive anyway, or
	// the next pass delivers the same message a second time.
	outcomes map[string]Result
}

// Serve drains the mailbox until the context is cancelled, then refuses whatever
// is still waiting: once the session is gone, nothing will ever deliver it, and
// a sender waiting on a status deserves to hear that rather than time out.
func (s *Server) Serve(ctx context.Context) {
	s.attempts = map[string]time.Time{}
	s.outcomes = map[string]Result{}
	s.sweepForeign()
	s.sweepFinished()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	sweeper := time.NewTicker(sweepInterval)
	defer sweeper.Stop()

	// The watch makes the common case immediate; the ticker still runs, because
	// a pending message has to be retried on time and a watch may not exist.
	changed := watchMailbox(ctx, s.Dir, s.Name)

	for {
		select {
		case <-ctx.Done():
			s.refuseWaiting("the session ended before this message could be delivered")
			return
		case <-changed:
			s.drain(ctx)
		case <-ticker.C:
			s.drain(ctx)
		case <-sweeper.C:
			s.sweepFinished()
		}
	}
}

// sweepFinished removes the messages and statuses that have been answered long
// enough ago that nobody is coming back for them.
func (s *Server) sweepFinished() {
	cutoff := time.Now().Add(-keepFinished)
	for _, directory := range []string{state.DonePath(s.Dir, s.Name), state.InboxPath(s.Dir, s.Name)} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			// In the mailbox itself only statuses are old news; a message still
			// waiting there is answered by the TTL, not by this.
			if directory != state.DonePath(s.Dir, s.Name) && !strings.HasSuffix(entry.Name(), ".status") {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			_ = os.Remove(filepath.Join(directory, entry.Name()))
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
		if s.Epoch != "" && message.ToEpoch != s.Epoch {
			// Somebody else's mail. It is left exactly where it is: the epoch it
			// names may belong to the session that takes this name next, and a
			// wrapper on its way out refusing that session's messages is how a
			// live conversation was killed by a dead one.
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
	s.outcomes[message.ID] = result
	delete(s.attempts, message.ID)
	s.publish(message.ID, result)
}

// publish writes the outcome down and takes the message out of the waiting set.
// Both steps are retried on later passes until they hold: a message whose
// outcome is known is never delivered again, only recorded again.
func (s *Server) publish(id string, result Result) {
	if err := writeStatus(s.Dir, s.Name, id, result); err != nil {
		return
	}
	_ = archive(s.Dir, s.Name, id)
}

// alreadySettled reports whether this message has an outcome, from this run or
// from a previous one whose status or archiving did not complete.
func (s *Server) alreadySettled(message Message) bool {
	if result, known := s.outcomes[message.ID]; known {
		s.publish(message.ID, result)
		return true
	}
	status, ok := ReadStatus(s.Dir, s.Name, message.ID)
	if !ok || status.State == Pending {
		return false
	}
	s.outcomes[message.ID] = Result{State: status.State, Via: status.Via, Detail: status.Detail}
	_ = archive(s.Dir, s.Name, message.ID)
	return true
}

// refuseWaiting marks this session's own waiting mail as failed when it ends.
// A message that already has an outcome is not one of them: turning a delivered
// message into a failed one sends its sender to say the whole thing again. Nor
// is mail addressed to another epoch — that belongs to somebody else.
func (s *Server) refuseWaiting(reason string) {
	messages, err := list(s.Dir, s.Name)
	if err != nil {
		return
	}
	for _, message := range messages {
		if s.Epoch != "" && message.ToEpoch != s.Epoch {
			continue
		}
		if s.alreadySettled(message) {
			continue
		}
		s.finish(message, Result{State: Failed, Detail: reason})
	}
}

// sweepForeign refuses mail left in this mailbox for a session that used the
// name before. It runs once, at the start: this session took the name, so
// whatever was addressed to an earlier one will never be delivered. Later
// arrivals for another epoch are a different matter — they belong to whoever
// takes the name next — and those are left alone.
func (s *Server) sweepForeign() {
	if s.Epoch == "" {
		return
	}
	messages, err := list(s.Dir, s.Name)
	if err != nil {
		return
	}
	for _, message := range messages {
		if message.ToEpoch == s.Epoch || s.alreadySettled(message) {
			continue
		}
		s.finish(message, Result{
			State:  Failed,
			Detail: "addressed to an earlier session that used this name",
		})
	}
}

func (s *Server) expired(message Message) bool {
	ttl := s.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}
	return time.Since(message.CreatedAt) > ttl
}

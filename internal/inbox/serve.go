package inbox

import (
	"context"
	"errors"
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

// watch is how the server hears about new mail; replaceable in tests.
var watch = watchMailbox

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
	// Thread identifies the target conversation before a task becomes readable.
	Thread func() (string, error)
	// TTL overrides how long a message may stay pending.
	TTL time.Duration
	// Epoch identifies this run of the session name. Mail addressed to an
	// earlier run is refused rather than handed to the current one.
	Epoch string
	// Owns reports whether this session still holds its name. A server that
	// starts late — after its harness is gone and somebody else took the name —
	// must not touch that mailbox at all.
	Owns func() bool

	// attempts remembers when each pending message was last tried.
	attempts map[string]time.Time
	// stopping lifts the retry limit. Shutdown is the last chance to write down
	// an outcome that is only in memory: a status write that failed a moment
	// ago would otherwise be skipped, and the next session with this name would
	// refuse a message that was delivered.
	stopping bool
	// lockContext bounds every wait for the mailbox lock: the serving context
	// while serving, a short deadline on the way out.
	lockContext context.Context
	// outcomes remembers what happened to a message the moment it happened, not
	// once the status file was written. Delivery is the part that cannot be
	// undone: if writing the status fails, the outcome has to survive anyway, or
	// the next pass delivers the same message a second time.
	outcomes map[string]Result
}

// Serve drains the mailbox until the context is canceled, then refuses whatever
// is still waiting: once the session is gone, nothing will ever deliver it, and
// a sender waiting on a status deserves to hear that rather than time out.
func (s *Server) Serve(ctx context.Context) {
	s.attempts = map[string]time.Time{}
	s.outcomes = map[string]Result{}
	s.lockContext = ctx
	if ctx.Err() != nil || !s.owned() {
		// Canceled before it began, or the name already belongs to somebody
		// else: refusing their mail on the way past is not this session's to do.
		return
	}
	s.sweepForeign()
	s.sweepFinished()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	sweeper := time.NewTicker(sweepInterval)
	defer sweeper.Stop()

	// The watch makes the common case immediate; the ticker still runs, because
	// a pending message has to be retried on time and a watch may not exist.
	changed := watch(ctx, s.Dir, s.Name)

	for {
		select {
		case <-ctx.Done():
			s.refuseWaiting("the session ended before this message could be delivered")
			return
		case _, open := <-changed:
			if !open {
				// The watch has ended. A closed channel is always ready, and
				// reading it again and again spun a core; the ticker carries
				// on alone.
				changed = nil
				continue
			}
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
	_ = s.lock(func() error {
		s.sweepFinishedLocked()
		return nil
	})
}

func (s *Server) sweepFinishedLocked() {
	cutoff := time.Now().Add(-keepFinished)
	// Unread mail goes by age too: a notice nobody acted on for a day describes
	// a conversation that has moved on, and the mailbox of a name reused for
	// weeks would otherwise keep every one of them.
	finished := []string{state.DonePath(s.Dir, s.Name), state.UnreadPath(s.Dir, s.Name), answerReceiptsPath(s.Dir, s.Name), state.AnsweringPath(s.Dir, s.Name), threadPath(s.Dir, s.Name)}
	for _, directory := range append(finished, state.InboxPath(s.Dir, s.Name)) {
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
			if directory == state.InboxPath(s.Dir, s.Name) && !strings.HasSuffix(entry.Name(), ".status") {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if directory == state.UnreadPath(s.Dir, s.Name) {
				// Queued mail is still being served. Its readable copy records
				// acceptance and must survive a long live answer reservation.
				if _, err := os.Stat(filepath.Join(state.InboxPath(s.Dir, s.Name), entry.Name())); err == nil {
					continue
				}
			}
			if directory == threadPath(s.Dir, s.Name) && keepThreadRecord(s.Dir, s.Name, s.Epoch, entry.Name()) {
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

		// Readable first, announced second: an agent that runs rewake inbox the
		// moment it is told has to find the message there. A message linked on an
		// earlier attempt may have been read since; then there is nothing left
		// to announce.
		read, answered, expired := false, false, false
		err := s.lock(func() error {
			if status, ok := ReadStatus(s.Dir, s.Name, message.ID); ok && status.State == Read {
				read = true
				return nil
			}
			// A lease keeps the report queued, but ordinary expired mail
			// must never become visible to a reader in the first place.
			answered = awaitedHere(s.Dir, s.Name, message)
			accepted := false
			if KindOf(message) == Finished {
				_, err := os.Stat(filepath.Join(state.UnreadPath(s.Dir, s.Name), message.ID+".json"))
				accepted = err == nil
			}
			expired = s.expired(message) && !answered && !accepted
			if expired {
				return nil
			}
			if accepted && !answered {
				// Retention starts again when an accepted reply becomes
				// available for ordinary delivery after a long reservation.
				now := time.Now()
				_ = os.Chtimes(filepath.Join(state.UnreadPath(s.Dir, s.Name), message.ID+".json"), now, now)
			}
			if s.Thread != nil && Owed(message) {
				thread, err := s.Thread()
				if err != nil {
					return err
				}
				if thread != "" {
					if err := recordDeliveryThread(s.Dir, s.Name, message.ID, thread); err != nil {
						return err
					}
					message.DeliveryThread = thread
				}
			}
			if err := linkUnread(s.Dir, s.Name, message.ID); err != nil {
				return err
			}
			return nil
		})
		if read {
			s.finish(message, Result{State: Read})
			continue
		}
		if errors.Is(err, state.ErrMailboxBusy) {
			// A reader holds the mailbox; this message waits for the next pass.
			s.attempts[message.ID] = time.Now()
			continue
		}
		if err != nil {
			s.attempts[message.ID] = time.Now()
			s.record(message.ID, Result{
				State:  Pending,
				Detail: "the message could not be made readable yet: " + err.Error(),
			})
			continue
		}
		if answered {
			// Reservation is provisional: keep the queue entry and check its
			// lease again next tick, including after a crashed sender.
			continue
		}
		if expired {
			s.finish(message, Result{
				State:  Failed,
				Detail: "expired before the session could take it",
			})
			continue
		}
		// The notice names how many messages wait, this one included.
		message.Unread = countUnread(s.Dir, s.Name, s.Epoch)
		result := s.Deliver(ctx, message)
		s.attempts[message.ID] = time.Now()
		if result.State == Pending {
			// A pending result is written too: a sender that is waiting should
			// learn the reason now, not when the message finally lands. The
			// agent may have read it meanwhile, and that outcome stands.
			if s.record(message.ID, result) == Read {
				s.finish(message, Result{State: Read})
			}
			continue
		}
		s.finish(message, result)
	}
}

// refuseWaiting marks this session's own waiting mail as failed when it ends.
// A message that already has an outcome is not one of them: turning a delivered
// message into a failed one sends its sender to say the whole thing again. Nor
// is mail addressed to another epoch — that belongs to somebody else.
func (s *Server) refuseWaiting(reason string) {
	s.stopping = true
	// The serving context has ended by now. The last writes get a deadline of
	// their own: long enough for a reader to finish, short enough that a stuck
	// one cannot keep the wrapper from exiting.
	ctx, cancel := context.WithTimeout(context.Background(), shutdownLockWait)
	defer cancel()
	s.lockContext = ctx
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

// owned reports whether this session still holds its name. With no check given
// it is taken to hold it, which is the case for every caller that has one
// wrapper per mailbox.
func (s *Server) owned() bool { return s.Owns == nil || s.Owns() }

// sweepForeign refuses mail left in this mailbox for a session that used the
// name before. It runs once, at the start: this session took the name, so
// whatever was addressed to an earlier one will never be delivered. Later
// arrivals for another epoch are a different matter — they belong to whoever
// takes the name next — and those are left alone.
func (s *Server) sweepForeign() {
	if s.Epoch == "" {
		return
	}
	_ = s.lock(func() error {
		sweepAwaiting(s.Dir, s.Name, s.Epoch)
		return nil
	})
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

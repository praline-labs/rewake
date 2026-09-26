package inbox

import (
	"context"
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
	// collectionInterval is fixed from the first wake, never extended by
	// arrivals. Mail that may wait for company waits longer (window.go).
	collectionInterval = 150 * time.Millisecond
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
	// Ready signals initialized servicing; it must not wait on external work.
	Ready func()
	// Dir is the state directory.
	Dir string
	// Name is the session whose mailbox this is.
	Name string
	// Deliver hands a message to the harness.
	Deliver Deliverer
	// Reserve fences destinations across readability and delivery when supported.
	Reserve Reserver
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
	// Receipts carries what the harness says about a notice after its
	// delivery returned: how a hold ended, or that a notice taken as accepted
	// was held or refused after all. Nil for a harness that never says.
	Receipts <-chan Receipt
	// Window is how long mail that asks for nothing waits for company before
	// it is announced (window.go); zero announces at once.
	Window Window
	// Opened is closed once the harness can take its first notice. Until then
	// mail waits, pending, and goes out right after. Nil means from the start.
	Opened <-chan struct{}

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
	// held maps a held announcement to the members it carried, so a receipt
	// naming the announcement settles each of them.
	held map[string][]Message
	// recent keeps the members of notices reported delivered for a while, so
	// a late word from the harness can still take the delivery back.
	recent map[string]recentAnnouncement
	// arrivals remembers when each waiting message was first seen, which is
	// what the window mail waits in is measured from.
	arrivals map[string]*arrival
}

// Serve drains the mailbox until the context is canceled, then refuses whatever
// is still waiting: once the session is gone, nothing will ever deliver it, and
// a sender waiting on a status deserves to hear that rather than time out.
func (s *Server) Serve(ctx context.Context) {
	s.attempts = map[string]time.Time{}
	s.outcomes = map[string]Result{}
	s.held = map[string][]Message{}
	s.recent = map[string]recentAnnouncement{}
	s.arrivals = map[string]*arrival{}
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
	collection := time.NewTimer(collectionInterval)
	defer collection.Stop()
	collect, due := collection.C, time.Now().Add(collectionInterval)
	// A pass already due sooner stays as it is, so a wake never extends a
	// collection; a sooner one replaces it, so mail held for company does not
	// hold back a task that arrives behind it.
	schedule := func(after time.Duration) {
		at := time.Now().Add(after)
		if collect != nil && !at.Before(due) {
			return
		}
		collection.Reset(after)
		collect, due = collection.C, at
	}
	opened := s.Opened
	if s.Ready != nil && ctx.Err() == nil && s.owned() && state.EnsureSubdir(state.InboxPath(s.Dir, s.Name)) == nil {
		s.Ready()
	}

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
			schedule(collectionInterval)
		case <-ticker.C:
			schedule(collectionInterval)
		case <-collect:
			collect = nil
			if wait := s.drain(ctx); wait > 0 {
				schedule(wait)
			}
		case <-sweeper.C:
			s.sweepFinished()
		case <-opened:
			opened = nil
			s.retryNow()
			schedule(collectionInterval)
		case receipt := <-s.Receipts:
			s.receive(receipt)
		}
	}
}

// drain makes one pass over the mailbox and answers how long the mail it found
// may still wait for company, zero once it has gone.
func (s *Server) drain(ctx context.Context) time.Duration {
	pending := s.pendingMessages(ctx)
	if s.gated() {
		s.waitForOpening(pending)
		return 0
	}
	if wait := s.holdFor(pending, time.Now()); wait > 0 {
		s.tellWaiting(pending)
		return wait
	}
	s.deliverGroup(ctx, pending)
	return 0
}

func (s *Server) pendingMessages(ctx context.Context) []Message {
	messages, err := list(s.Dir, s.Name)
	if err != nil {
		return nil
	}
	var pending, waiting []Message
	for _, message := range messages {
		if ctx.Err() != nil || !s.owned() {
			return nil
		}
		if s.Epoch != "" && message.ToEpoch != s.Epoch {
			continue
		}
		if s.alreadySettled(message) {
			continue
		}
		waiting = append(waiting, message)
		if last, tried := s.attempts[message.ID]; tried && time.Since(last) < retryInterval {
			continue
		}
		pending = append(pending, message)
	}
	s.noteArrivals(waiting, time.Now())
	return pending
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
	// Walked from what this run holds, not from the mailbox: a notice held
	// only after it was reported delivered has no waiting copy any more.
	s.failAllHeld("the session ended while its harness still held the notice")
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
		result := Result{State: Failed, Detail: reason}
		reserved := false
		if IsReport(message) {
			// Shutdown may precede the first drain. Admission still respects
			// expiry and reservations before making the report readable.
			err := s.lock(func() error {
				if status, ok := ReadStatus(s.Dir, s.Name, message.ID); ok && status.final() {
					return nil
				}
				reserved = awaitedHere(s.Dir, s.Name, message)
				expired, err := s.answerExpired(message, reserved)
				if err != nil || expired {
					return err
				}
				if err := linkUnread(s.Dir, s.Name, message.ID); err != nil {
					return err
				}
				result.ReportAvailable = true
				return nil
			})
			if err != nil || reserved {
				continue
			}
		}
		s.finish(message, result)
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
		if s.failForeignHeld(message) {
			continue
		}
		s.finish(message, Result{
			State:  Failed,
			Detail: "addressed to an earlier session that used this name",
		})
	}
	s.sweepForeignHeld()
}

func (s *Server) expired(message Message) bool {
	ttl := s.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}
	return time.Since(message.CreatedAt) > ttl
}

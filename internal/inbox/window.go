package inbox

import (
	"fmt"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/boottime"
	"github.com/iiiokojiadbi/rewake/internal/buildtime"
)

// Letters that ask for nothing come in bursts: three workers restarted give
// main a departure and an availability notice each, seconds apart, and every
// one of them used to wake main on its own. So such mail waits a little for
// company before it is announced, and whatever arrives in that time goes out
// in the same notice. Work does not wait: a task or a question is announced
// at once, and whatever is waiting rides along with it.
//
// September 25, 2026, the owner's request: about three seconds of quiet, and a
// cap of about five from the first letter, so that a steady stream cannot put
// the notice off for ever. The cap is four: see Coalescing.

// Window is how long mail that asks for nothing waits for company. The zero
// Window announces everything at once.
type Window struct {
	// Quiet is how long held mail waits after the latest arrival.
	Quiet time.Duration
	// Cap bounds the wait from the earliest held arrival.
	Cap time.Duration
}

// Coalescing is the window a wrapper serves its mailbox with. A sender waits
// five seconds for a status by default, and a letter must be announced inside
// that wait for its send to answer delivered: the cap, plus the first
// collection, plus the harness taking the notice — up to 300 ms of silence on
// Claude Code — has to stay under five. At five the first letter of a stream
// would come back pending nearly every time; at four it does not. The cap
// counts from when the letter was written, not from when this server first saw
// it: the pass that sees it comes up to a collection later, and a send that
// began waiting at the write would lose that margin.
var Coalescing = builtWindow(Window{Quiet: 3 * time.Second, Cap: 4 * time.Second}, builtQuiet, builtCap)

// builtQuiet and builtCap, set at build with
// -ldflags "-X github.com/iiiokojiadbi/rewake/internal/inbox.builtQuiet=1500ms", replace
// Coalescing's durations. The workflow suite builds its binary so: dozens of its
// cases wait for a heads-up or a report, and at the real window each such wait
// costs three seconds more than the logic under test needs. A release build sets
// neither.
var builtQuiet, builtCap string

// builtWindow is window with the durations a build set; see package buildtime.
func builtWindow(window Window, quiet, capped string) Window {
	return Window{
		Quiet: buildtime.Duration("builtQuiet", quiet, window.Quiet),
		Cap:   buildtime.Duration("builtCap", capped, window.Cap),
	}
}

// collectingDetail is what a sender reads while its letter waits for company.
func collectingDetail(window Window) string {
	return fmt.Sprintf("waiting up to %s to be announced together with other mail that arrives meanwhile", window.Cap)
}

// arrival is when this server first saw a waiting message, the earlier of that
// and when the message was written, which the cap counts from, and whether its
// wait has been written down for its sender.
type arrival struct {
	at    time.Time
	since time.Time
	told  bool
}

// canWait reports whether a message may wait for company. Only kinds known to
// ask for nothing may: a task, a question and a kind this build does not know
// are work — AsksForWork reads an unknown kind as a task — and a sender may be
// blocked on them. So is a report a waiting send --question takes as its
// answer: it is linked for that send, not announced, and holding it holds the
// send. That look is taken without the mailbox lock: a wrong answer only moves
// when the report goes, and whether it is linked or announced is decided under
// the lock when it does. A recall does not wait either: it exists to stop work
// a preview may have started, and three seconds is time to act on it. Nor does
// an edit's replacement, whatever its kind: it sends no recall, so its own
// preview is what sets the old notice aside, and it is the one to go at once.
func (s *Server) canWait(message Message) bool {
	return !AsksForWork(message) && message.Recall == nil && message.Replaces == "" && !awaitedHere(s.Dir, s.Name, message)
}

// noteArrivals learns when each waiting message was first seen and forgets the
// ones that left the mailbox. A message tried and put off — pending, or waiting
// for its session to open — keeps the time it first came: its window ran while
// it waited, and a retry is not a new arrival.
func (s *Server) noteArrivals(waiting []Message, now time.Time) {
	if s.arrivals == nil {
		s.arrivals = map[string]*arrival{}
	}
	present := make(map[string]bool, len(waiting))
	for _, message := range waiting {
		present[message.ID] = true
		if _, seen := s.arrivals[message.ID]; !seen {
			s.arrivals[message.ID] = &arrival{at: now, since: writtenAt(message, now, boottime.Now())}
		}
	}
	for id := range s.arrivals {
		if !present[id] {
			delete(s.arrivals, id)
		}
	}
}

// writtenAt places when a message was written on this process's own clock,
// which is what the cap counts from. CreatedAt will not do by itself: it is
// the writer's wall clock, which can be stepped by seconds at any moment — a
// step forward between the write and this look cut the cap short, and split
// bursts the window exists to join. The boot clock is one for every process
// and never stepped, so a letter that carries its reading is placed by how
// long ago that was, on now's monotonic reading. One without it, from an
// earlier build, or whose reading is ahead of boot — written before a reboot
// — falls back to its wall clock, and to now when that is ahead too.
func writtenAt(message Message, now time.Time, boot int64) time.Time {
	if message.CreatedBoot != 0 && boot != 0 && boot >= message.CreatedBoot {
		return now.Add(-time.Duration(boot - message.CreatedBoot))
	}
	if !message.CreatedAt.IsZero() && message.CreatedAt.Before(now) {
		return message.CreatedAt
	}
	return now
}

// holdFor answers how much longer this pass's mail may wait for company, and
// zero when it goes now: something in it cannot wait, or the window has closed.
func (s *Server) holdFor(pending []Message, now time.Time) time.Duration {
	if s.Window == (Window{}) {
		return 0
	}
	var first, last time.Time
	for _, message := range pending {
		held, seen := s.arrivals[message.ID]
		if !seen || !s.canWait(message) {
			return 0
		}
		if first.IsZero() || held.since.Before(first) {
			first = held.since
		}
		if held.at.After(last) {
			last = held.at
		}
	}
	if first.IsZero() {
		return 0
	}
	due := last.Add(s.Window.Quiet)
	if capped := first.Add(s.Window.Cap); capped.Before(due) {
		due = capped
	}
	return max(due.Sub(now), 0)
}

// tellWaiting writes each held message's wait down once, so a sender whose own
// wait ends first prints why the letter is still pending rather than nothing.
func (s *Server) tellWaiting(pending []Message) {
	for _, message := range pending {
		if held := s.arrivals[message.ID]; held != nil && !held.told {
			held.told = true
			s.record(message.ID, Result{State: Pending, Detail: collectingDetail(s.Window)})
		}
	}
}

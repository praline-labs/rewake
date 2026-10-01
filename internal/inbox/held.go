package inbox

import (
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A harness can take a notice and keep it from the agent: Claude Code parks a
// cross-session line it will not act on yet and says so to a reply address
// (docs/delivery-adapters.md). Such a notice is held, not delivered. Its message stays
// where a delivered one would not — the waiting copy and the readable one both
// remain — until the harness reports how the hold ended, and only then is the
// message settled like any other.

// Receipt is what a harness says about a notice after its delivery attempt
// returned: how a hold ended, or that a notice it was taken to accept was held or
// refused after all. ID is the id of the announcement that was handed over,
// which for a grouped notice names the group rather than a member.
type Receipt struct {
	ID     string
	Result Result
}

// A harness that accepts silently can still speak up late, and a delivery
// counted from its silence is then taken back. So the members of a delivered
// notice are kept this long, and at most this many notices; an adapter that
// sends late words keeps its own record for as long.
const (
	LateWordWindow = time.Minute
	LateWordKeep   = 128
)

// UndeliveredNotice tells the sender of a task or question that its message
// was held and never reached the agent. Nothing is sent again on its behalf.
type UndeliveredNotice struct {
	ID     string `json:"id"`
	Kind   Kind   `json:"kind"`
	Detail string `json:"detail"`
}

// recentAnnouncement is a notice reported delivered, kept for a late word.
type recentAnnouncement struct {
	members []Message
	at      time.Time
}

// opening is what a message waiting for the harness to become ready is told.
const opening = "the session is still starting; the message goes out as soon as it can take it"

// gated reports whether the harness still cannot take its first notice.
func (s *Server) gated() bool {
	if s.Opened == nil {
		return false
	}
	select {
	case <-s.Opened:
		return false
	default:
		return true
	}
}

// waitForOpening records the wait on each message instead of delivering it.
// Nothing is made readable yet: the agent must not find mail it has not been
// told about, and a notice now would only be held.
func (s *Server) waitForOpening(pending []Message) {
	for _, message := range pending {
		s.attempts[message.ID] = time.Now()
		s.record(message.ID, Result{State: Pending, Detail: opening})
	}
}

// retryNow lifts the retry interval for every message without an outcome, so
// the mail that waited for the opening goes out on the next pass.
func (s *Server) retryNow() {
	for id := range s.attempts {
		if _, known := s.outcomes[id]; !known {
			delete(s.attempts, id)
		}
	}
}

// stillHeld keeps a held message out of delivery while noticing what changed
// underneath it: the agent may have read it on its own, and a status write that
// failed earlier is repeated.
func (s *Server) stillHeld(message Message, held Result) {
	status, ok, err := ReadStatus(s.Dir, s.Name, message.ID)
	switch {
	case err != nil:
		// Still held for this pass: the status may be final, and a later
		// pass reads it again.
	case ok && status.final():
		s.finish(message, status.result())
	case !ok || status.State != Held:
		s.publish(message.ID, held)
	}
}

// track remembers what a receipt may later name: the members of a held
// notice, and, for a harness that sends receipts, those of a delivered one.
func (s *Server) track(id string, result Result, members []Message) {
	switch {
	case result.State == Held:
		if s.held == nil {
			s.held = map[string][]Message{}
		}
		s.held[id] = members
	case result.State == Delivered && s.Receipts != nil:
		if s.recent == nil {
			s.recent = map[string]recentAnnouncement{}
		}
		now := time.Now()
		s.recent[id] = recentAnnouncement{members: members, at: now}
		s.pruneRecent(now)
	}
}

// pruneRecent forgets delivered notices past the window, then the oldest
// beyond the limit.
func (s *Server) pruneRecent(now time.Time) {
	var oldest string
	for id, recent := range s.recent {
		if now.Sub(recent.at) > LateWordWindow {
			delete(s.recent, id)
		}
	}
	for len(s.recent) > LateWordKeep {
		oldest = ""
		for id, recent := range s.recent {
			if oldest == "" || recent.at.Before(s.recent[oldest].at) {
				oldest = id
			}
		}
		delete(s.recent, oldest)
	}
}

// receive applies what the harness said about a notice after the fact. A word
// about a notice this server no longer keeps changes nothing.
func (s *Server) receive(receipt Receipt) {
	if receipt.Result.State == Pending || !s.owned() {
		return
	}
	if members, ok := s.held[receipt.ID]; ok {
		if receipt.Result.State == Held {
			return
		}
		delete(s.held, receipt.ID)
		for _, member := range members {
			if s.outcomes[member.ID].State == Held {
				s.settleHeld(member, s.outcomeFor(member, receipt.Result))
			}
		}
		return
	}
	recent, ok := s.recent[receipt.ID]
	delete(s.recent, receipt.ID)
	if !ok || time.Since(recent.at) > LateWordWindow {
		return
	}
	switch receipt.Result.State {
	case Held:
		s.takeBack(receipt, recent.members)
	case Failed:
		// Refused or expired with no hold reported first: never delivered.
		for _, member := range recent.members {
			if s.outcomes[member.ID].State == Delivered {
				s.settleHeld(member, s.outcomeFor(member, receipt.Result))
			}
		}
	}
}

// takeBack turns a delivery reported from the harness's silence into the hold
// it turned out to be. Read stays final: a member the agent has read is left
// alone. The waiting copy is gone already; the readable one carries on, and so
// the hold ends through receive or failAllHeld, never through the mailbox.
func (s *Server) takeBack(receipt Receipt, members []Message) {
	var held []Message
	for _, member := range members {
		if s.outcomes[member.ID].State != Delivered {
			continue
		}
		if isFinal(s.record(member.ID, receipt.Result)) {
			continue
		}
		s.outcomes[member.ID] = receipt.Result
		held = append(held, member)
	}
	if len(held) > 0 {
		s.track(receipt.ID, receipt.Result, held)
	}
}

func (s *Server) outcomeFor(member Message, result Result) Result {
	result.ReportAvailable = result.State == Failed && IsReport(member)
	return result
}

// settleHeld ends the hold of one message. Read stays final — the agent has the
// text whatever the harness says — and a task or question that did not arrive
// is reported to its sender, who would otherwise wait for a report nobody owes.
func (s *Server) settleHeld(message Message, outcome Result) {
	s.finish(message, outcome)
	if s.outcomes[message.ID].State == Failed && !s.outcomes[message.ID].Withdrawn {
		s.tellUndelivered(message, outcome.Detail)
	}
}

// failAllHeld is the end of the session for everything it still holds: nothing
// will release it now.
func (s *Server) failAllHeld(reason string) {
	for id, members := range s.held {
		delete(s.held, id)
		for _, member := range members {
			if s.outcomes[member.ID].State == Held {
				s.settleHeld(member, s.outcomeFor(member, Result{State: Failed, Detail: reason}))
			}
		}
	}
}

// heldByEarlier is why a message held by an earlier run of the name failed.
const heldByEarlier = "an earlier session with this name held it and ended before it was released"

// failForeignHeld settles a message an earlier run of the name held and never
// ended — it was killed. Its sender is told, as if that run had ended cleanly.
func (s *Server) failForeignHeld(message Message) bool {
	if status, ok, err := ReadStatus(s.Dir, s.Name, message.ID); err != nil || !ok || status.State != Held {
		return false
	}
	s.settleHeld(message, s.outcomeFor(message, Result{State: Failed, Detail: heldByEarlier}))
	return true
}

// sweepForeignHeld does the same for what an earlier run took back after
// reporting it delivered: only its readable copy is left.
func (s *Server) sweepForeignHeld() {
	messages, err := listIn(state.UnreadPath(s.Dir, s.Name))
	if err != nil {
		return
	}
	for _, message := range messages {
		if message.ToEpoch != s.Epoch {
			s.failForeignHeld(message)
		}
	}
}

// tellUndelivered writes the sender a note. Only work that owes a report: a
// note asks for nothing, and a report's sender is not waiting on it.
func (s *Server) tellUndelivered(message Message, detail string) {
	if !Owed(message) {
		return
	}
	// Its sender took it back: a hold that ends after that ends nothing the
	// sender still waits on. Read from disk, because the outcome in memory
	// is the harness's when the status could not be recorded under the lock.
	// A status that cannot be read may say so too, and the note is not
	// written on a guess: its sender still finds the failure under rewake
	// inbox --awaited once the status can be read.
	if status, ok, err := ReadStatus(s.Dir, s.Name, message.ID); err != nil || ok && status.Withdrawn {
		return
	}
	kind := KindOf(message)
	text := fmt.Sprintf("Rewake: your %s to %s was not delivered: %s. Send it again if it still matters.\n\nWhat was sent:\n%s",
		kind, s.Name, detail, message.Text)
	_ = PutOnce(s.Dir, Message{
		ID:          NewID(),
		From:        s.Name,
		FromEpoch:   s.Epoch,
		To:          message.From,
		ToEpoch:     message.FromEpoch,
		Kind:        Note,
		Text:        text,
		CreatedAt:   time.Now(),
		Undelivered: &UndeliveredNotice{ID: message.ID, Kind: kind, Detail: detail},
	})
}

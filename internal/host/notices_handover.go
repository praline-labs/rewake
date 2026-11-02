package host

import (
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/session"
)

// Main's notice state goes over with its run in a switch of the room's
// build: what the scan has said and what it holds, with each pending
// message's own id and time, so the next image says nothing twice — a
// receipt outlives the message in the mailbox — and loses nothing it was
// about to say. The drop takes it between two completed scans, and the scan
// waits, without scanning, until the exec, or until a failed exec resumes it
// (docs/rules/state.md#switching-a-running-room).

// Notices are main's notice state as a handover carries it.
type Notices struct {
	Available map[string]registry.Session `json:"available"`
	Known     map[string]KnownWorker      `json:"known"`
	Letters   map[string]HeldLetter       `json:"letters"`
}

// KnownWorker is a worker the scan follows.
type KnownWorker struct {
	Session     registry.Session `json:"session"`
	Last        session.Snapshot `json:"last"`
	Compactions uint64           `json:"compactions"`
	Departure   *mail.Message    `json:"departure,omitempty"`
}

// HeldLetter is a letter of a compaction main asked for, not yet gone.
type HeldLetter struct {
	Seen    time.Time     `json:"seen"`
	Gone    time.Time     `json:"gone"`
	Message *mail.Message `json:"message,omitempty"`
}

// noticeHand is how a drop takes the scan's state: take hands it over, the
// scan then holding still until resume or its end.
type noticeHand struct {
	take   chan chan *Notices
	resume chan struct{}
	handed *Notices
}

func newNoticeHand(handover *Handover) *noticeHand {
	hand := &noticeHand{take: make(chan chan *Notices), resume: make(chan struct{})}
	if handover != nil {
		hand.handed = handover.Notices
	}
	return hand
}

// hold answers a take with n and waits until the exec ends the image, a
// failed one resumes, or ctx ends; false when ctx ended.
func (hand *noticeHand) hold(done <-chan struct{}, reply chan *Notices, n *sessionNotices) bool {
	reply <- n.handed()
	select {
	case <-done:
		return false
	case <-hand.resume:
		return true
	}
}

// handed is the state as a handover carries it.
func (n *sessionNotices) handed() *Notices {
	out := &Notices{Available: map[string]registry.Session{}, Known: map[string]KnownWorker{}, Letters: map[string]HeldLetter{}}
	for id, peer := range n.available {
		out.Available[id] = peer
	}
	for id, worker := range n.known {
		out.Known[id] = KnownWorker{Session: worker.session, Last: worker.last, Compactions: worker.compactions, Departure: worker.departure}
	}
	for id, letter := range n.letters {
		out.Letters[id] = HeldLetter{Seen: letter.seen, Gone: letter.gone, Message: letter.message}
	}
	return out
}

// noticesFrom is the scan's state as an earlier image handed it over, or a
// fresh one.
func noticesFrom(handed *Notices) *sessionNotices {
	n := newSessionNotices()
	if handed == nil {
		return n
	}
	for id, peer := range handed.Available {
		n.available[id] = peer
	}
	for id, worker := range handed.Known {
		n.known[id] = &knownWorker{session: worker.Session, last: worker.Last, compactions: worker.Compactions, departure: worker.Departure}
	}
	for id, letter := range handed.Letters {
		n.letters[id] = &heldLetter{seen: letter.Seen, gone: letter.Gone, message: letter.Message}
	}
	return n
}

// takeNotices takes main's notice state from its scan, between two scans;
// a worker's run has none.
func (r *reexec) takeNotices() (*Notices, string) {
	if r.notices == nil || r.session.Role != role.Main.ID {
		return nil, ""
	}
	reply := make(chan *Notices, 1)
	select {
	case r.notices.take <- reply:
		return <-reply, ""
	case <-time.After(3 * time.Second):
		return nil, "main's notices are being scanned"
	}
}

// giveNotices lets the scan go on after an exec that did not happen.
func (r *reexec) giveNotices() {
	if r.notices == nil || r.session.Role != role.Main.ID {
		return
	}
	select {
	case r.notices.resume <- struct{}{}:
	case <-time.After(time.Second):
	}
}

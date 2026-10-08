package wrap

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/buildtime"
	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// letterWait bounds how long a letter waits for its other half: the outcome
// of a compaction and its count come apart — on Claude Code from the module
// and from the PostCompact hook, which runs in the background — and each is
// there within moments of the other unless something is broken. After it the
// letter goes with what came, as it does when the worker has gone. A build may
// set it with builtLetterWait (see package buildtime); the workflow suite does,
// since each letter its cases wait for half of waits out the whole of it.
var letterWait = buildtime.Duration("builtLetterWait", builtLetterWait, 3*time.Second)

var builtLetterWait string

// letterBound bounds how long main's wrapper waits for any word of a
// compaction main asked for while its worker lives on, and for the end of one
// its worker last said was still running. It is well past the 10 minutes a
// worker's wrapper may wait for a compaction's end, and gives a Claude Code
// compaction — whose host call rewake does not bound — as long; a compaction
// still running past it is shown by rewake list.
var letterBound = 15 * time.Minute

// heldLetter is what main's wrapper keeps of one pending compaction between
// scans: when either half of its letter was first seen, when its worker was
// first seen gone, and the letter once built, so a write that failed is
// retried with the same id.
type heldLetter struct {
	seen, gone time.Time
	message    *inbox.Message
}

// letterParts are the two halves of a letter.
type letterParts struct {
	outcome *sessionstate.CompactionOutcome
	event   *sessionstate.CompactionEvent
}

// closeRequests sends main a letter for each compaction its `rewake compact`
// left a record of — the command leaves one before it asks and keeps it on
// an answer that leaves the outcome open: started, requested or an open
// failure — and closes the record (control.Pending) once the letter is in
// main's inbox: exactly once, by the first of three:
//
//   - the outcome in the worker's snapshot, paired with the compaction its
//     telemetry counted under the same request: the letter goes when both
//     halves are there, or when one has waited letterWait; a refusal or a
//     failure has no count to wait for;
//   - the worker's run gone with neither half seen letterWait after its
//     departure was: the compaction failed, as the worker left before it
//     ended;
//   - letterBound since the request with neither half seen.
//
// An outcome of started is no outcome yet: the worker stopped waiting for the
// end with the compaction still running, and records the end when it sees it,
// as a later outcome of the same request, and the count comes with it. Until
// then the record waits as if nothing were seen, and its bound letter says
// what the worker said.
//
// A record left by an earlier run of this main is closed the same way, to the
// current run: the letter says so.
func (n *sessionNotices) closeRequests(ctx context.Context, dir string, self registry.Session) {
	open := map[string]bool{}
	for _, pending := range control.PendingOf(dir, self.Name) {
		open[pending.ID] = true
		n.closeRequest(ctx, dir, self, pending)
	}
	for id := range n.letters {
		if !open[id] {
			delete(n.letters, id)
		}
	}
}

// closeRequest closes one record if its time has come. A record its command
// still holds is left alone: the command is still asking, and its answer —
// a refusal, say — may yet close the record itself, which a letter sent now
// would contradict. The kernel lets go of a killed command's hold.
func (n *sessionNotices) closeRequest(ctx context.Context, dir string, self registry.Session, listed control.Pending) {
	pending, release, ok := control.Hold(dir, self.Name, listed.ID)
	if !ok {
		return
	}
	defer release()
	now := time.Now()
	held := n.letters[pending.ID]
	if held == nil {
		held = &heldLetter{}
		n.letters[pending.ID] = held
	}
	if held.message == nil {
		found := halves(dir, self, pending)
		var running *sessionstate.CompactionOutcome
		if found.outcome != nil && found.outcome.Outcome == control.Started {
			running, found.outcome = found.outcome, nil
		}
		reason, known := observeDeparture(dir, pending.Worker)
		gone := known && reason != ""
		some := found.outcome != nil || found.event != nil
		if some && held.seen.IsZero() {
			held.seen = now
		}
		whole := found.outcome != nil && (found.outcome.Outcome != control.Done || found.event != nil)
		var message inbox.Message
		switch {
		case whole || some && (gone || now.Sub(held.seen) >= letterWait):
			message = outcomeLetter(self, pending, found)
		case gone && (held.gone.IsZero() || now.Sub(held.gone) < letterWait):
			// A worker that finished and left at once may not have
			// published its last snapshot yet: read it again first.
			if held.gone.IsZero() {
				held.gone = now
			}
			return
		case gone:
			message = closingLetter(self, pending, fmt.Sprintf("Rewake: the compaction of %s you asked for failed (%s left before the compaction ended: %s).", pending.Worker.Name, pending.Worker.Name, reason))
		case now.Sub(pending.AskedAt) >= letterBound && running != nil:
			message = closingLetter(self, pending, fmt.Sprintf("Rewake: the compaction of %s you asked for had no outcome within %s (%s); rewake list shows whether it compacted.", pending.Worker.Name, letterBound, running.Detail))
		case now.Sub(pending.AskedAt) >= letterBound:
			message = closingLetter(self, pending, fmt.Sprintf("Rewake: no outcome of the compaction of %s you asked for was seen within %s; rewake list shows whether it compacted.", pending.Worker.Name, letterBound))
		default:
			return
		}
		held.message = &message
	}
	if putMainNotice(ctx, dir, self, *held.message, func() error { return nil }) != nil {
		return
	}
	if control.Forget(dir, self.Name, pending.ID) == nil {
		delete(n.letters, pending.ID)
	}
}

// halves finds a pending compaction's outcome and count in its worker's
// snapshot, which stays readable after the worker has gone. The last outcome
// stands, except that started — no outcome yet — never stands over a final
// one, whichever order a worker recorded them in.
func halves(dir string, self registry.Session, pending control.Pending) letterParts {
	snapshot := sessionstate.Load(dir, pending.Worker.Name, pending.Worker.Epoch())
	var found letterParts
	for i, outcome := range snapshot.CompactionOutcomes {
		if outcome.Request != pending.ID || outcome.RequestedBy != self.Name {
			continue
		}
		if outcome.Outcome == control.Started && found.outcome != nil && found.outcome.Outcome != control.Started {
			continue
		}
		found.outcome = &snapshot.CompactionOutcomes[i]
	}
	for i, event := range snapshot.CompactionEvents {
		if event.Request == pending.ID && event.RequestedBy == self.Name {
			found.event = &snapshot.CompactionEvents[i]
		}
	}
	return found
}

// closingLetter is one letter for a pending compaction: a note from the
// worker, which owes main nothing and is announced as a notify. Its id is
// derived from both runs and the request.
func closingLetter(self registry.Session, pending control.Pending, text string) inbox.Message {
	peer := pending.Worker
	identity := fmt.Sprintf("compaction-outcome\x00%s\x00%s\x00%s\x00%s\x00%s", self.Name, self.Epoch(), peer.Name, peer.Epoch(), pending.ID)
	sum := sha256.Sum256([]byte(identity))
	at := time.Now()
	if pending.AskerEpoch != self.Epoch() {
		text += " You asked for it in an earlier run of this session."
	}
	return inbox.Message{ID: fmt.Sprintf("%019d-%x", at.UnixNano(), sum[:12]), From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Note, CreatedAt: at, Text: text}
}

// outcomeLetter is the letter from what the worker's snapshot holds. It
// carries what the command used to: the token counts and the session's count
// of compactions with this one, which the "context compacted" notice it
// replaces carried.
func outcomeLetter(self registry.Session, pending control.Pending, found letterParts) inbox.Message {
	name := pending.Worker.Name
	outcome := found.outcome
	detail := ""
	if outcome != nil && outcome.Detail != "" {
		detail = " (" + outcome.Detail + ")"
	}
	var text string
	switch {
	case outcome != nil && outcome.Outcome == control.Refused:
		text = fmt.Sprintf("Rewake: %s refused the compaction you asked for: %s%s. Nothing was compacted.", name, outcome.Reason, detail)
	case outcome != nil && outcome.Outcome == control.Failed:
		text = fmt.Sprintf("Rewake: the compaction of %s you asked for failed%s.", name, detail)
	default:
		text = "Rewake: compacted " + name
		if outcome != nil && outcome.TokensBefore != nil && outcome.TokensAfter != nil {
			text += fmt.Sprintf(": %d tokens before, %d after", *outcome.TokensBefore, *outcome.TokensAfter)
		}
		if found.event == nil {
			text += "; its telemetry has not counted it, and rewake list shows the count if it does."
		} else {
			text += fmt.Sprintf(" (compaction %d).", found.event.Sequence)
		}
	}
	message := closingLetter(self, pending, text)
	if event := found.event; event != nil {
		message.Compaction = &inbox.CompactionNotice{Count: event.Sequence, ObservedAt: event.ObservedAt}
	}
	return message
}

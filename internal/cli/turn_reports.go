package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

// turnOp names a turn end's operation from its run and its event
// (docs/turn-end-recovery.md#the-operation), so a retry of the same event
// finds its own journal. A hook heard once has no event to name it by and is
// never retried: its name is drawn. The kind is part of the event: a stop
// after an Esc and the finish of the same turn are two ends.
func turnOp(self registry.Session, event turnResult) string {
	if event.ID == "" {
		return inbox.NewID()
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		self.Epoch(), event.ID, event.kind(),
		strconv.FormatInt(event.Started, 10), strconv.FormatInt(event.Ended, 10),
	}, "\x00")))
	return fmt.Sprintf("%x", sum[:16])
}

// publishTurnContext writes a turn end's journal before any of its effects
// and completes it: the journal is the one source of what the end publishes,
// takes, clears and records (inbox/journal.go). The caller holds the mailbox
// lock and has run the barrier, so no journal of this run is unfinished.
//
// The scope is the event's (docs/turn-end-recovery.md#the-operation): waits
// and the kept answer at or below its read boundary, the pending mark in its
// window. mark is that mark, nil when there is none.
func publishTurnContext(ctx context.Context, dir string, self registry.Session, event turnResult, currentThread string, waiters []inbox.Waiter, op string, mark *inbox.Mark) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var through *uint64
	if event.Boundary != nil {
		through = &event.Boundary.Through
	}
	// The answer of a turn end held for confirmation is the report the
	// session gave; this end, its continuation or what cut the continuation
	// short, adds to it. A failure or a stop leads, as the preview, and the
	// answer follows. One that cannot be read stops the end here: published
	// without it, the report would lose the answer.
	held, version, kept, err := inbox.KeptAnswerThrough(dir, self.Name, self.Epoch(), through)
	if err != nil {
		return err
	}
	if kept {
		if event.Failed || event.Stopped {
			event.Text = joinTurnText(event.Text, held)
		} else {
			event.Text = joinTurnText(held, event.Text)
		}
	}
	pending := false
	if mark != nil {
		// Only a normal finish is softened: a failure or a stop says more
		// than "still working", and stays what it is. The mark's line comes
		// first, as the preview; the turn's own text follows, since an agent
		// may have put its findings there.
		text := mark.Text
		if turn := strings.TrimSpace(event.Text); turn != "" {
			text += "\n\n" + turn
		}
		pending, event.Text = true, text
	}
	reports, reported, err := prepareTurnReports(dir, self, event, pending, currentThread, waiters, op)
	if err != nil {
		return err
	}
	journal := inbox.TurnJournal{Epoch: self.Epoch(), Op: op, Ended: event.Ended, Reports: reports, Mark: mark}
	if !event.Stopped && !pending {
		journal.Clear = reported
	}
	// What the next unmarked turn end is asked about, recorded by the journal
	// with the rest. A stop says nothing of the work and leaves it as it was;
	// an end whose time is not known has no place among its run's ends, and
	// leaves the record in place for the next end heard.
	if event.Ended != 0 {
		switch {
		case pending:
			journal.Interim = &mark.Text
		case !event.Stopped:
			journal.Settles = true
		}
	}
	if kept {
		// Taken once published, and only this version of it: an answer kept
		// after it is a later turn end's.
		journal.Kept = &version
	}
	// Every end whose time is known writes one, done too: its Ended opens the
	// next end's window (inbox.TurnWindowStart).
	if event.Ended == 0 && len(journal.Reports) == 0 && len(journal.Clear) == 0 && journal.Kept == nil {
		return nil
	}
	if err := inbox.WriteJournal(dir, self.Name, op, journal); err != nil {
		return err
	}
	// The barrier completes it, as any journal: its plan reads what every
	// effect will decide by before the first one.
	return inbox.Reconcile(ctx, dir, self.Name)
}

func prepareTurnReports(dir string, self registry.Session, event turnResult, pending bool, currentThread string, waiters []inbox.Waiter, op string) ([]inbox.Message, []inbox.Waiter, error) {
	if !event.Failed && !event.Stopped && strings.TrimSpace(event.Text) == "" {
		if len(waiters) == 0 {
			return nil, nil, nil
		}
		event.Failed = true
		event.Text = ""
	}
	if !event.Failed && !event.Stopped && (role.Of(self.Role).Silent || len(waiters) == 0) {
		return nil, nil, nil
	}
	kind := inbox.Finished
	switch {
	case event.Stopped:
		kind = inbox.Stopped
	case event.Failed:
		kind = inbox.Error
	case pending:
		kind = inbox.Interim
	}
	var reports []inbox.Message
	for _, waiter := range waiters {
		// A report to an ended run can reach nobody.
		peer, err := registry.Lookup(dir, waiter.Name)
		if err != nil && !errors.Is(err, registry.ErrNotFound) {
			return nil, nil, err
		}
		if err != nil || peer.Epoch() != waiter.Epoch {
			continue
		}
		id := inbox.ReportID(self.Name, self.Epoch(), waiter)
		if event.Stopped {
			id += "-stopped-" + op
		}
		if pending {
			// Its own id, so the report that settles the wait later is not
			// taken for a copy of this one.
			id += "-pending-" + op
		}
		reports = append(reports, inbox.Message{ID: id, From: self.Name, FromEpoch: self.Epoch(), To: waiter.Name, ToEpoch: waiter.Epoch, Kind: kind, Text: event.Text, InReplyTo: waiter.Messages, CreatedAt: time.Now(), ThreadChanged: inbox.ReportThreadChanged(dir, self.Name, waiter.Messages, currentThread)})
	}
	// A failure nobody waits on still goes to main, which has to learn that a
	// session is broken. A stop does not: a person interrupting a turn no
	// rewake task depends on is their own business, and the session turning
	// idle says all there is to say (the owner's decision of 23.09.2026,
	// docs/turn-outcomes.md).
	if event.Failed && !event.Stopped && len(reports) == 0 {
		target := self
		if role.Of(self.Role).ID != role.Main.ID {
			sessions, err := registry.List(dir)
			if err != nil {
				return nil, nil, err
			}
			for _, candidate := range sessions {
				if role.Of(candidate.Role).ID == role.Main.ID {
					target = candidate
					break
				}
			}
		}
		reports = append(reports, inbox.Message{ID: reportOfOp(op), From: self.Name, FromEpoch: self.Epoch(), To: target.Name, ToEpoch: target.Epoch(), Kind: kind, Text: event.Text, CreatedAt: time.Now()})
	}
	return reports, waiters, nil
}

// reportOfOp names the report of an operation that answers nothing. The
// journal records it before it is published, so a retry takes it from there;
// the time in front keeps the recipient's mailbox in the order mail came in.
func reportOfOp(op string) string {
	if strings.Contains(op, "-") {
		// Drawn already in that form, for a hook heard once.
		return op
	}
	return fmt.Sprintf("%019d-%s", time.Now().UnixNano(), op[:12])
}

package mail

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// turnOp names a turn end's operation from its run and its event
// (docs/delivery-turn-end.md#the-operation), so a retry of the same event
// finds its own journal. An end that names no event — none a harness's turn
// boundary hands over since the Stop hook left in S9, only the tests' — is
// never retried: its name is drawn. The kind is part of the event: a stop
// after an Esc and the finish of the same turn are two ends.
func turnOp(self registry.Session, event TurnEnd) string {
	if event.ID == "" {
		return NewID()
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		self.Epoch(), event.ID, event.Kind(),
		strconv.FormatInt(event.Started, 10), strconv.FormatInt(event.Ended, 10),
	}, "\x00")))
	return fmt.Sprintf("%x", sum[:16])
}

// publishTurnContext makes a turn end's effects in one order under the
// mailbox lock the caller holds (docs/delivery-turn-end.md): every report is
// written, then the waits they answer are cleared, the kept answer they carry
// is taken and the end's word recorded, and last the end is recorded done
// (journal.go). Writing before clearing is the order that matters: a wait
// cleared counts as answered (awaited.go), so the other order, cut short,
// would leave an answer on record and no letter. A failure returns at the
// step it met, and a retry of the same event runs the steps again from what
// the disk says: the wait still names what is owed, a report's id is fixed
// by its wait, and a report found written is not written again.
//
// The scope is the event's (docs/delivery-turn-end.md#the-operation): waits
// and the kept answer at or below its read boundary, the pending mark in its
// window. mark is that mark, nil when there is none.
func publishTurnContext(ctx context.Context, dir string, self registry.Session, event TurnEnd, currentThread string, waiters []Waiter, op string, mark *Mark) error {
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
	held, kept, err := KeptAnswerThrough(dir, self.Name, self.Epoch(), through)
	if err != nil {
		return err
	}
	if kept {
		if event.Failed || event.Stopped {
			event.Text = joinTurnText(event.Text, held.Text)
		} else {
			event.Text = joinTurnText(held.Text, event.Text)
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
	for _, report := range reports {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeTurnReport(dir, self.Name, report); err != nil {
			return fmt.Errorf("could not write the report %s to %s: %w", report.ID, report.To, err)
		}
	}
	if !event.Stopped && !pending {
		for _, waiter := range reported {
			if err := ClearAwaiting(dir, self.Name, self.Epoch(), waiter); err != nil {
				return fmt.Errorf("could not clear what %s was owed: %w", waiter.Name, err)
			}
		}
	}
	if kept {
		// Taken once published, and only this version of it: an answer kept
		// after it is a later turn end's.
		if err := live(dir).dropKeptVersion(self.Name, self.Epoch(), held.Version); err != nil {
			return err
		}
	}
	// What the next unmarked turn end is asked about. A stop says nothing of
	// the work and leaves it as it was; an end whose time is not known has no
	// place among its run's ends, and leaves the record in place for the next
	// end heard.
	if event.Ended != 0 {
		var line *string
		if pending {
			line = &mark.Text
		}
		if err := recordInterim(dir, self.Name, self.Epoch(), op, event.Ended, line, !pending && !event.Stopped); err != nil {
			return err
		}
	}
	// Every end whose time is known is recorded, one that did nothing too:
	// its Ended opens the next end's window (TurnWindowStart). The held end
	// is named beside it, so that end confirmed again learns it was published
	// with this one (HeldEndTaken).
	if event.Ended == 0 && len(reports) == 0 && len(reported) == 0 && !kept {
		return nil
	}
	done := TurnJournal{Epoch: self.Epoch(), Op: op, Ended: event.Ended, Done: true}
	if kept {
		done.Held = held.Held
	}
	return writeDoneJournal(dir, self.Name, op, done)
}

// writeTurnReport writes one report into its recipient's mailbox unless it is
// there already, or the recipient's wait was answered by a report of this
// name already: a run that resumed a conversation and took over a wait an
// earlier run answered (answeredAlready). A failed main keeps its own report
// without waking itself into another failing turn.
func writeTurnReport(dir, name string, report Message) error {
	if report.To == name && len(report.InReplyTo) == 0 {
		return PutLocal(dir, report)
	}
	found, err := present(dir, report.To, report.ID)
	if found || err != nil {
		// A stage that cannot be searched may hold it: a second copy is the
		// outcome that cannot be taken back.
		return err
	}
	if kind := KindOf(report); len(report.InReplyTo) > 0 && (kind == Finished || kind == Error) {
		answered, err := answeredAlready(dir, report.To, report.ToEpoch, name, report.InReplyTo)
		if err != nil || answered {
			return err
		}
	}
	return Put(dir, report)
}

func prepareTurnReports(dir string, self registry.Session, event TurnEnd, pending bool, currentThread string, waiters []Waiter, op string) ([]Message, []Waiter, error) {
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
	kind := Finished
	switch {
	case event.Stopped:
		kind = Stopped
	case event.Failed:
		kind = Error
	case pending:
		kind = Interim
	}
	var reports []Message
	for _, waiter := range waiters {
		// A report to an ended run can reach nobody. Read without the name
		// lock's cleanup, which a holder of a mailbox lock must not enter.
		peer, err := registry.LookupReadOnly(dir, waiter.Name)
		if err != nil && !errors.Is(err, registry.ErrNotFound) {
			return nil, nil, err
		}
		if err != nil || peer.Epoch() != waiter.Epoch {
			continue
		}
		id := ReportID(self.Name, self.Epoch(), waiter)
		if event.Stopped {
			id += "-stopped-" + op
		}
		if pending {
			// Its own id, so the report that settles the wait later is not
			// taken for a copy of this one.
			id += "-pending-" + op
		}
		reports = append(reports, Message{ID: id, From: self.Name, FromEpoch: self.Epoch(), To: waiter.Name, ToEpoch: waiter.Epoch, Kind: kind, Text: event.Text, InReplyTo: waiter.Messages, CreatedAt: time.Now(), ThreadChanged: ReportThreadChanged(dir, self.Name, waiter.Messages, currentThread)})
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
		id, err := reportOfOp(dir, target.Name, op)
		if err != nil {
			return nil, nil, err
		}
		reports = append(reports, Message{ID: id, From: self.Name, FromEpoch: self.Epoch(), To: target.Name, ToEpoch: target.Epoch(), Kind: kind, Text: event.Text, CreatedAt: time.Now()})
	}
	return reports, waiters, nil
}

// reportOfOp names the report of an operation that answers nothing. A retry
// of the operation finds the one an earlier attempt wrote by the operation in
// its tail; the time in front keeps the recipient's mailbox in the order mail
// came in.
func reportOfOp(dir, to, op string) (string, error) {
	if strings.Contains(op, "-") {
		// Drawn already in that form, for an end that named no event.
		return op, nil
	}
	tail := "-" + op[:12] + ".json"
	for _, directory := range []string{state.InboxPath(dir, to), state.UnreadPath(dir, to), state.DonePath(dir, to)} {
		entries, err := os.ReadDir(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), tail) {
				return strings.TrimSuffix(entry.Name(), ".json"), nil
			}
		}
	}
	return fmt.Sprintf("%019d-%s", time.Now().UnixNano(), op[:12]), nil
}

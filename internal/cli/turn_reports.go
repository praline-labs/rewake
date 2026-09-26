package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

// The mailbox lock serializes preparation and clearing. Persist the entire
// batch before its first publication so retries cannot absorb later work.
func publishTurnContext(ctx context.Context, dir string, self registry.Session, event turnResult, currentThread string, waiters []inbox.Waiter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	receipt, path, err := loadTurnReceipt(dir, self, event)
	if err != nil {
		return err
	}
	if !receipt.Done {
		if !receipt.Prepared {
			// The answer of a turn end held for confirmation is the report
			// the session gave; this end, its continuation or what cut the
			// continuation short, adds to it. A failure or a stop leads, as
			// the preview, and the answer follows.
			if held, ok := inbox.KeptAnswer(dir, self.Name, self.Epoch()); ok {
				if event.Failed || event.Stopped {
					event.Text = joinTurnText(event.Text, held)
				} else {
					event.Text = joinTurnText(held, event.Text)
				}
			}
			// Once per turn end: a retry of the same turn finds the receipt
			// prepared and does not look at a later turn's mark.
			line, pending, err := inbox.TakePending(dir, self.Name, self.Epoch(), event.Started, event.Ended)
			if err != nil {
				return err
			}
			text, mark := line, ""
			if pending && !event.Failed && !event.Stopped {
				// Only a normal finish is softened: a failure or a stop says
				// more than "still working", and stays what it is. The mark's
				// line comes first, as the preview; the turn's own text follows,
				// since an agent may have put its findings there.
				if turn := strings.TrimSpace(event.Text); turn != "" {
					text += "\n\n" + turn
				}
				event.Pending, event.Text, mark = true, text, line
			}
			receipt.Reports, receipt.Waiters, err = prepareTurnReports(dir, self, event, currentThread, waiters, receipt.ID)
			if err != nil {
				return err
			}
			receipt.Prepared = true
			receipt.KeepWaiters = event.Stopped || event.Pending
			receipt.Interim = event.Pending
			if err := saveTurnReceipt(path, receipt); err != nil {
				return err
			}
			// What the next unmarked turn end is asked about. A stop says
			// nothing of the work and leaves it as it was.
			switch {
			case event.Pending:
				_ = inbox.NoteInterim(dir, self.Name, self.Epoch(), mark)
			case !event.Stopped:
				_ = inbox.ClearInterim(dir, self.Name)
			}
		}
		for _, report := range receipt.Reports {
			if err := ctx.Err(); err != nil {
				return err
			}
			peer, err := registry.Lookup(dir, report.To)
			if err != nil && !errors.Is(err, registry.ErrNotFound) {
				return err
			}
			if err != nil || peer.Epoch() != report.ToEpoch {
				continue
			}
			if report.To == self.Name && len(report.InReplyTo) == 0 {
				// A failed main must not wake itself into another failing turn.
				err = inbox.PutLocal(dir, report)
			} else {
				err = inbox.PutOnce(dir, report)
			}
			if err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		receipt.Done = true
		if err := saveTurnReceipt(path, receipt); err != nil {
			return err
		}
		// Published with this end; kept until now, so an end that failed on
		// the way leaves it to the next.
		_ = inbox.DropKeptAnswer(dir, self.Name)
	}
	if receipt.KeepWaiters {
		return nil
	}
	for _, waiter := range receipt.Waiters {
		if err := ctx.Err(); err != nil {
			return err
		}
		inbox.ClearAwaiting(dir, self.Name, self.Epoch(), waiter)
	}
	return nil
}

func prepareTurnReports(dir string, self registry.Session, event turnResult, currentThread string, waiters []inbox.Waiter, fallbackID string) ([]inbox.Message, []inbox.Waiter, error) {
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
		kind = errorKind.kind
	case event.Pending:
		kind = inbox.Interim
	}
	var reports []inbox.Message
	for _, waiter := range waiters {
		peer, err := registry.Lookup(dir, waiter.Name)
		if err != nil && !errors.Is(err, registry.ErrNotFound) {
			return nil, nil, err
		}
		if err != nil || peer.Epoch() != waiter.Epoch {
			continue
		}
		id := inbox.ReportID(self.Name, self.Epoch(), waiter)
		if event.Stopped {
			id += "-stopped-" + fallbackID
		}
		if event.Pending {
			// Its own id, so the report that settles the wait later is not
			// taken for a copy of this one.
			id += "-pending-" + fallbackID
		}
		reports = append(reports, inbox.Message{ID: id, From: self.Name, FromEpoch: self.Epoch(), To: peer.Name, ToEpoch: peer.Epoch(), Kind: kind, Text: event.Text, InReplyTo: waiter.Messages, CreatedAt: time.Now(), ThreadChanged: inbox.ReportThreadChanged(dir, self.Name, waiter.Messages, currentThread)})
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
		reports = append(reports, inbox.Message{ID: fallbackID, From: self.Name, FromEpoch: self.Epoch(), To: target.Name, ToEpoch: target.Epoch(), Kind: kind, Text: event.Text, CreatedAt: time.Now()})
	}
	return reports, waiters, nil
}

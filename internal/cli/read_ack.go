package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// The read boundary. A part counts as shown only on evidence that the model
// got the whole result carrying it; a letter is read once every part of its
// version was. The wrapper's observer is what sees that evidence, and it calls
// AcknowledgeRead: there is no command an agent could run to the same effect.

// ErrNotWhole says the evidence does not prove a whole result: a truncated,
// persisted or failed result, one returned inside a script, one over the
// calibrated bound, or one whose text is not the answer the call recorded
// acknowledges nothing.
var ErrNotWhole = errors.New("the result is not proven to have reached the model whole")

// ErrTurnEnded says the call's turn ended before the acknowledgment could
// commit: its end is on record or noted by the wrapper, so nothing was
// written and the letters show again.
var ErrTurnEnded = errors.New("the call's turn ended before its read was acknowledged")

// ackBudget bounds what an acknowledgment waits for before its check: the
// record's lock and the mailbox lock together. It is never checked after: a
// write begun is finished (docs/mail-bridge-turns.md#waits-and-their-bounds).
const ackBudget = 2 * time.Second

// AcknowledgeRead records that the tool call named in the evidence reached the
// model whole, and marks read each letter of the read whose parts are now all
// acknowledged. The evidence must carry the very text the call recorded, by
// digest, before printing it. It meets the call's turn end under the mailbox
// lock: an end on record at or after the call, or one gate noted, and nothing
// is written, the receipt included; otherwise gate holds it as writing until
// it leaves, which it does before releasing the lock (rules 7 and 8). It is
// idempotent: an acknowledgment seen twice changes nothing the second time,
// and a letter is marked once. A nil gate stands for a process that captures
// no ends.
func AcknowledgeRead(dir, name, epoch, token string, evidence bridge.Exposure, gate bridge.EndGate) error {
	if !evidence.Whole() {
		return ErrNotWhole
	}
	session, err := registry.LookupReadOnly(dir, name)
	if err != nil {
		return fmt.Errorf("could not read the record of %s: %w", name, err)
	}
	if session.Epoch() != epoch {
		return fmt.Errorf("the run %s of %s has ended", epoch, name)
	}
	budget, cancel := context.WithTimeout(context.Background(), ackBudget)
	defer cancel()
	release, err := receipt.Lock(budget, dir, name, epoch, token)
	if err != nil {
		return err
	}
	defer release()
	record, err := receipt.Load(dir, name, epoch, token)
	if err != nil {
		return err
	}
	if record.Read == nil {
		return fmt.Errorf("the receipt %s is not a read", token)
	}
	calledBoot, ok := answeredBy(record.Read, evidence)
	if !ok {
		return ErrNotWhole
	}
	carried := func(_, _ int, shown receipt.Shown) bool {
		return shown.CallID == evidence.CallID && shown.Transport != receipt.Shell
	}
	reports := !role.Of(session.Role).Silent
	var ended bool
	err = state.WithMailboxLock(budget, dir, name, func() error {
		// The check: before the first write, the receipt's included.
		if err := inbox.MailboxStopped(dir, name); err != nil {
			return err
		}
		onRecord, err := inbox.EndedSince(dir, name, epoch, calledBoot)
		if err != nil {
			return err
		}
		leave := func() {}
		if !onRecord && gate != nil {
			var open bool
			if leave, open = gate.Enter(calledBoot); !open {
				onRecord = true
			}
		}
		if onRecord {
			ended = true
			return nil
		}
		// Past the check, whatever the writes do, the gate learns where
		// they ended before the lock goes.
		defer leave()
		acknowledge(record.Read, carried)
		if err := markWhole(readSite{dir: dir, self: session, epoch: epoch}, record.Read, reports); err != nil {
			_ = receipt.Save(dir, name, record)
			return err
		}
		return receipt.Save(dir, name, record)
	})
	if ended {
		return ErrTurnEnded
	}
	if err != nil {
		return fmt.Errorf("the letters were shown, but recording that they were read failed, so they stay unread and show again: %w", err)
	}
	return nil
}

// answeredBy says whether the evidence is the answer the call recorded with
// the parts it carried, and the call's time: every entry of the call names the
// digest of exactly the text in the result. An entry with no digest was
// recorded by a build that kept none, and proves nothing.
func answeredBy(batch *receipt.ReadBatch, evidence bridge.Exposure) (int64, bool) {
	if evidence.Answer == nil {
		return 0, false
	}
	digest := bridge.AnswerDigest(evidence.Answer)
	var calledBoot int64
	found := false
	for _, letter := range batch.Letters {
		for _, part := range letter.Parts {
			for _, shown := range part.Shown {
				if shown.CallID != evidence.CallID || shown.Transport == receipt.Shell {
					continue
				}
				if shown.Answer == "" || shown.Answer != digest || shown.CalledBoot <= 0 {
					return 0, false
				}
				calledBoot, found = shown.CalledBoot, true
			}
		}
	}
	return calledBoot, found
}

// acknowledge marks acknowledged each part a matching call carried.
func acknowledge(batch *receipt.ReadBatch, carried func(letter, part int, shown receipt.Shown) bool) {
	for index := range batch.Letters {
		letter := &batch.Letters[index]
		for number := range letter.Parts {
			for _, shown := range letter.Parts[number].Shown {
				if carried(index, number, shown) {
					letter.Parts[number].Acked = true
				}
			}
		}
	}
}

// settleLetters acknowledges each part a matching call carried and marks read
// every letter that is now whole: the shell's way, where printed is read. The
// caller holds the record.
func settleLetters(site readSite, record *receipt.Record, carried func(letter, part int, shown receipt.Shown) bool) error {
	acknowledge(record.Read, carried)
	reports := !role.Of(site.self.Role).Silent
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	err := state.WithMailboxLock(wait, site.dir, site.self.Name, func() error {
		// A read frozen before the mailbox stopped is not marked read after.
		if err := inbox.MailboxStopped(site.dir, site.self.Name); err != nil {
			return err
		}
		return markWhole(site, record.Read, reports)
	})
	if saveErr := receipt.Save(site.dir, site.self.Name, *record); err == nil {
		err = saveErr
	}
	if err != nil {
		return failf("the letters were shown, but recording that they were read failed, so they stay unread and show again: %v", err)
	}
	return nil
}

// markWhole marks read each letter whose parts are all acknowledged. The
// caller holds the mailbox lock.
func markWhole(site readSite, batch *receipt.ReadBatch, reports bool) error {
	for index := range batch.Letters {
		letter := &batch.Letters[index]
		if letter.Read || !letter.Complete() {
			continue
		}
		if !inbox.StillUnread(site.dir, site.self.Name, letter.ID) {
			// Read already, through another call or the shell: a letter
			// shown in part leaves unread/ only so. Marking the stored copy
			// again would owe its report a second time once the first
			// status is swept.
			letter.Read = true
			continue
		}
		var message inbox.Message
		if err := json.Unmarshal(letter.Message, &message); err != nil {
			return fmt.Errorf("the read's copy of %s is not readable: %w", letter.ID, err)
		}
		if err := inbox.MarkRead(site.dir, site.self.Name, site.epoch, message, reports); err != nil {
			return err
		}
		letter.Read = true
	}
	return nil
}

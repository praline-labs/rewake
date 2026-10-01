package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
// persisted or failed result, one returned inside a script, or one over the
// calibrated bound acknowledges nothing.
var ErrNotWhole = errors.New("the result is not proven to have reached the model whole")

// AcknowledgeRead records that the tool call named in the evidence reached the
// model whole, and marks read each letter of the read whose parts are now all
// acknowledged. It is idempotent: an acknowledgment seen twice changes nothing
// the second time, and a letter is marked once.
func AcknowledgeRead(dir, name, epoch, token string, evidence bridge.Exposure) error {
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
	site := readSite{dir: dir, self: session, epoch: epoch}
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	release, err := receipt.Lock(wait, dir, name, epoch, token)
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
	return settleLetters(site, &record, func(_, _ int, shown receipt.Shown) bool {
		return shown.CallID == evidence.CallID && shown.Transport != receipt.Shell
	})
}

// settleLetters acknowledges each part a matching call carried and marks read
// every letter that is now whole. The caller holds the record.
func settleLetters(site readSite, record *receipt.Record, carried func(letter, part int, shown receipt.Shown) bool) error {
	batch := record.Read
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
	reports := !role.Of(site.self.Role).Silent
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	err := state.WithMailboxLock(wait, site.dir, site.self.Name, func() error {
		// A read frozen before the mailbox stopped is not marked read after.
		if err := inbox.MailboxStopped(site.dir, site.self.Name); err != nil {
			return err
		}
		for index := range batch.Letters {
			letter := &batch.Letters[index]
			if letter.Read || !letter.Complete() {
				continue
			}
			if !inbox.StillUnread(site.dir, site.self.Name, letter.ID) {
				// Read already, through another call or the shell: a
				// letter shown in part leaves unread/ only so. Marking the
				// stored copy again would owe its report a second time
				// once the first status is swept.
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
	})
	if saveErr := receipt.Save(site.dir, site.self.Name, *record); err == nil {
		err = saveErr
	}
	if err != nil {
		return failf("the letters were shown, but recording that they were read failed, so they stay unread and show again: %v", err)
	}
	return nil
}

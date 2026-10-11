package cli

import (
	"context"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A read settled from cli's side: the wrapper's evidence turned into the
// core's acknowledgment, and the shell's read, where printed is read.

// AcknowledgeRead records that the tool call named in the evidence reached the
// model whole: mail.Acknowledge, given the wrapper's observation in the core's
// terms. Whether the result is whole is decided first, as before anything is
// read, and the digest of its text is only compared once the record is.
func AcknowledgeRead(dir, name, epoch, token string, evidence bridge.Exposure, gate bridge.EndGate) error {
	read := mail.ReadEvidence{CallID: evidence.CallID, Whole: evidence.Whole()}
	if evidence.Answer != nil {
		read.AnswerDigest = bridge.AnswerDigest(evidence.Answer)
	}
	return mail.Acknowledge(dir, name, epoch, token, read, gate)
}

// settleLetters acknowledges each part a matching call carried and marks read
// every letter that is now whole: the shell's way, where printed is read. The
// caller holds the record.
func settleLetters(site readSite, record *receipt.Record, carried func(letter, part int, shown receipt.Shown) bool) error {
	mail.AcknowledgeParts(record.Read, carried)
	reports := !role.Of(site.self.Role).Silent
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	err := state.WithMailboxLock(wait, site.dir, site.self.Name, func() error {
		// A read frozen before the mailbox stopped is not marked read after.
		if err := mail.MailboxStopped(site.dir, site.self.Name); err != nil {
			return err
		}
		return mail.MarkWholeRead(site.dir, site.self, site.epoch, record.Read, reports)
	})
	if saveErr := receipt.Save(site.dir, site.self.Name, *record); err == nil {
		err = saveErr
	}
	if err != nil {
		return failf("the letters were shown, but recording that they were read failed, so they stay unread and show again: %v", err)
	}
	return nil
}

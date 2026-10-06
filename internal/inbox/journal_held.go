package inbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// deliverReport takes one report of a journal to its recipient, and records
// in the journal what came of it before anything else is done: to a running
// run it is published, under its marks; to a run that has ended or was
// replaced it is moot: it can reach nobody.
func (w world) deliverReport(ctx context.Context, name string, save func() error, journal *TurnJournal, report Message) error {
	published, err := w.publishReport(ctx, name, report)
	if err != nil {
		return err
	}
	if published {
		journal.Published = append(journal.Published, report.ID)
	} else {
		journal.Moot = append(journal.Moot, report.ID)
	}
	return save()
}

// recipientLockWait bounds how long a barrier, holding its own mailbox lock,
// waits for a recipient's: two mailboxes publishing to each other at once
// then end with both waits expired, never with both waiting for good.
var recipientLockWait = 2 * time.Second

// publishReport leaves one report in its recipient's mailbox unless it is
// there already, and answers false, writing nothing, when the recipient's run
// has ended: the report was addressed to that run, not to whoever holds the
// name now.
//
// The run is read, the evidence looked for and the report written in one
// section of the recipient's mailbox lock, and the proof of a landing is
// retired only under that lock (docs/v2/stage3-publication.md#the-contract).
// A run read live before the lock could end, and its proof be swept, before
// the evidence is read; the report would then be written a second time. A
// lock that is busy past the wait is a plain failure, not an unknown: the
// journal stays unfinished and the next barrier publishes. A report to the
// sender's own name is published inside the barrier's own lock, which is not
// reentrant. The plan takes no lock: it predicts, and only the real pass, in
// the section, decides.
func (w world) publishReport(ctx context.Context, name string, report Message) (bool, error) {
	put := world.putOnce
	if report.To == name && len(report.InReplyTo) == 0 {
		// A failed main must not wake itself into another failing turn.
		put = world.putLocal
	}
	if report.To == name {
		return w.publishAdmitted(report, put)
	}
	wait, cancel := context.WithTimeout(ctx, recipientLockWait)
	defer cancel()
	published := false
	err := w.lock(wait, report.To, func() error {
		var err error
		published, err = w.publishAdmitted(report, put)
		return err
	})
	if errors.Is(err, state.ErrMailboxBusy) {
		return false, fmt.Errorf("the mailbox of %s was busy, so the report to it was not published yet: %w", report.To, err)
	}
	return published, err
}

// publishAdmitted publishes a report whose recipient's run it reads first,
// inside the section. The read leaves stale records where they are: the
// caller holds a mailbox lock, and the cleanup a lookup does takes the name's.
func (w world) publishAdmitted(report Message, put func(world, Message) error) (bool, error) {
	peer, err := registry.LookupReadOnly(w.dir, report.To)
	if errors.Is(err, registry.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if peer.Epoch() != report.ToEpoch {
		return false, nil
	}
	w.publicationStep(PublicationAdmitted, report)
	return true, w.publishMarked(report, put)
}

// publishMarked puts a report under the marks its recipient's mailbox keeps
// of what was published to its run (once.go): "intent" before the report is
// written, "published" after, and the sweep turns an intent into published
// before it removes a report the mark names. So the mailbox proves a report
// was written for as long as its run lives, which is as long as a journal
// could publish to it: a journal that put the report and could not record it
// does not put it again once the recipient has read it and the sweep has
// removed it.
//
// A report found already written, by an attempt that died before its mark, is
// marked published before the journal goes on: the letter is proof only until
// the sweep removes it. The caller holds the recipient's lock, so no sweep
// runs between the reads and the writes.
func (w world) publishMarked(report Message, put func(world, Message) error) error {
	path, ok := oncePath(w.dir, report.To, report.ToEpoch, report.ID)
	if !ok {
		return put(w, report)
	}
	w.publicationStep(PublicationEvidence, report)
	found, err := w.present(report.To, report.ID)
	if err != nil {
		return err
	}
	mark, err := w.readOnceMark(path)
	if err != nil || mark == oncePublished {
		return err
	}
	if mark == "" {
		if err := w.ensureDir(filepath.Join(state.InboxPath(w.dir, report.To), "once")); err != nil {
			return err
		}
		if err := w.ensureDir(filepath.Dir(path)); err != nil {
			return err
		}
		if !found {
			if err := w.writeFile(path, []byte(onceIntent)); err != nil {
				return err
			}
		}
	}
	if !found {
		if err := put(w, report); err != nil {
			return err
		}
	}
	return w.writeFile(path, []byte(oncePublished))
}

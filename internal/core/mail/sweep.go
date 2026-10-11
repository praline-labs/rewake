package mail

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// SweepFinishedLocked removes what a mailbox kept of mail answered long enough
// ago that nobody is coming back for it. The delivery server's sweep calls it
// inside its mailbox section, once it has read there that the run epoch is the
// live run of the name (docs/v2/stage3-publication.md#the-contract). It takes no
// lock of its own, since the mailbox lock is not reentrant, and proves neither
// the lock nor the run itself: its caller has.
//
// Nothing is removed on a guess: a record whose keeping depends on another
// that could not be read stays, and the next sweep decides it.
func SweepFinishedLocked(dir, name, epoch string) {
	cutoff := time.Now().Add(-keepFinished)
	kept := keptByStopOf(dir, name)
	receipts, receiptsErr := retainedReceipts(dir, name)
	// A task read a day ago and still worked on is still owed, and rewake
	// inbox --owed must be able to show it again.
	owed, owedErr := owedIDs(dir, name, epoch)
	// Unread mail goes by age too: a notice nobody acted on for a day describes
	// a conversation that has moved on, and the mailbox of a name reused for
	// weeks would otherwise keep every one of them.
	finished := []string{state.DonePath(dir, name), state.UnreadPath(dir, name), answerReceiptsPath(dir, name), state.AnsweringPath(dir, name), threadPath(dir, name), retentionPath(dir, name)}
	for _, directory := range append(finished, state.InboxPath(dir, name)) {
		entries, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || kept.keeps(filepath.Join(directory, entry.Name())) {
				continue
			}
			// In the mailbox itself only statuses are old news; a message still
			// waiting there is answered by the TTL, not by this.
			if directory == state.InboxPath(dir, name) && !strings.HasSuffix(entry.Name(), ".status") {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if directory == state.UnreadPath(dir, name) {
				// Queued mail is still being served. Its readable copy records
				// acceptance and must survive a long live answer reservation.
				if _, err := os.Stat(filepath.Join(state.InboxPath(dir, name), entry.Name())); !errors.Is(err, os.ErrNotExist) {
					continue
				}
			}
			if directory == threadPath(dir, name) && keepThreadRecord(dir, name, entry.Name()) {
				continue
			}
			// A read whose last step did not finish left the text in unread/.
			if (directory == state.DonePath(dir, name) || directory == state.UnreadPath(dir, name)) && (owedErr != nil || owed[strings.TrimSuffix(entry.Name(), ".json")]) {
				continue
			}
			if directory == answerReceiptsPath(dir, name) && (receiptsErr != nil || receipts[entry.Name()]) {
				continue
			}
			if directory == state.UnreadPath(dir, name) && claimed(dir, name, strings.TrimSuffix(entry.Name(), ".json")) {
				// Being read in parts: what the agent saw of it is uncertain.
				continue
			}
			if (directory == state.DonePath(dir, name) || directory == state.UnreadPath(dir, name)) &&
				!settleOnce(dir, name, epoch, strings.TrimSuffix(entry.Name(), ".json")) {
				// The letter is the only proof an intent was carried out; a
				// retry finding neither would write it again.
				continue
			}
			if directory == retentionPath(dir, name) {
				if _, err := os.Stat(filepath.Join(state.InboxPath(dir, name), entry.Name()+".json")); !errors.Is(err, os.ErrNotExist) {
					continue
				}
			}
			_ = state.Remove(filepath.Join(directory, entry.Name()))
		}
	}
	sweepClaims(dir, name, cutoff)
	sweepOnce(dir, name, epoch)
	sweepTurnRecords(dir, name, epoch, cutoff)
	sweepMarks(dir, name, epoch)
	// The receipts sweep their own records under their own locks: one an
	// open occurrence names keeps all of them.
	if !kept.keeps(filepath.Join(state.InboxPath(dir, name), "receipts")) {
		receipt.Sweep(dir, name, epoch, cutoff, func(id string) bool { return stillUnread(dir, name, id) })
	}
}

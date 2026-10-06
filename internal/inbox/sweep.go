package inbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// sweepFinished removes the messages and statuses that have been answered long
// enough ago that nobody is coming back for them.
//
// It retires the proof of what was published to the mailbox — a letter, the
// marks of a run — so it removes only under the mailbox lock, never in the
// unlocked fallback the server's other work takes, and only while its own run
// is the live run of the name, read inside that section. A publisher reads the
// run, the proof and writes in one section of the same lock, so a sweep never
// runs between them; a server of a run that has ended would otherwise remove
// the live run's marks and letters (docs/v2/stage3-publication.md#the-contract).
func (s *Server) sweepFinished() {
	ctx := s.lockContext
	if ctx == nil {
		ctx = context.Background()
	}
	_ = state.WithMailboxLock(ctx, s.Dir, s.Name, func() error {
		if !s.liveRun() {
			return nil
		}
		s.sweepFinishedLocked()
		return nil
	})
}

// liveRun says whether this server's run is the live run of its name, read
// without the name lock's cleanup, which a holder of a mailbox lock must not
// enter.
func (s *Server) liveRun() bool {
	current, err := registry.LookupReadOnly(s.Dir, s.Name)
	return err == nil && s.Epoch != "" && current.Epoch() == s.Epoch
}

// Nothing is removed on a guess: a record whose keeping depends on another
// that could not be read stays, and the next sweep decides it.
func (s *Server) sweepFinishedLocked() {
	cutoff := time.Now().Add(-keepFinished)
	receipts, receiptsErr := retainedReceipts(s.Dir, s.Name)
	// A task read a day ago and still worked on is still owed, and rewake
	// inbox --owed must be able to show it again.
	owed, owedErr := owedIDs(s.Dir, s.Name, s.Epoch)
	// Unread mail goes by age too: a notice nobody acted on for a day describes
	// a conversation that has moved on, and the mailbox of a name reused for
	// weeks would otherwise keep every one of them.
	finished := []string{state.DonePath(s.Dir, s.Name), state.UnreadPath(s.Dir, s.Name), answerReceiptsPath(s.Dir, s.Name), state.AnsweringPath(s.Dir, s.Name), threadPath(s.Dir, s.Name), retentionPath(s.Dir, s.Name)}
	for _, directory := range append(finished, state.InboxPath(s.Dir, s.Name)) {
		entries, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			// In the mailbox itself only statuses are old news; a message still
			// waiting there is answered by the TTL, not by this.
			if directory == state.InboxPath(s.Dir, s.Name) && !strings.HasSuffix(entry.Name(), ".status") {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if directory == state.UnreadPath(s.Dir, s.Name) {
				// Queued mail is still being served. Its readable copy records
				// acceptance and must survive a long live answer reservation.
				if _, err := os.Stat(filepath.Join(state.InboxPath(s.Dir, s.Name), entry.Name())); !errors.Is(err, os.ErrNotExist) {
					continue
				}
			}
			if directory == threadPath(s.Dir, s.Name) && keepThreadRecord(s.Dir, s.Name, entry.Name()) {
				continue
			}
			// A read whose last step did not finish left the text in unread/.
			if (directory == state.DonePath(s.Dir, s.Name) || directory == state.UnreadPath(s.Dir, s.Name)) && (owedErr != nil || owed[strings.TrimSuffix(entry.Name(), ".json")]) {
				continue
			}
			if directory == answerReceiptsPath(s.Dir, s.Name) && (receiptsErr != nil || receipts[entry.Name()]) {
				continue
			}
			if directory == state.UnreadPath(s.Dir, s.Name) && claimed(s.Dir, s.Name, strings.TrimSuffix(entry.Name(), ".json")) {
				// Being read in parts: what the agent saw of it is uncertain.
				continue
			}
			if (directory == state.DonePath(s.Dir, s.Name) || directory == state.UnreadPath(s.Dir, s.Name)) &&
				!settleOnce(s.Dir, s.Name, s.Epoch, strings.TrimSuffix(entry.Name(), ".json")) {
				// The letter is the only proof an intent was carried out; a
				// retry finding neither would write it again.
				continue
			}
			if directory == retentionPath(s.Dir, s.Name) {
				if _, err := os.Stat(filepath.Join(state.InboxPath(s.Dir, s.Name), entry.Name()+".json")); !errors.Is(err, os.ErrNotExist) {
					continue
				}
			}
			_ = state.Remove(filepath.Join(directory, entry.Name()))
		}
	}
	sweepClaims(s.Dir, s.Name, cutoff)
	sweepOnce(s.Dir, s.Name, s.Epoch)
	sweepTurnRecords(s.Dir, s.Name, s.Epoch, cutoff)
	sweepMarks(s.Dir, s.Name, s.Epoch)
	receipt.Sweep(s.Dir, s.Name, s.Epoch, cutoff, func(id string) bool { return stillUnread(s.Dir, s.Name, id) })
}

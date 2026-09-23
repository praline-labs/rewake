package inbox

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
) // sweepFinished removes the messages and statuses that have been answered long

// enough ago that nobody is coming back for them.
func (s *Server) sweepFinished() {
	_ = s.lock(func() error {
		s.sweepFinishedLocked()
		return nil
	})
}

func (s *Server) sweepFinishedLocked() {
	cutoff := time.Now().Add(-keepFinished)
	receipts := retainedReceipts(s.Dir, s.Name)
	// A task read a day ago and still worked on is still owed, and rewake
	// inbox --owed must be able to show it again.
	owed := owedIDs(s.Dir, s.Name, s.Epoch)
	// Unread mail goes by age too: a notice nobody acted on for a day describes
	// a conversation that has moved on, and the mailbox of a name reused for
	// weeks would otherwise keep every one of them.
	finished := []string{state.DonePath(s.Dir, s.Name), state.UnreadPath(s.Dir, s.Name), answerReceiptsPath(s.Dir, s.Name), state.AnsweringPath(s.Dir, s.Name), threadPath(s.Dir, s.Name), retentionPath(s.Dir, s.Name), filepath.Join(state.InboxPath(s.Dir, s.Name), "turns")}
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
				if _, err := os.Stat(filepath.Join(state.InboxPath(s.Dir, s.Name), entry.Name())); err == nil {
					continue
				}
			}
			if directory == threadPath(s.Dir, s.Name) && keepThreadRecord(s.Dir, s.Name, s.Epoch, entry.Name()) {
				continue
			}
			// A read whose last step did not finish left the text in unread/.
			if (directory == state.DonePath(s.Dir, s.Name) || directory == state.UnreadPath(s.Dir, s.Name)) && owed[strings.TrimSuffix(entry.Name(), ".json")] {
				continue
			}
			if directory == answerReceiptsPath(s.Dir, s.Name) && receipts[entry.Name()] {
				continue
			}
			if directory == retentionPath(s.Dir, s.Name) {
				if _, err := os.Stat(filepath.Join(state.InboxPath(s.Dir, s.Name), entry.Name()+".json")); err == nil {
					continue
				}
			}
			_ = os.Remove(filepath.Join(directory, entry.Name()))
		}
	}
}

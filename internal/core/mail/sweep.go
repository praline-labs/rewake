package mail

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// SweepFinishedLocked removes what a mailbox kept of mail answered long enough
// ago that nobody is coming back for it. The delivery server's sweep calls it
// inside its mailbox section, once it has read there that the run epoch is the
// live run of the name (docs/rules/effects.md#publication-once). It takes no
// lock of its own, since the mailbox lock is not reentrant, and proves neither
// the lock nor the run itself: its caller has. Its callers are held by
// internal/layout_callers_test.go.
//
// Nothing is removed on a guess: a record whose keeping depends on another
// that could not be read stays, and the next sweep decides it.
func SweepFinishedLocked(dir, name, epoch string) {
	cutoff := time.Now().Add(-keepFinished)
	receipts, receiptsErr := retainedReceipts(dir, name)
	// A task read a day ago and still worked on is still owed, and rewake
	// inbox --owed must be able to show it again.
	owed, owedErr := owedIDs(dir, name, epoch)
	// Unread mail goes by age too: a notice nobody acted on for a day describes
	// a conversation that has moved on, and the mailbox of a name reused for
	// weeks would otherwise keep every one of them.
	finished := []string{state.DonePath(dir, name), state.UnreadPath(dir, name), answerReceiptsPath(dir, name), state.AnsweringPath(dir, name), ThreadPath(dir, name), RetentionPath(dir, name)}
	for _, directory := range append(finished, state.InboxPath(dir, name)) {
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
			if directory == ThreadPath(dir, name) && keepThreadRecord(dir, name, entry.Name()) {
				continue
			}
			// A read whose last step did not finish left the text in unread/.
			if (directory == state.DonePath(dir, name) || directory == state.UnreadPath(dir, name)) && (owedErr != nil || owed[strings.TrimSuffix(entry.Name(), ".json")]) {
				continue
			}
			if directory == answerReceiptsPath(dir, name) && (receiptsErr != nil || receipts[entry.Name()]) {
				continue
			}
			if (directory == state.DonePath(dir, name) || directory == state.UnreadPath(dir, name)) &&
				reportStillAwaited(dir, name, filepath.Join(directory, entry.Name())) {
				// Its writer's wait still names what it answers: the end that
				// wrote it did not clear the wait, and a retry finding no letter
				// would write it again.
				continue
			}
			if (directory == state.DonePath(dir, name) || directory == state.UnreadPath(dir, name)) &&
				!settleOnce(dir, name, epoch, strings.TrimSuffix(entry.Name(), ".json")) {
				// The letter is the only proof an intent was carried out; a
				// retry finding neither would write it again.
				continue
			}
			if directory == RetentionPath(dir, name) {
				if _, err := os.Stat(filepath.Join(state.InboxPath(dir, name), entry.Name()+".json")); !errors.Is(err, os.ErrNotExist) {
					continue
				}
			}
			_ = state.Remove(filepath.Join(directory, entry.Name()))
		}
	}
	sweepOnce(dir, name, epoch)
	sweepTurnRecords(dir, name, epoch, cutoff)
	sweepMarks(dir, name, epoch)
	// The receipts sweep their own records under their own locks.
	receipt.Sweep(dir, name, epoch, cutoff)
}

// reportStillAwaited says whether the letter at path is a report whose writer's
// run still records the wait it answers. One whose wait cannot be read may be
// such a report, and is kept; a letter that cannot be read is no report.
func reportStillAwaited(dir, name, path string) bool {
	raw, err := state.ReadFile(path)
	if err != nil {
		return false
	}
	var letter Message
	if json.Unmarshal(raw, &letter) != nil || !IsReport(letter) || len(letter.InReplyTo) == 0 || !state.ValidName(letter.From) {
		return false
	}
	waits, ok := awaitingPath(dir, letter.From, letter.FromEpoch)
	if !ok {
		return false
	}
	raw, err = state.ReadFile(filepath.Join(waits, name))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	waiter, err := parseWaiter(name, string(raw))
	if err != nil {
		return true
	}
	return waiter.Epoch == letter.ToEpoch && slices.ContainsFunc(letter.InReplyTo, func(id string) bool { return slices.Contains(waiter.Messages, id) })
}

// keepFinished is how long a delivered or refused message and its status are
// kept. Long enough for a sender that came back late to read the answer,
// short enough that a machine running for weeks does not collect a mailbox
// full of last month's conversations.
const keepFinished = 24 * time.Hour

// AnswerLifetime records a report's answer reservation: written when one is
// first seen, stamped when it is released, and from then the report's delivery
// window is counted once more. The delivery server keeps it; the sweep removes
// it once its report waits no more.
type AnswerLifetime struct {
	ReleasedAt time.Time `json:"releasedAt"`
}

// RetentionPath is the directory of a mailbox's AnswerLifetime records.
func RetentionPath(dir, name string) string {
	return filepath.Join(state.InboxPath(dir, name), "retention")
}

// retainedReceipts names the questions whose answer receipts a kept report
// still needs. An error says a report could not be read, and may need any.
func retainedReceipts(dir, name string) (map[string]bool, error) {
	keep := map[string]bool{}
	for _, directory := range []string{state.InboxPath(dir, name), state.UnreadPath(dir, name), state.DonePath(dir, name)} {
		messages, err := ListIn(directory)
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			for _, id := range message.InReplyTo {
				keep[id] = true
			}
		}
	}
	return keep, nil
}

package inbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A harness is told that mail is waiting, not handed the text, so the message
// has to be readable before the notice goes out: an agent that runs rewake inbox
// the moment it is told must find it. It is linked into unread/ first and leaves
// the waiting set once its notice is out. Reading moves it on to done/, records
// the read, and notes who waits for the end of the reader's turn.

// linkUnread makes a waiting message readable. The link is the same file, so
// nothing can be read that was not written, and linking twice is harmless.
func linkUnread(dir, to, id string) error {
	unread := state.UnreadPath(dir, to)
	if err := state.EnsureSubdir(unread); err != nil {
		return err
	}
	err := os.Link(filepath.Join(state.InboxPath(dir, to), id+".json"), filepath.Join(unread, id+".json"))
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	_ = state.SyncDir(unread)
	return nil
}

// dropUnread takes back a message that will not be announced after all.
func dropUnread(dir, to, id string) {
	_ = os.Remove(filepath.Join(state.UnreadPath(dir, to), id+".json"))
}

// noticeContext chooses the newest available letter; active reservations stay
// invisible to ordinary notifications just as they do to inbox reads.
func noticeContext(dir, name, epoch string, message Message) Message {
	messages, err := AvailableUnread(dir, name, epoch)
	if err != nil {
		return message
	}
	message.Unread = len(messages)
	for _, candidate := range messages {
		if message.Latest == nil || candidate.CreatedAt.After(message.Latest.CreatedAt) || candidate.CreatedAt.Equal(message.Latest.CreatedAt) && candidate.ID > message.Latest.ID {
			latest := candidate
			message.Latest = &latest
		}
	}
	return message
}

// PeekUnread returns the unread messages of one run of a session, oldest first,
// without marking anything. The caller shows them, then marks each one read: a
// message marked before its text reached anybody would be lost.
func PeekUnread(dir, name, epoch string) ([]Message, error) {
	messages, err := listIn(state.UnreadPath(dir, name))
	if err != nil {
		return nil, err
	}
	mine := make([]Message, 0, len(messages))
	for _, message := range messages {
		if epoch == "" || message.ToEpoch == epoch {
			mine = append(mine, asWithdrawn(dir, name, message))
		}
	}
	return mine, nil
}

// asWithdrawn shows a message its status calls withdrawn as its tombstone. A
// withdrawal writes the status before the tombstone, and one that failed in
// between leaves the original readable by hard link: its sender was told it
// failed, but the text must not reach the reader as work all the same.
func asWithdrawn(dir, name string, message Message) Message {
	if message.Withdrawn != nil {
		return message
	}
	if status, ok := ReadStatus(dir, name, message.ID); ok && status.Withdrawn {
		return tombstoneOf(message, "")
	}
	return message
}

// MarkRead records that this run of the session has read a message. The caller
// holds the mailbox lock and has already shown the text.
//
// Everything reading implies is written before the message leaves unread/, and
// leaving it is the last step. A failure anywhere keeps the message unread, so
// the next read shows it again and records it again; the other order lost the
// report its sender was owed.
//
// A reader that owes nobody — the main session — records no waits at all.
func MarkRead(dir, name, epoch string, message Message, reports bool) error {
	// A read status is written only after the waiter, so finding one means
	// this is a retry of a read whose last step failed: the waiter was recorded
	// then, and may have been reported to since. Recording it again owed a
	// second report for one message.
	status, known := ReadStatus(dir, name, message.ID)
	retry := known && status.State == Read
	// Withdrawn is final like read, whatever copy the caller holds: a
	// withdrawal that failed after its status leaves the original in unread/,
	// and reading it must neither owe a report nor write read over the status
	// its sender, and a second withdraw, go on reading.
	withdrawn := message.Withdrawn != nil || known && status.Withdrawn
	// A note or a report owes nothing, and a sender without a run of its own —
	// a shell, or mail from before runs were recorded — has nowhere a report
	// could go.
	if reports && !retry && !withdrawn && Owed(message) {
		if err := markScopedAwaiting(dir, name, epoch, message); err != nil {
			return err
		}
	}
	if !withdrawn {
		if err := writeStatus(dir, name, message.ID, Result{State: Read}); err != nil {
			return err
		}
	}
	if withdrawn && !tombstoneIn(state.UnreadPath(dir, name), message.ID) {
		// What is kept is what was read: the tombstone, not the original.
		return archiveTombstone(dir, name, tombstoneOf(message, ""))
	}
	err := move(message.ID, state.UnreadPath(dir, name), state.DonePath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// archiveTombstone keeps a tombstone in done/ in place of the original a
// half-done withdrawal left in unread/.
func archiveTombstone(dir, name string, stone Message) error {
	encoded, err := json.MarshalIndent(stone, "", "  ")
	if err != nil {
		return err
	}
	if err := state.EnsureSubdir(state.DonePath(dir, name)); err != nil {
		return err
	}
	if err := state.WriteAtomic(filepath.Join(state.DonePath(dir, name), stone.ID+".json"), append(encoded, '\n')); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(state.UnreadPath(dir, name), stone.ID+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = state.SyncDir(state.UnreadPath(dir, name))
	return nil
}

// move renames a message file between two directories of one mailbox.
func move(id, from, into string) error {
	if err := state.EnsureSubdir(into); err != nil {
		return err
	}
	source := filepath.Join(from, id+".json")
	target := filepath.Join(into, id+".json")
	if err := os.Rename(source, target); err != nil {
		return err
	}
	// The age that matters is the age of the move: an old message refused at
	// startup would otherwise be swept away in the same breath.
	now := time.Now()
	_ = os.Chtimes(target, now, now)
	// Both ends are flushed: after a crash the message must be in one of the two
	// places, never in both and never in neither.
	_ = state.SyncDir(from)
	_ = state.SyncDir(into)
	return nil
}

// listIn returns the messages in one directory, oldest first.
func listIn(directory string) ([]Message, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	messages := make([]Message, 0, len(names))
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			continue
		}
		var message Message
		if err := json.Unmarshal(raw, &message); err != nil {
			// A file that is not a message is not ours to interpret; leave it
			// where it is rather than deleting somebody else's data.
			continue
		}
		messages = append(messages, message)
	}
	return messages, nil
}

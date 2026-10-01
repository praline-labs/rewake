package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
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
// invisible to ordinary notifications just as they do to inbox reads. What it
// adds is the notice's count and preview, never whether the notice goes, so a
// mailbox it cannot read leaves the notice about this one message.
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
// message marked before its text reached anybody would be lost. An error says
// a letter, or its status, could not be read, and so what is unread is not
// known.
func PeekUnread(dir, name, epoch string) ([]Message, error) {
	messages, err := listIn(state.UnreadPath(dir, name))
	if err != nil {
		return nil, err
	}
	mine := make([]Message, 0, len(messages))
	for _, message := range messages {
		if epoch == "" || message.ToEpoch == epoch {
			shown, err := asWithdrawn(dir, name, message)
			if err != nil {
				return nil, err
			}
			mine = append(mine, shown)
		}
	}
	return mine, nil
}

// UnreadCopy is one letter of a run as unread/ holds it now, a withdrawn one as
// its tombstone. It looks the letter up by itself, not through a listing that
// passes over a file it cannot read: false says the letter is not unread, and
// an error that it, or its status, could not be read. The caller holds the
// mailbox lock.
func UnreadCopy(dir, name, epoch, id string) (Message, bool, error) {
	if !safeID(id) {
		return Message{}, false, nil
	}
	raw, err := os.ReadFile(filepath.Join(state.UnreadPath(dir, name), id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, err
	}
	var message Message
	if err := json.Unmarshal(raw, &message); err != nil {
		return Message{}, false, fmt.Errorf("the letter %s is not readable: %w", id, err)
	}
	if epoch != "" && message.ToEpoch != epoch {
		return Message{}, false, nil
	}
	shown, err := asWithdrawn(dir, name, message)
	if err != nil {
		return Message{}, false, err
	}
	return shown, true, nil
}

// asWithdrawn shows a message its status calls withdrawn as its tombstone. A
// withdrawal writes the status before the tombstone, and one that failed in
// between leaves the original readable by hard link: its sender was told it
// failed, but the text must not reach the reader as work all the same. A
// status that cannot be read may say withdrawn, so it is an error.
func asWithdrawn(dir, name string, message Message) (Message, error) {
	if message.Withdrawn != nil {
		return message, nil
	}
	status, known, err := ReadStatus(dir, name, message.ID)
	if err != nil {
		return Message{}, err
	}
	if known && status.Withdrawn {
		return tombstoneOf(message, ""), nil
	}
	return message, nil
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
	// second report for one message. A status that cannot be read may be
	// that read one, so it stops the read here, still unread.
	status, known, err := ReadStatus(dir, name, message.ID)
	if err != nil {
		return err
	}
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
		if err := markScopedAwaiting(dir, name, epoch, message, 0); err != nil {
			return err
		}
	}
	if !withdrawn {
		if err := writeStatus(dir, name, message.ID, Result{State: Read}); err != nil {
			return err
		}
	}
	if withdrawn {
		stone, err := tombstoneIn(state.UnreadPath(dir, name), message.ID)
		if err != nil {
			return err
		}
		if !stone {
			// What is kept is what was read: the tombstone, not the original.
			return archiveTombstone(dir, name, tombstoneOf(message, ""))
		}
	}
	err = move(message.ID, state.UnreadPath(dir, name), state.DonePath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err == nil {
		// Read to the end, by whichever channel: no longer in progress.
		dropClaim(dir, name, message.ID)
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

// listIn returns the messages in one directory, oldest first. A file that
// left the directory after it was listed moved on, and is not listed; one that
// is there and cannot be read, or does not parse, may be any letter at all, so
// the listing is an error naming it rather than a list without it.
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
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var message Message
		if err := json.Unmarshal(raw, &message); err != nil {
			// Nothing is deleted or passed over: the file stays where it is,
			// and whoever lists the directory hears it cannot tell.
			return nil, fmt.Errorf("%s is not a readable letter: %w", filepath.Join(directory, name), err)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

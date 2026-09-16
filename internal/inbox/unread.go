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

// countUnread is how many messages of this run are waiting to be read.
func countUnread(dir, to, epoch string) int {
	messages, err := PeekUnread(dir, to, epoch)
	if err != nil {
		return 0
	}
	return len(messages)
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
			mine = append(mine, message)
		}
	}
	return mine, nil
}

// MarkRead records that this run of the session has read a message. The caller
// holds the mailbox lock and has already shown the text.
//
// Everything reading implies is written before the message leaves unread/, and
// leaving it is the last step. A failure anywhere keeps the message unread, so
// the next read shows it again and records it again; the other order lost the
// report its sender was owed.
func MarkRead(dir, name, epoch string, message Message) error {
	// A finished notice is an answer already, and waiting on it would have two
	// sessions report their turns to each other forever. A sender without a run
	// of its own — a shell, or mail from before runs were recorded — has nowhere
	// a report could go.
	if KindOf(message) != Finished && message.FromEpoch != "" {
		if err := markAwaiting(dir, name, epoch, message.From, message.FromEpoch); err != nil {
			return err
		}
	}
	if err := writeStatus(dir, name, message.ID, Result{State: Read}); err != nil {
		return err
	}
	err := move(message.ID, state.UnreadPath(dir, name), state.DonePath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Waiter is a session run waiting for the end of this turn.
type Waiter struct {
	Name  string
	Epoch string
}

// awaitingPath holds the waiters of one run. Keyed by run, because a name can
// be taken over, and what the previous run read is not the next one's to report.
func awaitingPath(dir, name, epoch string) (string, bool) {
	if epoch == "" || strings.ContainsAny(epoch, `/\`) || strings.HasPrefix(epoch, ".") {
		return "", false
	}
	return filepath.Join(state.AwaitingPath(dir, name), epoch), true
}

// markAwaiting records that a run of another session waits for this turn.
func markAwaiting(dir, name, epoch, from, fromEpoch string) error {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok || !state.ValidName(from) {
		return nil
	}
	if err := state.EnsureSubdir(state.AwaitingPath(dir, name)); err != nil {
		return err
	}
	if err := state.EnsureSubdir(path); err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(path, from), []byte(fromEpoch))
}

// Waiters lists who waits for the end of this run's turn. Nothing is forgotten
// here: a waiter is cleared only once the report to it is written.
func Waiters(dir, name, epoch string) []Waiter {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	waiters := make([]Waiter, 0, len(entries))
	for _, entry := range entries {
		peer := entry.Name()
		if entry.IsDir() || !state.ValidName(peer) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(path, peer))
		if err != nil {
			continue
		}
		waiters = append(waiters, Waiter{Name: peer, Epoch: strings.TrimSpace(string(raw))})
	}
	sort.Slice(waiters, func(i, j int) bool { return waiters[i].Name < waiters[j].Name })
	return waiters
}

// ClearAwaiting forgets a waiter once it has been reported to, or once its run
// has ended. Only that run is forgotten: a newer run of the same name that
// wrote in the meantime is still owed its report. The caller holds the mailbox
// lock, which is what keeps the check and the removal together.
func ClearAwaiting(dir, name, epoch string, peer Waiter) {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok || !state.ValidName(peer.Name) {
		return
	}
	file := filepath.Join(path, peer.Name)
	raw, err := os.ReadFile(file)
	if err != nil || strings.TrimSpace(string(raw)) != peer.Epoch {
		return
	}
	_ = os.Remove(file)
}

// sweepAwaiting forgets what earlier runs of this name were waited on for.
func sweepAwaiting(dir, name, epoch string) {
	entries, err := os.ReadDir(state.AwaitingPath(dir, name))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() != epoch {
			_ = os.RemoveAll(filepath.Join(state.AwaitingPath(dir, name), entry.Name()))
		}
	}
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

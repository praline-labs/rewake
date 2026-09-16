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

// A delivered message is not archived. The harness was only told that it is
// waiting, so it moves to unread/ and stays there until the agent fetches it.
// Reading moves it on to done/ and records the read, which is also what tells a
// session who is waiting for the end of its turn.

// markUnread moves a message whose notice went out into unread/.
func markUnread(dir, to, id string) error {
	return move(id, state.InboxPath(dir, to), state.UnreadPath(dir, to))
}

// countUnread is how many messages of this run are waiting to be read.
func countUnread(dir, to, epoch string) int {
	messages, err := listIn(state.UnreadPath(dir, to))
	if err != nil {
		return 0
	}
	count := 0
	for _, message := range messages {
		if epoch == "" || message.ToEpoch == epoch {
			count++
		}
	}
	return count
}

// TakeUnread returns the unread messages of one run of a session, oldest first,
// and marks them read. A message is returned only if this call moved it: two
// readers at once each get their own share, never the same message twice.
func TakeUnread(dir, name, epoch string) ([]Message, error) {
	messages, err := listIn(state.UnreadPath(dir, name))
	if err != nil {
		return nil, err
	}
	taken := make([]Message, 0, len(messages))
	for _, message := range messages {
		if epoch != "" && message.ToEpoch != epoch {
			continue
		}
		if err := move(message.ID, state.UnreadPath(dir, name), state.DonePath(dir, name)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return taken, err
		}
		_ = writeStatus(dir, name, message.ID, Result{State: Read})
		if message.From != "" && KindOf(message) != Finished && !message.Reply {
			// A finished notice or a reply is an answer already; waiting on it
			// would have two sessions wake each other for nothing.
			_ = markAwaiting(dir, name, message.From)
		}
		taken = append(taken, message)
	}
	return taken, nil
}

// markAwaiting records that a session is waiting for the end of this turn.
func markAwaiting(dir, name, from string) error {
	if !state.ValidName(from) {
		return nil
	}
	awaiting := state.AwaitingPath(dir, name)
	if err := state.EnsureSubdir(awaiting); err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(awaiting, from), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"))
}

// ClearAwaiting clears a waiting session once this one has written to it
// directly, and reports whether it was waiting: the message is then an answer,
// and it says more than a notice that the turn ended would.
func ClearAwaiting(dir, name, to string) bool {
	if !state.ValidName(to) {
		return false
	}
	return os.Remove(filepath.Join(state.AwaitingPath(dir, name), to)) == nil
}

// TakeAwaiting returns the sessions waiting for the end of this turn and forgets
// them, so each is told once.
func TakeAwaiting(dir, name string) []string {
	awaiting := state.AwaitingPath(dir, name)
	entries, err := os.ReadDir(awaiting)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		peer := entry.Name()
		if entry.IsDir() || !state.ValidName(peer) {
			continue
		}
		if err := os.Remove(filepath.Join(awaiting, peer)); err != nil {
			continue
		}
		names = append(names, peer)
	}
	sort.Strings(names)
	return names
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

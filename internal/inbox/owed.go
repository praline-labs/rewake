package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/praline-labs/rewake/internal/state"
)

// OwedMessage is a message this run has read and still owes a report for.
// Kept is false when the text is no longer on disk, and then only the id and
// the sender are known.
type OwedMessage struct {
	Message
	Kept bool
}

// OwedMessages lists what this run has read and not yet reported on: the
// messages its wait records still name, oldest read first. A wait record is
// cleared only when a report settles it, and kept through an interim turn end
// or a stop, so this is exactly what the end of a turn would report on.
//
// It only reads. Nothing is marked, recorded or announced, and no lock is
// taken: every record read here is written whole or not at all. An error says
// a wait record or a copy could not be read: a list without it would show a
// task as not owed, or its text as no longer kept.
func OwedMessages(dir, name, epoch string) ([]OwedMessage, error) {
	type reading struct {
		owed     OwedMessage
		sequence uint64
		since    int64
		order    int
	}
	var readings []reading
	waiters, err := ReadWaiters(dir, name, epoch)
	if err != nil {
		return nil, err
	}
	for _, waiter := range waiters {
		for index, id := range waiter.Messages {
			if !safeID(id) {
				continue
			}
			message, kept, err := readCopy(dir, name, id)
			if err != nil {
				return nil, err
			}
			if !kept {
				message = Message{ID: id, From: waiter.Name, FromEpoch: waiter.Epoch, To: name}
			}
			var sequence uint64
			if index < len(waiter.ReadSequences) {
				sequence = waiter.ReadSequences[index]
			}
			readings = append(readings, reading{
				owed:     OwedMessage{Message: message, Kept: kept},
				sequence: sequence,
				since:    waiter.Since,
				order:    len(readings),
			})
		}
	}
	// The read sequence orders reads across senders; a record from before it
	// was kept falls back on when its wait began.
	sort.SliceStable(readings, func(i, j int) bool {
		a, b := readings[i], readings[j]
		if a.sequence != 0 && b.sequence != 0 {
			return a.sequence < b.sequence
		}
		if a.since != b.since {
			return a.since < b.since
		}
		return a.order < b.order
	})
	owed := make([]OwedMessage, 0, len(readings))
	for _, r := range readings {
		owed = append(owed, r.owed)
	}
	return owed, nil
}

// owedIDs is the set of message ids this run still owes a report for. An
// error says a wait record could not be read, and the set may lack an id.
func owedIDs(dir, name, epoch string) (map[string]bool, error) {
	waiters, err := ReadWaiters(dir, name, epoch)
	ids := map[string]bool{}
	for _, waiter := range waiters {
		for _, id := range waiter.Messages {
			ids[id] = true
		}
	}
	return ids, err
}

// readCopy finds a message wherever its mailbox keeps it. A read message is
// normally in done/; a read whose last step failed left it in unread/. An
// error says a copy is there and could not be read.
func readCopy(dir, name, id string) (Message, bool, error) {
	for _, directory := range []string{state.DonePath(dir, name), state.UnreadPath(dir, name), state.InboxPath(dir, name)} {
		raw, err := state.ReadFile(filepath.Join(directory, id+".json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Message{}, false, err
		}
		var message Message
		if err := json.Unmarshal(raw, &message); err != nil {
			return Message{}, false, fmt.Errorf("the copy of %s in %s is not readable: %w", id, directory, err)
		}
		if message.ID == id {
			return message, true, nil
		}
	}
	return Message{}, false, nil
}

// safeID keeps an id read from a record inside the mailbox it names.
func safeID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\`) && !strings.HasPrefix(id, ".")
}

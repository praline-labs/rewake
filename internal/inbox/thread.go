package inbox

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/state"
)

// ErrThreadUnavailable refuses delivery without making a task readable in an unknown conversation.
var ErrThreadUnavailable = errors.New("delivery thread is unavailable")

// ErrNotYet says the conversation is there but cannot take a message at the
// moment — it is being compacted, say. The message stays pending and is tried
// again, rather than failed as for a conversation that is gone.
var ErrNotYet = errors.New("the conversation cannot take a message yet")

// ThreadChangedWarning lets the caller decide whether an old task needs resending.
const ThreadChangedWarning = "Rewake: the reader's thread changed after delivery; this may not answer it, resend the message"

func threadPath(dir, name string) string { return filepath.Join(state.InboxPath(dir, name), "threads") }

// recordDeliveryThread runs under the mailbox lock before the message becomes
// readable. Recording only after delivery would lose a fast read and report.
func recordDeliveryThread(dir, name, id, thread string) error {
	if err := state.EnsureSubdir(threadPath(dir, name)); err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(threadPath(dir, name), id), []byte(thread))
}

// deliveryThread is the conversation a message was delivered into, "" when
// none was pinned. An error says a record is there and could not be read.
func deliveryThread(dir, name, id string) (string, error) {
	if !state.ValidName(id) {
		return "", nil
	}
	raw, err := state.ReadFile(filepath.Join(threadPath(dir, name), id))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ReportThreadChanged is advisory: without both identities there is no known
// mismatch. The report is still sent and never causes an automatic resend. A
// record that cannot be read gives no identity either: the warning's advice is
// to send the task again, which on a guess could have the work done twice.
func ReportThreadChanged(dir, name string, ids []string, current string) bool {
	if current == "" {
		return false
	}
	for _, id := range ids {
		if before, err := deliveryThread(dir, name, id); err == nil && before != "" && before != current {
			return true
		}
	}
	return false
}

// Unread messages and unsettled waits still need their delivery context, even
// when a long turn outlives the normal retention window. What cannot be looked
// up keeps the record: the sweep is the one effect here, and it is final.
func keepThreadRecord(dir, name, id string) bool {
	stages, err := lookUp(id, state.InboxPath(dir, name), state.UnreadPath(dir, name))
	if err != nil || stages[0] || stages[1] {
		return true
	}
	// Any run's: an earlier run's owed task is what a resumed one takes over
	// by this record (adopt.go).
	owed, err := owedByAnyRun(dir, name, id)
	return err != nil || owed
}

package inbox

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/iiiokojiadbi/rewake/internal/state"
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

func deliveryThread(dir, name, id string) string {
	if !state.ValidName(id) {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(threadPath(dir, name), id))
	if err != nil {
		return ""
	}
	return string(raw)
}

// ReportThreadChanged is advisory: without both identities there is no known
// mismatch. The report is still sent and never causes an automatic resend.
func ReportThreadChanged(dir, name string, ids []string, current string) bool {
	if current == "" {
		return false
	}
	for _, id := range ids {
		if before := deliveryThread(dir, name, id); before != "" && before != current {
			return true
		}
	}
	return false
}

// Unread messages and unsettled waits still need their delivery context, even
// when a long turn outlives the normal retention window.
func keepThreadRecord(dir, name, epoch, id string) bool {
	for _, directory := range []string{state.InboxPath(dir, name), state.UnreadPath(dir, name)} {
		if _, err := os.Stat(filepath.Join(directory, id+".json")); err == nil {
			return true
		}
	}
	for _, waiter := range Waiters(dir, name, epoch) {
		for _, message := range waiter.Messages {
			if message == id {
				return true
			}
		}
	}
	return false
}

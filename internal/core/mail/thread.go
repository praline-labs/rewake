package mail

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// ErrThreadUnavailable refuses delivery without making a task readable in an unknown conversation.
var ErrThreadUnavailable = errors.New("delivery thread is unavailable")

// ErrNotYet says the conversation is there but cannot take a message at the
// moment — it is being compacted, say. The message stays pending and is tried
// again, rather than failed as for a conversation that is gone.
var ErrNotYet = errors.New("the conversation cannot take a message yet")

// ThreadChangedWarning lets the caller decide whether an old task needs resending.
const ThreadChangedWarning = "Rewake: the reader's thread changed after delivery; check that this answers the task, and read rewake inbox --awaited before sending the work again"

// ThreadPath is the directory in which the delivery server records the
// conversation an owed letter goes into, before the letter becomes readable;
// mail reads it back (deliveryThread) and sweeps it.
func ThreadPath(dir, name string) string { return filepath.Join(state.InboxPath(dir, name), "threads") }

// deliveryThread is the conversation a message was delivered into, "" when
// none was pinned. An error says a record is there and could not be read.
func deliveryThread(dir, name, id string) (string, error) {
	if !state.ValidName(id) {
		return "", nil
	}
	raw, err := state.ReadFile(filepath.Join(ThreadPath(dir, name), id))
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
	owed, err := owedByAnyRun(dir, name, id)
	return err != nil || owed
}

// owedByAnyRun says whether a wait record of any run of the name lists the
// message. An error says a wait record could not be read.
func owedByAnyRun(dir, name, id string) (bool, error) {
	runs, err := os.ReadDir(state.AwaitingPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, run := range runs {
		if !run.IsDir() {
			continue
		}
		waiters, err := ReadWaiters(dir, name, run.Name())
		if err != nil {
			return false, err
		}
		for _, waiter := range waiters {
			if slices.Contains(waiter.Messages, id) {
				return true, nil
			}
		}
	}
	return false, nil
}

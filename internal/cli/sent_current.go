package cli

import (
	"fmt"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// currentSent leads a letter rewake edit replaced to what replaces it now,
// through every edit since, and answers any other letter as it stands on disk.
// After an edit the old id is still the name main has for the task — the one
// it was printed, and the one send --to already follows — so withdrawing or
// editing by it acts on the task as it stands, rather than finding a tombstone
// and sending main to write the task out again. The record is read afresh
// rather than trusted: the caller's copy may predate an edit that ran beside
// it, and under the mailbox lock this is the look that decides.
func currentSent(dir string, self registry.Session, epoch string, message inbox.Message) (inbox.Message, error) {
	id := inbox.CurrentTask(dir, message.To, message.ID)
	matches := inbox.SentMatching(dir, self.Name, epoch, id)
	if len(matches) == 1 && matches[0].ID == id {
		return matches[0], nil
	}
	if id == message.ID {
		return message, nil
	}
	kind := inbox.KindOf(message)
	if message.Withdrawn != nil {
		kind = message.Withdrawn.Kind
	}
	return message, failf("your %s %s was replaced by %s, which is no longer kept; see what you sent with: rewake inbox --awaited", kind, message.ID, id)
}

// redirectLine says that a call acted on the replacement of the letter it
// named; doing is the verb, "withdrawing" or "editing".
func redirectLine(named, current, doing string) string {
	return fmt.Sprintf("Rewake: %s was replaced by %s; %s %s.", named, current, doing, current)
}

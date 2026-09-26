package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// sentBySelf finds the message a reference names among what this run sent.
// Every refusal says what to run next: the caller is an agent, and a refusal
// it cannot act on is a dead end.
func sentBySelf(call Call, dir, reference, retry string) (registry.Session, string, inbox.Message, error) {
	self, epoch, err := ownRun(dir)
	if err != nil {
		return registry.Session{}, "", inbox.Message{}, &UsageError{
			Command: call.Command,
			Message: fmt.Sprintf("Only the session run that sent a message may change it, and this shell is not one (%v).", err),
		}
	}
	if len(reference) < inbox.MinReference {
		return self, epoch, inbox.Message{}, &UsageError{
			Command: call.Command,
			Message: fmt.Sprintf("%q is too short to name a message: give at least %d characters of its id, or of the part after the dash. rewake inbox --awaited lists the ids.", reference, inbox.MinReference),
		}
	}
	matches := inbox.SentMatching(dir, self.Name, epoch, reference)
	switch len(matches) {
	case 1:
		return self, epoch, matches[0], nil
	case 0:
		return self, epoch, inbox.Message{}, failf("no message this run sent matches %s; list what you sent with: rewake inbox --awaited", reference)
	}
	lines := []string{fmt.Sprintf("%s matches %d messages this run sent; give more of the id:", reference, len(matches))}
	for _, message := range matches {
		line := fmt.Sprintf("  %s · %s to %s · %s", message.ID, inbox.KindOf(message), message.To, message.CreatedAt.Local().Format("15:04:05"))
		if message.Withdrawn != nil {
			// Its text is the tombstone's, which says nothing about which
			// message it was.
			line = fmt.Sprintf("  %s · %s (withdrawn) to %s · %s", message.ID, message.Withdrawn.Kind, message.To, message.CreatedAt.Local().Format("15:04:05"))
		} else {
			line += " · " + harness.Preview(message.Text)
		}
		lines = append(lines, line)
	}
	// The placeholder stays: any one id filled in would be a guess, and a
	// ready line is copied as it stands.
	lines = append(lines, "then: "+retry)
	return self, epoch, inbox.Message{}, &FailedError{Message: strings.Join(lines, "\n")}
}

// shortRef is the reference a hint offers: the tail of the id, which is
// enough to tell the messages of one run apart.
func shortRef(id string) string { return inbox.ShortID(id) }

// withdrawRefusal explains why a message cannot be taken back, and what to do
// instead. editing says the call was rewake edit, whose retry is its own.
func withdrawRefusal(dir string, message inbox.Message, err error, editing bool) error {
	kind := inbox.KindOf(message)
	if message.Withdrawn != nil {
		kind = message.Withdrawn.Kind
	}
	switch {
	case errors.Is(err, inbox.ErrAlreadyRead):
		next := fmt.Sprintf("rewake send %s \"...\"", message.To)
		if inbox.AsksForWork(message) {
			next = fmt.Sprintf("rewake send %s \"...\" --to %s", message.To, shortRef(message.ID))
		}
		return failf("%s has read your %s %s, and a read message is final; add to it with: %s", message.To, kind, message.ID, next)
	case errors.Is(err, inbox.ErrNotDelivered):
		return failf("your %s %s was not delivered to %s and never will be, so there is nothing to take back; see why with: rewake inbox --awaited", kind, message.ID, message.To)
	case errors.Is(err, inbox.ErrAlreadyWithdrawn):
		return failf("your %s %s to %s was already withdrawn; send a new one with: rewake send %s \"...\"", kind, message.ID, message.To, message.To)
	case errors.Is(err, inbox.ErrNoLongerKept):
		return failf("your %s %s is no longer kept in the mailbox of %s; see what you sent with: rewake inbox --awaited", kind, message.ID, message.To)
	}
	// A withdrawal writes its status first, so the status on disk says how far
	// it got: marked withdrawn, the recipient reads it as withdrawn already,
	// and the next call finishes the rest.
	status, _ := inbox.ReadStatus(dir, message.To, message.ID)
	switch {
	case editing && status.Withdrawn:
		return failf("your %s %s to %s is withdrawn, but its replacement was not sent (%v); send it with: rewake edit %s \"...\"", kind, message.ID, message.To, err, shortRef(message.ID))
	case editing:
		return failf("could not replace your %s %s to %s (%v); nothing changed; try again: rewake edit %s \"...\"", kind, message.ID, message.To, err, shortRef(message.ID))
	case status.Withdrawn:
		return failf("your %s %s to %s is marked withdrawn and %s will read it so, but not every step finished (%v); finish it with: rewake withdraw %s", kind, message.ID, message.To, message.To, err, shortRef(message.ID))
	}
	return failf("could not withdraw your %s %s from %s (%v); nothing changed; try again: rewake withdraw %s", kind, message.ID, message.To, err, shortRef(message.ID))
}

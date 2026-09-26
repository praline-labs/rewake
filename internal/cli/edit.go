package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// handleEdit replaces a message nobody has read: the old one is withdrawn and
// a new one of the same kind goes in its place, under one lock of the
// recipient's mailbox. Not an edit in place: the old notice showed the old
// first line, and read being final needs an id whose text never changes, so
// the replacement is a letter of its own, announced with its own preview.
func handleEdit(ctx *Context, call Call) error {
	if len(call.Positionals) < 2 {
		return &UsageError{Command: call.Command, Message: "edit needs the id of the message, or a unique prefix of it, and the new text."}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	text := call.Positionals[1]
	if text == "-" {
		read, err := io.ReadAll(os.Stdin)
		if err != nil {
			return failf("could not read the new text from stdin: %v", err)
		}
		text = string(read)
	}
	if strings.TrimSpace(text) == "" {
		return &UsageError{Command: call.Command, Message: "The new text is empty; to take the message back without a replacement, run rewake withdraw."}
	}
	self, epoch, named, err := sentBySelf(call, dir, call.Positionals[0], "rewake edit <id> \"...\"")
	if err != nil {
		return err
	}
	old, err := currentSent(dir, self, epoch, named)
	if err != nil {
		return err
	}
	if old.Withdrawn != nil {
		return editedWithdrawn(old)
	}
	target, err := registry.Lookup(dir, old.To)
	if errors.Is(err, registry.ErrNotFound) || err == nil && target.Epoch() != old.ToEpoch {
		return failf("the session run your %s %s was written for has ended, so it will never be read; see what you sent with: rewake inbox --awaited", inbox.KindOf(old), old.ID)
	}
	if err != nil {
		return failf("could not read the record of %s: %v", old.To, err)
	}
	if old.AddendumTo != "" {
		// The replacement adds to the same task, which may have been reported
		// on since: then it would owe a report nobody waits for. Asked again
		// under the lock, below; this look only refuses early.
		root, err := rootOf(dir, self, epoch, old, target)
		if err != nil {
			return err
		}
		if err := addendumRefusal(dir, self, epoch, root, target); err != nil {
			return err
		}
	}
	kind := sendKinds[0]
	for _, candidate := range sendKinds {
		if candidate.kind == inbox.KindOf(old) {
			kind = candidate
		}
	}
	wait, err := waitDuration(call, kind.wait)
	if err != nil {
		return err
	}
	started := time.Now()
	replacement := inbox.Message{
		// A grant goes on only while the sender is still the main that
		// could give it.
		GrantGit: old.GrantGit && self.Role == role.Main.ID,
		ID:       inbox.NewID(), From: self.Name, FromEpoch: epoch, To: old.To, ToEpoch: old.ToEpoch,
		Kind: old.Kind, Text: text, CreatedAt: time.Now(), Replaces: old.ID, AddendumTo: old.AddendumTo,
	}
	if kind.kind == inbox.Question {
		release, err := inbox.ReserveAnswer(dir, self.Name, replacement.ID)
		if err != nil {
			return failf("could not reserve the answer: %v", err)
		}
		defer release()
	}
	// No recall follows: the replacement is announced at once, and its own
	// preview names the message it replaces (harness.Notice), so one line
	// both sets the old work aside and shows the new.
	replace := func() error {
		// Asked again under the lock: an edit running beside this one may have
		// replaced old since the look above, and this one then replaces the
		// letter that stands now rather than find a tombstone.
		current, err := currentSent(dir, self, epoch, named)
		if err != nil {
			return err
		}
		if current.Withdrawn != nil {
			return editedWithdrawn(current)
		}
		old, replacement.Replaces = current, current.ID
		_, err = inbox.Withdraw(dir, old, &replacement)
		return err
	}
	if old.AddendumTo == "" {
		err = underMailboxLock(dir, old.To, replace)
	} else {
		err = underAddendumLock(dir, self, epoch, old, target, replace)
	}
	var refused *FailedError
	if errors.As(err, &refused) {
		return err
	}
	if err != nil {
		return withdrawRefusal(dir, old, err, true)
	}
	after := sent{dir: dir, self: self, epoch: epoch, target: target, deadline: started.Add(wait)}
	if old.ID != named.ID {
		after.named = named.ID
		if !ctx.JSON {
			_ = emit(ctx, redirectLine(named.ID, old.ID, "editing"))
		}
	}
	// Addenda are never rewritten: they name the old id, which now leads to
	// the replacement (inbox.CurrentTask), and the sender is told they came
	// along, since a correction it meant to drop with the old text would
	// otherwise go on being read.
	for _, addendum := range inbox.AddendaOf(dir, replacement) {
		after.addenda = append(after.addenda, addendum.ID)
	}
	return reportSent(ctx, after, replacement, kind, wait)
}

// editedWithdrawn refuses an edit of a letter withdrawn outright.
func editedWithdrawn(old inbox.Message) error {
	return failf("your %s %s to %s was already withdrawn; send a new one with: rewake send %s \"...\"", old.Withdrawn.Kind, old.ID, old.To, old.To)
}

package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// withdrawModel is the machine form of one withdrawal. Result is unseen (no
// notice had gone out), announced (the recipient finds a tombstone), held
// (the notice still waits in the harness's approval queue) or already.
type withdrawModel struct {
	ID     string     `json:"id"`
	To     string     `json:"to"`
	Kind   inbox.Kind `json:"kind"`
	Result string     `json:"result"`
	// Recall is the note that told the recipient not to act on the notice,
	// sent when the notice may have gone out.
	Recall string `json:"recall,omitempty"`
}

var withdrawalNames = map[inbox.Withdrawal]string{
	inbox.WithdrawnUnseen:    "unseen",
	inbox.WithdrawnAnnounced: "announced",
	inbox.WithdrawnHeld:      "held",
	inbox.AlreadyWithdrawn:   "already",
}

// handleWithdraw takes back a message this run sent and nobody has read.
func handleWithdraw(ctx *Context, call Call) error {
	if len(call.Positionals) < 1 {
		return &UsageError{Command: call.Command, Message: "withdraw needs the id of the message, or a unique prefix of it."}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	_, _, message, err := sentBySelf(call, dir, call.Positionals[0], "rewake withdraw <id>")
	if err != nil {
		return err
	}
	var result inbox.Withdrawal
	if err := underMailboxLock(dir, message.To, func() error {
		result, err = inbox.Withdraw(dir, message, nil)
		return err
	}); err != nil {
		return withdrawRefusal(dir, message, err, false)
	}
	kind := inbox.KindOf(message)
	if message.Withdrawn != nil {
		kind = message.Withdrawn.Kind
	}
	model := withdrawModel{ID: message.ID, To: message.To, Kind: kind, Result: withdrawalNames[result]}
	lines := []string{withdrawLine(model, result)}
	if result == inbox.WithdrawnAnnounced {
		note, err := inbox.Recall(dir, message)
		if err != nil {
			lines = append(lines, fmt.Sprintf("Rewake: could not tell %s not to act on the notice (%v); tell it with: rewake send %s \"disregard %s\" --notify", message.To, err, message.To, shortRef(message.ID)))
		} else {
			model.Recall = note.ID
		}
	}
	return printValue(ctx, model, func() []string { return lines })
}

func withdrawLine(model withdrawModel, result inbox.Withdrawal) string {
	head := fmt.Sprintf("Rewake: withdrew your %s %s from %s", model.Kind, model.ID, model.To)
	switch result {
	case inbox.WithdrawnUnseen:
		return head + " before its notice went out; " + model.To + " saw nothing."
	case inbox.WithdrawnAnnounced:
		return head + "; its notice may have gone out, so " + model.To + " is told not to act on it and will find it marked withdrawn."
	case inbox.WithdrawnHeld:
		return head + "; its notice still waits in the approval queue of " + model.To + "'s harness, which rewake cannot empty: if it is approved, " + model.To + " finds the message marked withdrawn."
	}
	return fmt.Sprintf("Rewake: your %s %s to %s was already withdrawn.", model.Kind, model.ID, model.To)
}

// underMailboxLock runs fn under the recipient's mailbox lock, waiting for it
// as long as a reader does.
func underMailboxLock(dir, name string, fn func() error) error {
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	err := state.WithMailboxLock(wait, dir, name, fn)
	if errors.Is(err, state.ErrMailboxBusy) {
		return fmt.Errorf("the mailbox of %s stayed busy for %s", name, readerLockWait)
	}
	return err
}

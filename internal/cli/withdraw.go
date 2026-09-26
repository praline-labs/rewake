package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// withdrawModel is the machine form of one withdrawal. Result is unseen (no
// notice had gone out), announced (the recipient finds a tombstone), held
// (the notice still waits in the harness's approval queue) or already.
type withdrawModel struct {
	ID string `json:"id"`
	// Named is the id the call gave when an edit had replaced it: ID is then
	// the replacement, which is what was withdrawn.
	Named  string     `json:"named,omitempty"`
	To     string     `json:"to"`
	Kind   inbox.Kind `json:"kind"`
	Result string     `json:"result"`
	// Recall is the note that told the recipient not to act on the notice,
	// sent when the notice may have gone out.
	Recall string `json:"recall,omitempty"`
	// Addenda are the task's addenda, withdrawn with it where still unread.
	Addenda []addendumModel `json:"addenda,omitempty"`
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
	self, epoch, named, err := sentBySelf(call, dir, call.Positionals[0], "rewake withdraw <id>")
	if err != nil {
		return err
	}
	message := named
	var result inbox.Withdrawal
	var addenda []takenAddendum
	if err := underMailboxLock(dir, named.To, func() error {
		// Under the lock, so the replacement found is the one withdrawn.
		if message, err = currentSent(dir, self, epoch, named); err != nil {
			return err
		}
		if result, err = inbox.Withdraw(dir, message, nil); err != nil {
			return err
		}
		// Under the same lock: an addendum left standing would be a task
		// adding to nothing, still owing a report.
		addenda = withdrawAddenda(dir, message)
		return nil
	}); err != nil {
		var refused *FailedError
		if errors.As(err, &refused) {
			return err
		}
		return withdrawRefusal(dir, message, err, false)
	}
	kind := inbox.KindOf(message)
	if message.Withdrawn != nil {
		kind = message.Withdrawn.Kind
	}
	model := withdrawModel{ID: message.ID, To: message.To, Kind: kind, Result: withdrawalNames[result]}
	var lines []string
	if message.ID != named.ID {
		model.Named = named.ID
		lines = append(lines, redirectLine(named.ID, message.ID, "withdrawing"))
	}
	// A read addendum brought the task to the agent's attention even if the
	// task's own notice never went out, so the task is recalled for it too.
	read := readAddendum(addenda)
	recallTask := result == inbox.WithdrawnAnnounced || read && (result == inbox.WithdrawnUnseen || result == inbox.WithdrawnHeld)
	lines = append(lines, withdrawLine(model, result, read))
	if recallTask {
		lines = append(lines, recallLine(dir, message, &model.Recall)...)
	}
	more, failed := addendaLines(dir, message.To, addenda, result == inbox.AlreadyWithdrawn, &model)
	lines = append(lines, more...)
	if failed {
		if ctx.JSON {
			_ = printValue(ctx, model, func() []string { return nil })
			return &FailedError{Message: ""}
		}
		return &FailedError{Message: strings.Join(lines, "\n")}
	}
	return printValue(ctx, model, func() []string { return lines })
}

// recallLine tells the recipient not to act on a notice, and records the
// note's id; a failure to write it is said with the command to send instead.
func recallLine(dir string, message inbox.Message, recall *string) []string {
	note, err := inbox.Recall(dir, message)
	if err != nil {
		return []string{fmt.Sprintf("Rewake: could not tell %s not to act on the notice (%v); tell it with: rewake send %s \"disregard %s\" --notify", message.To, err, message.To, shortRef(message.ID))}
	}
	*recall = note.ID
	return nil
}

// withdrawLine says what became of the message itself; readAddendum says the
// recipient has read one of its addenda, and so knows of it after all.
func withdrawLine(model withdrawModel, result inbox.Withdrawal, readAddendum bool) string {
	head := fmt.Sprintf("Rewake: withdrew your %s %s from %s", model.Kind, model.ID, model.To)
	switch {
	case result == inbox.WithdrawnUnseen && readAddendum:
		return head + " before its notice went out; " + model.To + " has read an addendum to it, so it is told not to act on it and will find it marked withdrawn."
	case result == inbox.WithdrawnUnseen:
		return head + " before its notice went out; " + model.To + " saw nothing."
	case result == inbox.WithdrawnAnnounced:
		return head + "; its notice may have gone out, so " + model.To + " is told not to act on it and will find it marked withdrawn."
	case result == inbox.WithdrawnHeld:
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

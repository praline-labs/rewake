package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/inbox"
)

// addendumModel is what withdrawing a task did to one of its addenda: unseen,
// announced, held or already, as for the task; read when the recipient had
// read it, which is final; failed when the withdrawal did not finish.
type addendumModel struct {
	ID     string `json:"id"`
	Result string `json:"result"`
	Recall string `json:"recall,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// takenAddendum is one addendum's withdrawal, before anything is printed.
type takenAddendum struct {
	message inbox.Message
	result  inbox.Withdrawal
	err     error
}

// withdrawAddenda takes back the addenda of a task being withdrawn. An addendum
// is a task that only makes sense with its task: left standing, it would be
// read on its own and owe a report on work its sender took back. The caller
// holds the recipient's mailbox lock.
func withdrawAddenda(dir string, task inbox.Message) []takenAddendum {
	var taken []takenAddendum
	for _, addendum := range inbox.AddendaOf(dir, task) {
		result, err := inbox.Withdraw(dir, addendum, nil)
		taken = append(taken, takenAddendum{message: addendum, result: result, err: err})
	}
	return taken
}

// readAddendum says whether the recipient read one of the addenda already.
func readAddendum(taken []takenAddendum) bool {
	for _, addendum := range taken {
		if errors.Is(addendum.err, inbox.ErrAlreadyRead) {
			return true
		}
	}
	return false
}

// addendaLines says what became of each addendum, sends the recalls their
// notices call for, and fills the model. again says the task was withdrawn by
// an earlier call, which sent any recall a read addendum called for. failed
// says a withdrawal did not finish, which makes the call's exit 1.
func addendaLines(dir, to string, taken []takenAddendum, again bool, model *withdrawModel) ([]string, bool) {
	var lines []string
	failed := false
	for _, addendum := range taken {
		id := addendum.message.ID
		entry := addendumModel{ID: id}
		switch {
		case addendum.err == nil:
			entry.Result = withdrawalNames[addendum.result]
			switch addendum.result {
			case inbox.WithdrawnUnseen:
				lines = append(lines, fmt.Sprintf("Rewake: withdrew its addendum %s too, before its notice went out.", id))
			case inbox.WithdrawnAnnounced:
				lines = append(lines, fmt.Sprintf("Rewake: withdrew its addendum %s too; its notice may have gone out, so %s is told not to act on it.", id, to))
				lines = append(lines, recallLine(dir, addendum.message, &entry.Recall)...)
			case inbox.WithdrawnHeld:
				lines = append(lines, fmt.Sprintf("Rewake: withdrew its addendum %s too; its notice still waits in the approval queue of %s's harness.", id, to))
			}
		case errors.Is(addendum.err, inbox.ErrAlreadyRead):
			entry.Result = "read"
			told := "is told the task was withdrawn"
			if again {
				told = "was told the task was withdrawn when it was"
			}
			lines = append(lines, fmt.Sprintf("Rewake: %s has read its addendum %s already, and a read message is final; %s %s and reports on the addendum as usual.", to, id, to, told))
		case errors.Is(addendum.err, inbox.ErrNotDelivered), errors.Is(addendum.err, inbox.ErrNoLongerKept):
			// Never read and never will be: nothing to take back or to say.
			entry.Result = "undelivered"
		default:
			entry.Result, entry.Detail, failed = "failed", addendum.err.Error(), true
			lines = append(lines, fmt.Sprintf("Rewake: could not withdraw its addendum %s (%v); finish it with: rewake withdraw %s", id, addendum.err, shortRef(id)))
		}
		model.Addenda = append(model.Addenda, entry)
	}
	return lines, failed
}

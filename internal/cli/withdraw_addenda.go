package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/mail"
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

// readAddendum says whether the recipient read one of the addenda already.
func readAddendum(taken []mail.AddendumWithdrawn) bool {
	for _, addendum := range taken {
		if errors.Is(addendum.Err, mail.ErrAlreadyRead) {
			return true
		}
	}
	return false
}

// addendaLines says what became of each addendum, sends the recalls their
// notices call for, and fills the model. again says the task was withdrawn by
// an earlier call, which sent any recall a read addendum called for. failed
// says a withdrawal did not finish, which makes the call's exit 1.
func addendaLines(dir, to string, taken []mail.AddendumWithdrawn, again bool, model *withdrawModel) ([]string, bool) {
	var lines []string
	failed := false
	for _, addendum := range taken {
		id := addendum.Letter.ID
		entry := addendumModel{ID: id}
		switch {
		case addendum.Err == nil:
			entry.Result = withdrawalNames[addendum.Result]
			switch addendum.Result {
			case mail.WithdrawnUnseen:
				lines = append(lines, fmt.Sprintf("Rewake: withdrew its addendum %s too, before its notice went out.", id))
			case mail.WithdrawnAnnounced:
				lines = append(lines, fmt.Sprintf("Rewake: withdrew its addendum %s too; its notice may have gone out, so %s is told not to act on it.", id, to))
				lines = append(lines, recallLine(dir, addendum.Letter, &entry.Recall)...)
			case mail.WithdrawnHeld:
				lines = append(lines, fmt.Sprintf("Rewake: withdrew its addendum %s too; its notice still waits in the approval queue of %s's harness.", id, to))
			}
		case errors.Is(addendum.Err, mail.ErrAlreadyRead):
			entry.Result = "read"
			told := "is told the task was withdrawn"
			if again {
				told = "was told the task was withdrawn when it was"
			}
			lines = append(lines, fmt.Sprintf("Rewake: %s has read its addendum %s already, and a read message is final; %s %s and reports on the addendum as usual.", to, id, to, told))
		case errors.Is(addendum.Err, mail.ErrNotDelivered), errors.Is(addendum.Err, mail.ErrNoLongerKept):
			// Never read and never will be: nothing to take back or to say.
			entry.Result = "undelivered"
		default:
			entry.Result, entry.Detail, failed = "failed", addendum.Err.Error(), true
			lines = append(lines, fmt.Sprintf("Rewake: could not withdraw its addendum %s (%v); finish it with: rewake withdraw %s", id, addendum.Err, shortRef(id)))
		}
		model.Addenda = append(model.Addenda, entry)
	}
	return lines, failed
}

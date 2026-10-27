package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
)

// briefFlagFits refuses --brief beside a kind that asks for no work: a
// binding without a task would give a worker a brief it was never told of.
func briefFlagFits(call Call, kind messageKind) error {
	value, given := call.Flags["brief"]
	if !given {
		if call.Switch("review") {
			return &UsageError{Command: call.Command, Message: "--review says how a brief is given, and goes with --brief <id>."}
		}
		return nil
	}
	if kind.kind != mail.Task && kind.kind != mail.Question {
		return &UsageError{Command: call.Command, Message: "--brief goes with a task or a --question, which the recipient works on; a notify asks for nothing."}
	}
	if _, ok := taskbrief.ParseID(value); !ok {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("--brief takes a brief's id, a number from 1 as rewake brief list shows, not %q.", value)}
	}
	return nil
}

// bindBrief gives the brief --brief names to the recipient's run, after
// every other check of send and before the letter is written, and answers
// the id the letter carries: as work, or with --review to read and comment
// only. Only main gives a brief. A send that fails after this leaves the run
// bound, a known limit (docs/briefs.md).
func bindBrief(call Call, dir string, self registry.Session, selfErr error, recipient registry.Session) (string, error) {
	value, given := call.Flags["brief"]
	if !given {
		return "", nil
	}
	if selfErr != nil {
		return "", failf("only main gives a brief, and this shell is not a session: %v.", selfErr)
	}
	if self.Role != role.Main.ID {
		return "", failf("only main gives a brief; %s is a %s session. Ask main to do it.", self.Name, self.Role)
	}
	id, _ := taskbrief.ParseID(value)
	kind := taskbrief.Work
	if call.Switch("review") {
		kind = taskbrief.Review
	}
	err := taskbrief.Bind(quietHomeOf(dir), id, taskbrief.Author{Name: recipient.Name, Epoch: recipient.Epoch()}, kind)
	var closed *taskbrief.ClosedError
	switch {
	case errors.Is(err, taskbrief.ErrNotFound):
		return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("no brief %d: rewake brief list shows the open ones; nothing was sent.", id)}
	case errors.As(err, &closed):
		return "", failf("%v; nothing was sent.", err)
	case err != nil:
		return "", failf("could not give brief %d to %s: %v; nothing was sent.", id, recipient.Name, err)
	}
	return taskbrief.FormatID(id), nil
}

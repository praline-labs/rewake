package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/grant/authority"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// decideHandler is the transaction this wrapper runs for a decision a command
// below it asks for (docs/v2/stage3-decision.md#authority): verified, as this
// run, on a report addressed to this run and no other. It runs as a worker of
// the run, under its hold on the room's lease, and none starts once the run
// has begun to end, so no decision is written in this run's name after it.
func decideHandler(dir, name, epoch string, life *lifetime) authority.DecideHandler {
	return func(ctx context.Context, ask mail.DecideRequest) (mail.DecideAnswer, error) {
		if !life.admit() {
			return mail.DecideAnswer{}, &mail.DecisionError{Outcome: mail.DecisionBusy, Err: errors.New("this session is ending and takes no decision")}
		}
		defer life.dismiss()
		state.Step("decision admitted")
		if ask.Expect.Recipient != name || ask.Expect.RecipientRun != epoch {
			return mail.DecideAnswer{}, &mail.DecisionError{Outcome: mail.DecisionUnauthorized, Err: fmt.Errorf(
				"report %s of %s was addressed to %s (run %s), and this wrapper decides only for its own run, %s of %s",
				ask.Report, ask.Sender, ask.Expect.Recipient, ask.Expect.RecipientRun, epoch, name)}
		}
		ask.By = mail.DecidedBy{Kind: mail.ByRecipient, Name: name, Run: epoch, Verified: true}
		return mail.Decide(ctx, dir, ask)
	}
}

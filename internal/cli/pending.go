package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// pendingModel is what rewake pending answers.
type pendingModel struct {
	Session string   `json:"session"`
	Text    string   `json:"text"`
	Waiting []string `json:"waiting"`
}

// handlePending marks the running turn as not the end of the work. The mark is
// consumed by that turn's end, which then tells the senders the work is still
// going instead of reporting; the obligation stays open for the next turn end.
func handlePending(ctx *Context, call Call) error {
	text := ""
	if len(call.Positionals) > 0 {
		text = strings.TrimSpace(call.Positionals[0])
	}
	if text == "" {
		return &UsageError{Command: call.Command, Message: "rewake pending needs the text to send: what the work is waiting for."}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	self, box, err := ownMail(accessChange, dir)
	if err != nil {
		return &UsageError{Command: call.Command, Message: "rewake pending marks the turn of the session running it, and this is not one: " + err.Error() + "."}
	}
	if !mail.OwesReports(self.Role) {
		return &UsageError{Command: call.Command, Message: "the " + self.Role + " session's turns are reported to nobody, so there is nothing to keep open; end the turn as usual."}
	}
	// The mark is tied to the turn by the time this call started, on the boot
	// clock.
	at := boottime.ProcessStarted
	if at <= 0 {
		// A mark with no time would belong to no turn
		// (docs/delivery-turn-end.md#pending-marks).
		return failf("the time this call started cannot be read, so a mark could not be tied to this turn; nothing was marked")
	}
	waiting, err := box.MarkTurnPending(context.Background(), text, at)
	var step *mail.StepError
	switch {
	case errors.Is(err, mail.ErrTurnEnded):
		return failf("not marked: the turn ended first, so its end reported without this mark")
	case errors.Is(err, mail.ErrNothingOwed):
		return &UsageError{Command: call.Command, Message: "nothing is owed a report, so there is nothing to keep open; end the turn as usual."}
	case errors.As(err, &step) && step.Step == mail.StepTurnStart:
		return failf("the latest start of this turn cannot be read, so a mark could not be tied to this turn; nothing was marked: %v", err)
	case errors.As(err, &step) && step.Step == mail.StepWaiters:
		return failf("could not mark the turn pending (could not read who waits for this turn: %v); nothing was marked, so run rewake pending again", err)
	case err != nil:
		return failf("could not mark the turn pending (%v); nothing was marked, so run rewake pending again", err)
	}
	model := pendingModel{Session: self.Name, Text: text, Waiting: waiting}
	return printValue(ctx, model, func() []string {
		return []string{"Rewake: marked pending; at this turn's end " + strings.Join(model.Waiting, ", ") + " will read that the work goes on."}
	})
}

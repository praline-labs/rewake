package cli

import (
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// reportSent waits for the delivery result of a message just written and
// prints it, then hands over to what its kind does next — a question waits
// for its answer.
func reportSent(ctx *Context, after sent, message mail.Message, kind messageKind, wait time.Duration) error {
	session := after.target
	delivery := deliveryWait(kind, wait)
	model := sendModel{ID: message.ID, To: session.Name, From: message.From, Addenda: after.addenda, Replaces: message.Replaces, Named: after.named}
	got := after.box.AwaitSent(session, message.ID, delivery)
	model.State, model.Via, model.Detail = string(got.State), got.Via, got.Detail
	switch got.Fate {
	case mail.FateRead:
		model.Detail = "already read"
	case mail.FateUnknown:
		model.Detail = fmt.Sprintf("its result could not be read (%v); the session has it, and rewake inbox --awaited shows where it stands", got.Err)
	case mail.FateEnded:
		model.Detail = "the session ended before the message was delivered"
	case mail.FateReplaced:
		model.Detail = "the session ended and another one took its name before the message was delivered"
	case mail.FateAnsweredLongAgo:
		model.Detail = "this message was answered earlier and the result is no longer kept"
	case mail.FateNoResult:
		model.Detail = fmt.Sprintf("no result yet after %s; the session has it and will take it", delivery)
	}

	if mail.State(model.State) != mail.Failed && kind.after != nil {
		// The id before the wait, which may be long: it is what an edit or an
		// addendum to this message takes while the sender still waits.
		if !ctx.JSON {
			_ = emit(ctx, idLines(model)...)
		}
		after.model = model
		return kind.after(ctx, after)
	}
	return printDelivery(ctx, session, model)
}

// deliveryWait is how much of wait goes to the delivery. A kind that waits for
// something after it — a question's answer — gives it a few seconds at most;
// any other waits for the delivery as long as --wait says.
func deliveryWait(kind messageKind, wait time.Duration) time.Duration {
	if kind.after != nil {
		return min(wait, defaultWait)
	}
	return wait
}

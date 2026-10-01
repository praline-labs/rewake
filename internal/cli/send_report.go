package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// reportSent waits for the delivery result of a message just written and
// prints it, then hands over to what its kind does next — a question waits
// for its answer.
func reportSent(ctx *Context, after sent, message inbox.Message, kind messageKind, wait time.Duration) error {
	dir, session := after.dir, after.target
	// A kind that waits for something after the delivery — a question's
	// answer — gives the delivery a few seconds of its wait at most; any other
	// waits for the delivery as long as --wait says.
	delivery := wait
	if kind.after != nil {
		delivery = min(wait, defaultWait)
	}
	status, known := awaitStatus(dir, session.Name, message.ID, delivery, recipientRunning(dir, session))
	model := sendModel{ID: message.ID, To: session.Name, From: message.From, GrantGit: message.GrantGit, GrantDirs: message.GrantDirs, AlreadyWritable: after.writable, Addenda: after.addenda, Replaces: message.Replaces, Named: after.named}
	if ctx.op != nil {
		model.Receipt = ctx.op.record.Token
	}
	// What cannot be read says neither delivered nor lost: the answer is
	// pending, which sends nobody to do it twice. Await takes a status it
	// cannot read for none yet, so one more look tells the two apart.
	var answered bool
	var unknown error
	if !known {
		status, known, unknown = inbox.ReadStatus(dir, session.Name, message.ID)
		known = known && unknown == nil
	}
	if !known && unknown == nil {
		answered, unknown = inbox.Answered(dir, session.Name, message.ID)
	}
	if answered {
		// The message left the mailbox, and a status may have been written
		// after the wait gave up. Absent a moment ago is not absent now:
		// calling a fresh delivery lost sends the sender to do it twice.
		status, known, unknown = inbox.ReadStatus(dir, session.Name, message.ID)
		known = known && unknown == nil
	}
	if known {
		model.GrantApplied = status.GrantApplied
	}
	switch {
	case known && status.State == inbox.Read:
		// Read already, which is delivered and then some.
		model.State, model.Via = string(inbox.Delivered), status.Via
		model.Detail = "already read"
	case known && status.State != inbox.Pending && status.State != inbox.Held:
		model.State, model.Via, model.Detail = string(status.State), status.Via, status.Detail
	default:
		// No final answer. Held is none either: it waits for the session that
		// holds it, and a session that is gone will never release it.
		// Promising a later delivery is only honest while the session is still
		// there to make one; a session that ended between the lookup and now
		// leaves the message with nobody to take it.
		// The name is not enough: it may already belong to a session that
		// started after this message was written, and that session will refuse
		// it. Promising a later delivery then would be a promise nobody keeps.
		// None of that holds for a result that could not be read: it may say
		// delivered or read, whoever holds the name now.
		if unknown != nil {
			model.State = string(inbox.Pending)
			model.Detail = fmt.Sprintf("its result could not be read (%v); the session has it, and rewake inbox --awaited shows where it stands", unknown)
			break
		}
		current, err := registry.Lookup(dir, session.Name)
		if errors.Is(err, registry.ErrNotFound) {
			model.State = string(inbox.Failed)
			model.Detail = "the session ended before the message was delivered"
			break
		}
		if err == nil && current.Epoch() != session.Epoch() {
			model.State = string(inbox.Failed)
			model.Detail = "the session ended and another one took its name before the message was delivered"
			break
		}
		if answered && !known {
			// The message is out of the mailbox but has no status: it was
			// answered long enough ago that the answer is no longer kept.
			// Saying "pending" here would promise a delivery that has happened.
			model.State = string(inbox.Failed)
			model.Detail = "this message was answered earlier and the result is no longer kept"
			break
		}
		if known && status.State == inbox.Held {
			model.State, model.Via, model.Detail = string(inbox.Held), status.Via, status.Detail
			break
		}
		model.State = string(inbox.Pending)
		if known && status.Detail != "" {
			model.Detail = status.Detail
		} else {
			model.Detail = fmt.Sprintf("no result yet after %s; the session has it and will take it", delivery)
		}
	}

	if inbox.State(model.State) != inbox.Failed && kind.after != nil {
		// The id before the wait, which may be long: it is what an edit or an
		// addendum to this message takes while the sender still waits.
		if !ctx.JSON {
			_ = emit(ctx, append(sentGrantLines(session, model), idLines(model)...)...)
		}
		after.model = model
		return kind.after(ctx, after)
	}
	return printDelivery(ctx, session, model)
}

// awaitStatus waits for the status of a message; replaceable in tests.
var awaitStatus = inbox.Await

// recipientRunning says whether the run a message was written for still runs,
// so a long --wait ends with it. A run in another pid namespace cannot be
// judged from here and counts as running, as everywhere else: its wrapper,
// which can see it, is the one that delivers. A record that cannot be read
// says nothing either way.
func recipientRunning(dir string, target registry.Session) func() bool {
	return func() bool {
		current, err := registry.LookupReadOnly(dir, target.Name)
		if errors.Is(err, registry.ErrNotFound) {
			return false
		}
		return err != nil || current.Epoch() == target.Epoch()
	}
}

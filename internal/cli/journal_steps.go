package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// toolMargin is kept between the last wait of a tool call and its deadline,
// for the answer to get back before the harness stops waiting.
const toolMargin = 500 * time.Millisecond

// pinnedNotify is the heads-up a resumed operation fixed earlier, or nil.
func (op *operation) pinnedNotify() *receipt.NotifyStep {
	if op == nil {
		return nil
	}
	return op.record.Notify
}

// resolveEndedRun settles a heads-up pinned to a recipient run that is no
// longer the live one, before anything else is looked up: it never goes to
// the session that took the name since, and what it did in the run that ended
// is read from that run's mailbox. Not having recorded the publication is no
// proof it did not happen — a crash between the two leaves exactly that — so
// when the mailbox no longer shows either way, the answer says the effect is
// unknown and the record is kept as such. A lookup that failed tells nothing,
// neither whether the run ended nor what its mailbox holds: the operation
// stays open for a retry.
func (op *operation) resolveEndedRun(ctx *Context) error {
	step := op.pinnedNotify()
	if step == nil || step.Published {
		return nil
	}
	current, err := registry.Lookup(op.site.dir, step.To)
	switch {
	case err == nil && current.Epoch() == step.ToEpoch:
		return nil
	case err != nil && !errors.Is(err, registry.ErrNotFound):
		return &unfinishedError{message: fmt.Sprintf("Rewake: could not tell whether the run of %s this heads-up was written for still lives (%v), so nothing was sent; finish it with: rewake retry %s", step.To, err, op.record.Token)}
	}
	wait := readerLockWait
	if ctx.scope != nil {
		wait = max(time.Millisecond, min(wait, ctx.scope.remaining()))
	}
	lockCtx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	publication, err := inbox.PublicationOf(lockCtx, op.site.dir, step.To, step.ToEpoch, step.MessageID)
	if err != nil {
		return &unfinishedError{message: fmt.Sprintf("Rewake: the run of %s this heads-up was written for has ended, and its mailbox could not be read to tell whether it got there (%v); nothing was sent, and it does not go to the session that took the name since; look again with: rewake retry %s", step.To, err, op.record.Token)}
	}
	switch publication {
	case inbox.PublicationWritten:
		step.Published = true
		return failf("this heads-up was written into the mailbox of the run of %s that has since ended, and does not go to the session that took the name since", step.To)
	case inbox.PublicationAbsent:
		return failf("the run of %s this heads-up was written for ended before it was published, so it was not; it does not go to the session that took the name since", step.To)
	}
	op.record.Uncertain = true
	return failf("the run of %s this heads-up was written for has ended, and whether it reached that run is no longer known; it does not go to the session that took the name since", step.To)
}

// pinNotify fixes a heads-up's id, its text and its recipient's run in the
// journal before anything is published; a resumed operation takes its id from
// there, and handleSend its text, before it would read stdin again.
func (op *operation) pinNotify(message *inbox.Message) error {
	step := op.record.Notify
	if step == nil {
		op.record.Notify = &receipt.NotifyStep{MessageID: message.ID, To: message.To, ToEpoch: message.ToEpoch, Text: message.Text}
		return op.save()
	}
	if step.To != message.To || step.ToEpoch != message.ToEpoch {
		return failf("the receipt %s is a heads-up to another run of %s", op.record.Token, step.To)
	}
	message.ID = step.MessageID
	return nil
}

// publish writes a journaled heads-up into its recipient's mailbox once. The
// deadline is checked under the recipient's lock, just before the write.
func (op *operation) publish(ctx *Context, message inbox.Message) error {
	wait := readerLockWait
	if ctx.scope != nil {
		wait = max(time.Millisecond, min(wait, ctx.scope.remaining()))
	}
	lockCtx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, err := inbox.PublishOnce(lockCtx, op.site.dir, message, func() error {
		return beforeCommit(ctx, op.record.Token, "the heads-up was published")
	})
	var unfinished *unfinishedError
	switch {
	case errors.As(err, &unfinished):
		return err
	case errors.Is(err, inbox.ErrRecipientEnded):
		// Read inside the recipient's lock: the retry learns from that run's
		// mailbox what an earlier attempt did there.
		return &unfinishedError{message: "Rewake: the run of " + message.To + " this heads-up was written for ended before it was published, so this attempt wrote nothing; settle it with: rewake retry " + op.record.Token}
	case errors.Is(err, state.ErrMailboxBusy):
		return &unfinishedError{message: "Rewake: the mailbox of " + message.To + " was busy, so the heads-up was not published yet; publish it with: rewake retry " + op.record.Token}
	case err != nil:
		return &unfinishedError{message: "Rewake: could not write the heads-up into the mailbox of " + message.To + " (" + err.Error() + "); publish it with: rewake retry " + op.record.Token}
	}
	op.record.Notify.Published = true
	if err := op.save(); err != nil {
		// Published and not recorded: the retry finds the letter's mark and
		// writes nothing again.
		return &unfinishedError{message: fmt.Sprintf("Rewake: the heads-up was published, but recording it failed (%v); finish it with: rewake retry %s", err, op.record.Token)}
	}
	return nil
}

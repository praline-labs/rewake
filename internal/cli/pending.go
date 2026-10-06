package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/harness/claude/telemetry"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// pendingModel is what rewake pending answers.
type pendingModel struct {
	Session string   `json:"session"`
	Text    string   `json:"text"`
	Waiting []string `json:"waiting"`
	// Receipt names the journal record of the mark, for rewake retry.
	Receipt string `json:"receipt,omitempty"`
}

// claudeHarnessID is the Claude Code harness's id, spelled here rather than
// imported from its adapter, which the command layer does not depend on.
const claudeHarnessID = "claude"

// pendingLockWait bounds the wait for the mailbox, as reading does.
const pendingLockWait = 5 * time.Second

// handlePending marks the running turn as not the end of the work. The mark is
// consumed by that turn's end, which then tells the senders the work is still
// going instead of reporting; the obligation stays open for the next turn end.
func handlePending(ctx *Context, call Call) error {
	if !ctx.journaling {
		// A mark is one of the tool's words: its receipt ties a retry to the
		// call that made it, never to a later turn.
		return journaled(ctx, call, handlePending)
	}
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
	self, epoch, err := ownRun(dir)
	if err != nil {
		return &UsageError{Command: call.Command, Message: "rewake pending marks the turn of the session running it, and this is not one: " + err.Error() + "."}
	}
	if role.Of(self.Role).Silent {
		return &UsageError{Command: call.Command, Message: "the " + self.Role + " session's turns are reported to nobody, so there is nothing to keep open; end the turn as usual."}
	}
	if self.Harness == claudeHarnessID && telemetry.ReadTurnStart(telemetry.TurnStartPath(registry.ObservationFor(dir, self.Name, epoch))) == 0 {
		// Claude Code tells rewake a turn started only through the telemetry
		// hook. Without its record the mark could not be tied to this turn,
		// and a mark that might belong to another is worse than none.
		return &UsageError{Command: call.Command, Message: "this session records no turn starts (its telemetry hooks are not running), so a mark could not be tied to this turn; end the turn with the result, or wait inside it."}
	}
	at, file, err := pendingTime(ctx, text)
	if err != nil {
		return err
	}
	if at <= 0 {
		// A mark with no time would belong to no turn
		// (docs/turn-end-recovery.md#pending-marks).
		return failf("the time this call started cannot be read, so a mark could not be tied to this turn; nothing was marked")
	}
	model := pendingModel{Session: self.Name, Text: text}
	if ctx.op != nil {
		model.Receipt, text = ctx.op.record.Token, ctx.op.record.Pending.Text
	}
	token := ""
	if ctx.op != nil {
		token = ctx.op.record.Token
	}
	wait := pendingLockWait
	if ctx.scope != nil {
		wait = max(time.Millisecond, min(wait, ctx.scope.remaining()))
	}
	lockCtx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	var waiting []string
	var late error
	verdict := markWrite
	err = state.WithMailboxLock(lockCtx, dir, self.Name, func() error {
		// Under the lock, just before the mark: the wait for it may have
		// outlasted the call.
		if late = beforeCommit(ctx, token, "the turn was marked"); late != nil {
			return nil
		}
		// A stopped mailbox changes nothing until a person settles it.
		if err := inbox.MailboxStopped(dir, self.Name); err != nil {
			return err
		}
		// A record that cannot be read may be a waiter: "nothing owed" is
		// an answer only when every one of them was read.
		waiters, err := inbox.ReadWaiters(dir, self.Name, epoch)
		if err != nil {
			return fmt.Errorf("could not read who waits for this turn: %w", err)
		}
		for _, waiter := range waiters {
			waiting = append(waiting, waiter.Name)
		}
		// A letter a tool read showed and the wrapper has not confirmed yet
		// is owed as surely: its confirmation comes before the turn ends.
		claimedBy, err := inbox.ClaimedOwedBy(dir, self.Name, epoch)
		if err != nil {
			return fmt.Errorf("could not read the letters being read in parts: %w", err)
		}
		for _, name := range claimedBy {
			if !contains(waiting, name) {
				waiting = append(waiting, name)
			}
		}
		if verdict, err = judgeMark(ctx, dir, self, epoch, file, at); err != nil || verdict != markWrite {
			return err
		}
		if len(waiting) == 0 {
			return nil
		}
		return inbox.MarkPending(dir, self.Name, epoch, file, text, at)
	})
	if late != nil {
		return late
	}
	switch verdict {
	case markTurnEnded:
		return failf("not marked: the turn ended first, so its end reported without this mark")
	case markUnproven:
		return failf("not marked: this call cannot show it runs in the turn the mark was for, and a mark is never written for another turn")
	}
	if err != nil && ctx.op != nil {
		return &unfinishedError{message: fmt.Sprintf("Rewake: could not mark the turn pending (%v); mark it with: rewake retry %s", err, token)}
	}
	if err != nil {
		return failf("could not mark the turn pending: %v", err)
	}
	if len(waiting) == 0 && verdict != markFound {
		return &UsageError{Command: call.Command, Message: "nothing is owed a report, so there is nothing to keep open; end the turn as usual."}
	}
	if ctx.op != nil {
		ctx.op.record.Pending.Marked = true
	}
	model.Waiting = waiting
	return printValue(ctx, model, func() []string {
		return []string{"Rewake: marked pending; at this turn's end " + strings.Join(model.Waiting, ", ") + " will read that the work goes on."}
	})
}

// pendingTime is when the mark was asked for, on the boot clock, and the
// file its mark is kept in: the tool call's own time, since the process
// resuming it may run in a later turn, and a journaled mark's recorded time
// and file on every retry, so a retry never makes a second mark.
func pendingTime(ctx *Context, text string) (int64, string, error) {
	at := boottime.ProcessStarted
	if ctx.scope != nil {
		at = ctx.scope.ticket.CalledBoot
	}
	if ctx.op == nil {
		return at, inbox.MarkName(at, inbox.NewID()), nil
	}
	if step := ctx.op.record.Pending; step != nil {
		return step.At, step.Mark, nil
	}
	ctx.op.record.Pending = &receipt.PendingStep{Text: text, At: at, Mark: inbox.MarkName(at, inbox.NewID())}
	return at, ctx.op.record.Pending.Mark, ctx.op.save()
}

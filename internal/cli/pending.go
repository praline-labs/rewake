package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// pendingModel is what rewake pending answers.
type pendingModel struct {
	Session string   `json:"session"`
	Text    string   `json:"text"`
	Waiting []string `json:"waiting"`
	// Receipt names the journal record of the mark, for rewake retry.
	Receipt string `json:"receipt,omitempty"`
}

// pendingLockWait bounds the wait for the mailbox, as reading does.
const pendingLockWait = 5 * time.Second

// handlePending marks the running turn as not the end of the work. The mark is
// consumed by that turn's end, which then tells the senders the work is still
// going instead of reporting; the obligation stays open for the next turn end.
func handlePending(ctx *Context, call Call) error {
	if !ctx.journaling {
		// A mark's receipt ties a retry to the call that made it, never to a
		// later turn.
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
	if _, err := mail.LatestTurnStart(dir, self.Name, epoch); err != nil {
		return failf("the latest start of this turn cannot be read, so a mark could not be tied to this turn; nothing was marked: %v", err)
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
	lockCtx, cancel := context.WithTimeout(context.Background(), pendingLockWait)
	defer cancel()
	var waiting []string
	verdict := mail.VerdictWrite
	err = state.WithMailboxLock(lockCtx, dir, self.Name, func() error {
		// A record that cannot be read may be a waiter: "nothing owed" is
		// an answer only when every one of them was read.
		waiters, err := mail.ReadWaiters(dir, self.Name, epoch)
		if err != nil {
			return fmt.Errorf("could not read who waits for this turn: %w", err)
		}
		for _, waiter := range waiters {
			waiting = append(waiting, waiter.Name)
		}
		started, err := mail.LatestTurnStart(dir, self.Name, epoch)
		if err != nil {
			return fmt.Errorf("the latest start of this turn cannot be read, so the mark cannot be tied to its turn: %w", err)
		}
		if verdict, err = mail.JudgeMark(dir, self, epoch, file, at, attemptScope(ctx.op), started); err != nil || verdict != mail.VerdictWrite {
			return err
		}
		if len(waiting) == 0 {
			return nil
		}
		return mail.MarkPending(dir, self.Name, epoch, file, text, at)
	})
	switch verdict {
	case mail.VerdictTurnEnded:
		return failf("not marked: the turn ended first, so its end reported without this mark")
	case mail.VerdictUnproven:
		return failf("not marked: this call cannot show it runs in the turn the mark was for, and a mark is never written for another turn")
	}
	if err != nil && ctx.op != nil {
		return &unfinishedError{message: fmt.Sprintf("Rewake: could not mark the turn pending (%v); mark it with: rewake retry %s", err, token)}
	}
	if err != nil {
		return failf("could not mark the turn pending: %v", err)
	}
	if len(waiting) == 0 && verdict != mail.VerdictFound {
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
// file its mark is kept in: a journaled mark's recorded time and file on every
// retry, so a retry never makes a second mark.
func pendingTime(ctx *Context, text string) (int64, string, error) {
	at := boottime.ProcessStarted
	if ctx.op == nil {
		return at, mail.MarkName(at, mail.NewID()), nil
	}
	if step := ctx.op.record.Pending; step != nil {
		return step.At, step.Mark, nil
	}
	ctx.op.record.Pending = &receipt.PendingStep{Text: text, At: at, Mark: mail.MarkName(at, mail.NewID())}
	return at, ctx.op.record.Pending.Mark, ctx.op.save()
}

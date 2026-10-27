package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// pendingLockWait bounds the wait for the mailbox, as reading does.
const pendingLockWait = 5 * time.Second

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
	self, epoch, err := ownRun(dir)
	if err != nil {
		return &UsageError{Command: call.Command, Message: "rewake pending marks the turn of the session running it, and this is not one: " + err.Error() + "."}
	}
	if !mail.OwesReports(self.Role) {
		return &UsageError{Command: call.Command, Message: "the " + self.Role + " session's turns are reported to nobody, so there is nothing to keep open; end the turn as usual."}
	}
	if _, err := mail.LatestTurnStart(dir, self.Name, epoch); err != nil {
		return failf("the latest start of this turn cannot be read, so a mark could not be tied to this turn; nothing was marked: %v", err)
	}
	// The mark is tied to the turn by the time this call started, on the boot
	// clock.
	at := boottime.ProcessStarted
	if at <= 0 {
		// A mark with no time would belong to no turn
		// (docs/delivery-turn-end.md#pending-marks).
		return failf("the time this call started cannot be read, so a mark could not be tied to this turn; nothing was marked")
	}
	file := mail.MarkName(at, mail.NewID())
	model := pendingModel{Session: self.Name, Text: text}
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
		if verdict, err = mail.JudgeMark(dir, self, epoch, at, started); err != nil || verdict != mail.VerdictWrite {
			return err
		}
		if len(waiting) == 0 {
			return nil
		}
		return mail.MarkPending(dir, self.Name, epoch, file, text, at)
	})
	if verdict == mail.VerdictTurnEnded {
		return failf("not marked: the turn ended first, so its end reported without this mark")
	}
	if err != nil {
		return failf("could not mark the turn pending (%v); nothing was marked, so run rewake pending again", err)
	}
	if len(waiting) == 0 {
		return &UsageError{Command: call.Command, Message: "nothing is owed a report, so there is nothing to keep open; end the turn as usual."}
	}
	model.Waiting = waiting
	return printValue(ctx, model, func() []string {
		return []string{"Rewake: marked pending; at this turn's end " + strings.Join(model.Waiting, ", ") + " will read that the work goes on."}
	})
}

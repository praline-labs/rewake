package cli

import (
	"context"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/harness/claude/telemetry"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// pendingModel is what rewake pending answers.
type pendingModel struct {
	Session string   `json:"session"`
	Text    string   `json:"text"`
	Waiting []string `json:"waiting"`
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
	model := pendingModel{Session: self.Name, Text: text}
	lockCtx, cancel := context.WithTimeout(context.Background(), pendingLockWait)
	defer cancel()
	var waiting []inbox.Waiter
	err = state.WithMailboxLock(lockCtx, dir, self.Name, func() error {
		waiting = inbox.Waiters(dir, self.Name, epoch)
		if len(waiting) == 0 {
			return nil
		}
		return inbox.MarkPending(dir, self.Name, epoch, text, boottime.ProcessStarted)
	})
	if err != nil {
		return failf("could not mark the turn pending: %v", err)
	}
	if len(waiting) == 0 {
		return &UsageError{Command: call.Command, Message: "nothing is owed a report, so there is nothing to keep open; end the turn as usual."}
	}
	for _, waiter := range waiting {
		model.Waiting = append(model.Waiting, waiter.Name)
	}
	return printValue(ctx, model, func() []string {
		return []string{"Rewake: marked pending; at this turn's end " + strings.Join(model.Waiting, ", ") + " will read that the work goes on."}
	})
}

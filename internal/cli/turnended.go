package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/claude/telemetry"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// maxPayload bounds what is read from a hook. The payload is a small JSON
// object; anything much larger is not one.
const maxPayload = 4 << 20

// payloadWait bounds the wait for a payload on stdin. A hook gets its payload
// at once and the pipe closed; one that stays open with nothing on it would
// otherwise hold the end of the turn for as long as the harness allows.
const payloadWait = 3 * time.Second

// handleTurnEnded is called by a harness at the end of every turn of its
// session. It tells each session run whose message was read during that turn
// that the turn is over, and passes the last reply along.
//
// It never fails loudly. It runs inside the harness's own machinery — a Claude
// Code Stop hook, a Codex notify program — where an error is at best noise on
// the screen and at worst a turn that will not end. What it could not do stays
// owed and is tried again at the end of the next turn.
func handleTurnEnded(ctx *Context, call Call) error {
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	self, _, err := ownRun(dir)
	if err != nil {
		// A stale hook cannot report for a newer run of the same name.
		return nil
	}

	var payload []byte
	if len(call.Positionals) > 0 {
		// Codex passes the payload as the last argument.
		payload = []byte(call.Positionals[len(call.Positionals)-1])
	} else {
		// A Claude Code hook gets it on stdin.
		payload = readPayload(os.Stdin)
	}
	event, ok := completedTurn(payload)
	if !ok {
		return nil
	}
	// The end of this turn is this process's start: the harness runs the
	// hook as the turn ends. Its start is the latest UserPromptSubmit the
	// telemetry hook recorded; without one it is unknown, and a pending mark
	// cannot be tied to the turn.
	observation := registry.ObservationFor(dir, self.Name, self.Epoch())
	event.Ended = boottime.ProcessStarted
	event.Started = telemetry.ReadTurnStart(telemetry.TurnStartPath(observation))
	// The conversation the turn ended in, as the harness itself names it;
	// a tracker is asked only when the payload named none.
	currentThread := event.Thread
	if currentThread == "" {
		currentThread, _ = harness.SessionThread(self)
	}

	if reason, _ := endTurnContext(context.Background(), dir, self, event.turnResult, event.Holdable, currentThread); reason != "" {
		// Held: the turn goes on, and its end is still to come. Recording
		// this moment as a start would put a mark the continuation makes
		// outside the turn it belongs to.
		out := io.Writer(os.Stdout)
		if ctx != nil && ctx.Stdout != nil {
			out = ctx.Stdout
		}
		printHold(out, reason)
		return nil
	}
	// Whatever comes next starts after this end, so the recorded start moves
	// up to it. A turn end that was lost — an Esc, a payload or a lock that
	// never came — is then corrected by the next one heard: a mark made
	// before it cannot be taken by a later turn, even one that starts without
	// a UserPromptSubmit.
	telemetry.RecordTurnStart(observation, event.Ended)
	return nil
}

func completeTurn(dir string, self registry.Session, event turnResult, currentThread string) error {
	return completeTurnContext(context.Background(), dir, self, event, currentThread)
}

func completeTurnContext(parent context.Context, dir string, self registry.Session, event turnResult, currentThread string) error {
	_, err := endTurnContext(parent, dir, self, event, false, currentThread)
	return err
}

// endTurnContext publishes a turn end, or holds it for the session to confirm
// (turn_hold.go): then it publishes nothing and answers the reason to hand
// the model. holdable says the end may be held at all, which only the
// harness that reported it can say.
func endTurnContext(parent context.Context, dir string, self registry.Session, event turnResult, holdable bool, currentThread string) (string, error) {
	if !registry.OwnsName(dir, self.Name, self.Epoch()) {
		return "", nil
	}
	if event.ID != "" && event.Boundary == nil {
		// Its scope could come only from a record written at its first
		// attempt, and a retry after that write failed would fix the scope of
		// its own moment instead (docs/turn-end-recovery.md#the-operation).
		// Its waits stay owed for the next end.
		return "", errors.New("a turn end named by an event id carries no read boundary, so its scope is unknown and it reports nothing; its waits stay owed for the next turn end")
	}

	// Under the mailbox lock, so two ends of a turn reported at once tell each
	// waiter once, and a waiter recorded by a read in the meantime is not taken
	// for the one reported.
	ctx, cancel := context.WithTimeout(parent, hookLockWait)
	defer cancel()
	reason := ""
	err := state.WithMailboxLock(ctx, dir, self.Name, func() error {
		// Every record of the mailbox is reconciled first: a wait an
		// unfinished journal answered would be answered again, and a stopped
		// mailbox changes nothing (docs/turn-end-recovery.md#reconciliation).
		if err := inbox.Reconcile(ctx, dir, self.Name); err != nil {
			return err
		}
		op := turnOp(self, event)
		if event.ID != "" {
			// A retry whose journal is on record: the barrier has completed it.
			recorded, err := inbox.JournalRecorded(dir, self.Name, op)
			if err != nil || recorded {
				return err
			}
		}
		waiters, err := inbox.ScopedWaiters(dir, self.Name, self.Epoch(), event.Boundary)
		if err != nil {
			return err
		}
		mark, err := turnMark(dir, self, event)
		if err != nil {
			return err
		}
		if reason = holdTurn(dir, self, event, holdable, waiters, mark != nil); reason != "" {
			return nil
		}
		beforeReports()
		return publishTurnContext(ctx, dir, self, event, currentThread, waiters, op, mark)
	})
	return reason, err
}

// turnMark finds the pending mark that decides a turn end: this run's latest
// in the end's window (docs/turn-end-recovery.md#pending-marks). Only a
// finish is softened by one, and an end whose time is not known has no
// window. A mark that cannot be read may be this turn's, and stops the end.
func turnMark(dir string, self registry.Session, event turnResult) (*inbox.Mark, error) {
	if event.Failed || event.Stopped || event.Ended == 0 {
		return nil, nil
	}
	start, err := inbox.TurnWindowStart(dir, self.Name, self.Epoch(), event.Started, event.Ended)
	if err != nil {
		return nil, err
	}
	mark, ok, err := inbox.MarkWithin(dir, self.Name, self.Epoch(), start, event.Ended)
	if err != nil || !ok {
		return nil, err
	}
	return &mark, nil
}

// hookLockWait is how long the end of a turn waits for the mailbox. What it
// could not report stays owed until the next turn ends.
const hookLockWait = 5 * time.Second

// beforeReports runs between reading the waiters and reporting to them. It
// does nothing; a test widens that window through it.
var beforeReports = func() {}

// readPayload reads stdin, but not forever, and never from a person.
func readPayload(input *os.File) []byte {
	if isTerminal(input) {
		return nil
	}
	read := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(io.LimitReader(input, maxPayload))
		read <- data
	}()
	select {
	case data := <-read:
		return data
	case <-time.After(payloadWait):
		return nil
	}
}

// isTerminal reports whether a file is a character device, which is what an
// interactive stdin is. Reading from one would wait for a person.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

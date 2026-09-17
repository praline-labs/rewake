package cli

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/state"
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
func handleTurnEnded(_ *Context, call Call) error {
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
	currentThread, _ := harness.SessionThread(self)

	// Under the mailbox lock, so two ends of a turn reported at once tell each
	// waiter once, and a waiter recorded by a read in the meantime is not taken
	// for the one reported.
	ctx, cancel := context.WithTimeout(context.Background(), hookLockWait)
	defer cancel()
	_ = state.WithMailboxLock(ctx, dir, self.Name, func() error {
		greeting, err := inbox.GreetingPending(dir, self.Name, self.Epoch())
		if err != nil {
			return err
		}
		bootstrap := greeting && !event.Failed && strings.TrimSpace(event.Text) == "ready"
		if bootstrap {
			// Save before consuming the marker: retries must remain harmless even
			// when an early task is already owed or the marker removal fails.
			receipt, path, err := loadTurnReceipt(dir, self, event)
			if err != nil {
				return err
			}
			receipt.Done = true
			if err := saveTurnReceipt(path, receipt); err != nil {
				return err
			}
		}
		if _, err := inbox.TakeGreeting(dir, self.Name, self.Epoch()); err != nil {
			return err
		}
		if bootstrap {
			return nil
		}
		waiters := inbox.Waiters(dir, self.Name, self.Epoch())
		beforeReports()
		return publishTurn(dir, self, event, currentThread, waiters)
	})
	return nil
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

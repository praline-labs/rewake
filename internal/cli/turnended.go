package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// maxPayload bounds what is read from a hook. The payload is a small JSON
// object; anything much larger is not one.
const maxPayload = 4 << 20

// payloadWait bounds the wait for a payload on stdin. A hook gets its payload
// at once and the pipe closed; one that stays open with nothing on it would
// otherwise hold the end of the turn for as long as the harness allows.
const payloadWait = 3 * time.Second

// silentEnd is the text of a finished notice when the turn ended without a reply.
const silentEnd = "(the turn ended without a final message)"

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
	self, epoch, err := ownRun(dir)
	if err != nil {
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
	reply, ok := lastReply(payload)
	if !ok {
		return nil
	}
	if strings.TrimSpace(reply) == "" {
		reply = silentEnd
	}

	// Under the mailbox lock, so two ends of a turn reported at once tell each
	// waiter once, and a waiter recorded by a read in the meantime is not taken
	// for the one reported.
	_ = state.WithMailboxLock(dir, self.Name, func() error {
		waiters := inbox.Waiters(dir, self.Name, epoch)
		beforeReports()
		for _, waiter := range waiters {
			peer, err := registry.Lookup(dir, waiter.Name)
			if err != nil || peer.Epoch() != waiter.Epoch {
				// The run that wrote has ended, whether or not its name lives
				// on: nobody is left to tell.
				if err == nil || errors.Is(err, registry.ErrNotFound) {
					inbox.ClearAwaiting(dir, self.Name, epoch, waiter)
				}
				continue
			}
			err = inbox.Put(dir, inbox.Message{
				ID:        inbox.NewID(),
				From:      self.Name,
				FromEpoch: epoch,
				To:        peer.Name,
				ToEpoch:   waiter.Epoch,
				Kind:      inbox.Finished,
				Text:      reply,
				CreatedAt: time.Now(),
			})
			if err == nil {
				inbox.ClearAwaiting(dir, self.Name, epoch, waiter)
			}
		}
		return nil
	})
	return nil
}

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

// lastReply reads the last reply of the turn from a hook payload. The two
// harnesses name the field differently, and Codex also calls its program for
// events that are not the end of a turn.
func lastReply(payload []byte) (string, bool) {
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return "", false
	}
	if event, present := fields["type"]; present && event != "agent-turn-complete" {
		return "", false
	}
	for _, key := range []string{"last_assistant_message", "last-assistant-message"} {
		if text, ok := fields[key].(string); ok {
			return text, true
		}
	}
	return "", true
}

// isTerminal reports whether a file is a character device, which is what an
// interactive stdin is. Reading from one would wait for a person.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// inboxModel is the machine form of the messages a session fetched.
type inboxModel struct {
	Session  string          `json:"session"`
	Messages []inbox.Message `json:"messages"`
}

// handleInbox hands the agent its unread mail. A harness is only told that mail
// is waiting; the text comes from here, so the agent reads it as the output of
// a tool it ran rather than as something its user typed.
//
// The text is written out before anything is marked read. A message marked
// first and lost on the way — a full disk, a killed process — would be gone
// for good; shown twice, it is merely shown twice.
func handleInbox(ctx *Context, call Call) error {
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	session, epoch, err := ownRun(dir)
	switch {
	case errors.Is(err, errNotASession):
		return &UsageError{
			Command: call.Command,
			Message: "This shell is not part of a rewake session, so it has no inbox. Start an agent with rewake to give it one.",
		}
	case errors.Is(err, errEarlierRun):
		return failf("%v; its mail is not this shell's to read", err)
	case err != nil:
		return failf("%v; its mail cannot be read", err)
	}

	// Held from looking to marking. Two readers at once — parallel tool calls,
	// a command run again while the first still prints — would otherwise both
	// show the same task, and the server must not record a delivery over a read
	// that is half done.
	var failure error
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	err = state.WithMailboxLock(wait, dir, session.Name, func() error {
		messages, err := inbox.PeekUnread(dir, session.Name, epoch)
		if err != nil {
			failure = failf("could not read the inbox of %s: %v", session.Name, err)
			return nil
		}
		if err := writeInbox(ctx, inboxModel{Session: session.Name, Messages: messages}); err != nil {
			failure = failf("could not print the messages, so none were marked read: %v", err)
			return nil
		}
		for _, message := range messages {
			if err := inbox.MarkRead(dir, session.Name, epoch, message, !role.Of(session.Role).Silent); err != nil {
				failure = failf("the messages were shown, but recording that failed, so they stay unread and show again next time: %v", err)
				return nil
			}
		}
		return nil
	})
	if errors.Is(err, state.ErrMailboxBusy) {
		return failf("the inbox of %s is being read by another command right now; run rewake inbox again in a moment", session.Name)
	}
	if err != nil {
		return failf("could not lock the inbox of %s: %v", session.Name, err)
	}
	return failure
}

// readerLockWait is how long a reader waits for another reader, or for the
// server, to let go of the mailbox.
const readerLockWait = 10 * time.Second

// writeInbox prints the messages and reports whether the output got through.
func writeInbox(ctx *Context, model inboxModel) error {
	var text string
	if ctx.JSON {
		encoded, err := json.MarshalIndent(model, "", "  ")
		if err != nil {
			return err
		}
		text = string(encoded)
	} else {
		text = strings.Join(inboxLines(model.Messages), "\n")
	}
	_, err := io.WriteString(ctx.Stdout, text+"\n")
	return err
}

func inboxLines(messages []inbox.Message) []string {
	if len(messages) == 0 {
		return []string{"No new messages."}
	}
	var lines []string
	for index, message := range messages {
		if index > 0 {
			lines = append(lines, "")
		}
		lines = append(lines,
			fmt.Sprintf("from %s · %s · %s", message.From, inbox.KindOf(message), message.CreatedAt.Local().Format("15:04:05")),
			message.Text,
		)
	}
	return lines
}

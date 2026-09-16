package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
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
func handleInbox(ctx *Context, call Call) error {
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	name := os.Getenv(state.SessionEnv)
	if name == "" {
		return &UsageError{
			Command: call.Command,
			Message: "This shell is not part of a rewake session, so it has no inbox. Start an agent with rewake to give it one.",
		}
	}

	// The epoch keeps a session to its own mail: the name may have been used by
	// a session before this one, and what was left for that one is not ours.
	session, err := registry.Lookup(dir, name)
	if errors.Is(err, registry.ErrNotFound) {
		return failf("the session %s is not registered any more; its mail cannot be read", name)
	}
	if err != nil {
		return failf("could not read the record of %s: %v", name, err)
	}

	messages, err := inbox.TakeUnread(dir, name, session.Epoch())
	if err != nil && len(messages) == 0 {
		return failf("could not read the inbox of %s: %v", name, err)
	}

	return printValue(ctx, inboxModel{Session: name, Messages: messages}, func() []string {
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
	})
}

package cli

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

// inboxMode is what a call of rewake inbox asked for: all unread mail, an
// overview of it, one message of it, what was read and is still owed, or what
// was sent and is still awaited.
type inboxMode struct {
	peek, owed, awaited bool
	selected            string
}

func inboxSelection(call Call) (inboxMode, error) {
	value, peek := call.Flags["peek"]
	owedValue, owed := call.Flags["owed"]
	id, selected := call.Flags["message"]
	awaitedValue, awaited := call.Flags["awaited"]
	if peek && selected {
		return inboxMode{}, &UsageError{Command: call.Command, Message: "--peek and --message are mutually exclusive; choose an overview or one full message."}
	}
	if owed && (peek || selected) {
		return inboxMode{}, &UsageError{Command: call.Command, Message: "--owed is used alone: it shows what was already read, while --peek and --message look at unread mail."}
	}
	if awaited && (peek || selected || owed) {
		return inboxMode{}, &UsageError{Command: call.Command, Message: "--awaited is used alone: it shows what you sent, while the other flags look at mail you received."}
	}
	if awaited && awaitedValue != "true" {
		return inboxMode{}, &UsageError{Command: call.Command, Message: "--awaited is a switch and takes no value."}
	}
	if peek && value != "true" {
		return inboxMode{}, &UsageError{Command: call.Command, Message: "--peek is a switch and takes no value."}
	}
	if owed && owedValue != "true" {
		return inboxMode{}, &UsageError{Command: call.Command, Message: "--owed is a switch and takes no value."}
	}
	if selected && (id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\") || strings.ContainsFunc(id, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })) {
		return inboxMode{}, &UsageError{Command: call.Command, Message: "--message needs one opaque ID from rewake inbox --peek."}
	}
	return inboxMode{peek: peek, owed: owed, awaited: awaited, selected: id}, nil
}

// No embedded Message: overview JSON must never acquire bodies or private fields.
type messagePreview struct {
	ID        string                 `json:"id"`
	From      string                 `json:"from"`
	Kind      inbox.Kind             `json:"kind"`
	CreatedAt time.Time              `json:"createdAt"`
	Preview   string                 `json:"preview"`
	Telemetry *sessionstate.Snapshot `json:"telemetry,omitempty"`
}

type inboxPeekModel struct {
	Session  string           `json:"session"`
	Messages []messagePreview `json:"messages"`
}

func writeInboxPeek(ctx *Context, name string, messages []messageView) error {
	model := inboxPeekModel{Session: name, Messages: make([]messagePreview, 0, len(messages))}
	for _, message := range messages {
		model.Messages = append(model.Messages, messagePreview{ID: message.ID, From: message.From, Kind: inbox.KindOf(message.Message), CreatedAt: message.CreatedAt, Preview: harness.Preview(message.Text), Telemetry: message.Telemetry})
	}
	return printValue(ctx, model, func() []string {
		if len(model.Messages) == 0 {
			return []string{"Rewake: no new messages."}
		}
		noun := "messages"
		if len(model.Messages) == 1 {
			noun = "message"
		}
		lines := []string{fmt.Sprintf("Rewake: %d unread %s:", len(model.Messages), noun)}
		for _, message := range model.Messages {
			if message.Telemetry != nil {
				lines = append(lines, stateLine(message.From, message.Telemetry, false))
			}
			lines = append(lines, fmt.Sprintf("%s · %s · %s · %s · %s", message.ID, message.From, message.Kind, message.CreatedAt.Local().Format("2006-01-02 15:04:05"), message.Preview))
		}
		return lines
	})
}

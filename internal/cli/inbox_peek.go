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

func inboxSelection(call Call) (bool, string, error) {
	value, peek := call.Flags["peek"]
	id, selected := call.Flags["message"]
	if peek && selected {
		return false, "", &UsageError{Command: call.Command, Message: "--peek and --message are mutually exclusive; choose an overview or one full message."}
	}
	if peek && value != "true" {
		return false, "", &UsageError{Command: call.Command, Message: "--peek is a switch and takes no value."}
	}
	if selected && (id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\") || strings.ContainsFunc(id, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })) {
		return false, "", &UsageError{Command: call.Command, Message: "--message needs one opaque ID from rewake inbox --peek."}
	}
	return peek, id, nil
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
			return []string{"No new messages."}
		}
		lines := []string{fmt.Sprintf("%d unread messages (overview; none marked read):", len(model.Messages))}
		for _, message := range model.Messages {
			if message.Telemetry != nil {
				lines = append(lines, stateLine(message.From, message.Telemetry, false))
			}
			lines = append(lines, fmt.Sprintf("%s · %s · %s · %s · %s", message.ID, message.From, message.Kind, message.CreatedAt.Local().Format("2006-01-02 15:04:05"), message.Preview))
		}
		return append(lines, "Read one: rewake inbox --message <id>; read all: rewake inbox")
	})
}

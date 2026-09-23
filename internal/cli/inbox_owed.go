package cli

import (
	"fmt"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

// owedView is one message read and still owed a report. Kept is false when the
// mailbox no longer has its text; then only its id and sender are known.
type owedView struct {
	messageView
	Kept bool `json:"kept"`
}

type owedModel struct {
	Session  string     `json:"session"`
	Messages []owedView `json:"messages"`
}

// showOwed prints again, in full, what this run has read and not yet reported
// on — the task a session is working on, for one that lost it to a compaction.
// It changes nothing: a second read of a task is not a second obligation, and
// the report still goes when the turn ends.
func showOwed(ctx *Context, call Call, dir string, session registry.Session, epoch string) error {
	if role.Of(session.Role).Silent {
		return &UsageError{
			Command: call.Command,
			Message: fmt.Sprintf("%s owes no reports: a %s session's reads record no obligation, so --owed has nothing to show. rewake inbox --awaited shows what others owe it.", session.Name, session.Role),
		}
	}
	owed := inbox.OwedMessages(dir, session.Name, epoch)
	messages := make([]inbox.Message, 0, len(owed))
	for _, message := range owed {
		messages = append(messages, message.Message)
	}
	model := owedModel{Session: session.Name, Messages: make([]owedView, 0, len(owed))}
	for index, view := range viewedMessages(dir, messages) {
		model.Messages = append(model.Messages, owedView{messageView: view, Kept: owed[index].Kept})
	}
	return printValue(ctx, model, func() []string { return owedLines(model.Messages) })
}

func owedLines(messages []owedView) []string {
	if len(messages) == 0 {
		return []string{"Rewake: nothing owed a report."}
	}
	noun := "messages"
	if len(messages) == 1 {
		noun = "message"
	}
	lines := []string{fmt.Sprintf("Rewake: owed a report for %d %s:", len(messages), noun)}
	for _, message := range messages {
		lines = append(lines, "")
		if !message.Kept {
			lines = append(lines, fmt.Sprintf("from %s · %s · text no longer kept", message.From, message.ID))
			continue
		}
		if message.Telemetry != nil {
			lines = append(lines, stateLine(message.From, message.Telemetry, message.Availability != nil || message.Departure != nil), "")
		}
		lines = append(lines,
			fmt.Sprintf("from %s · %s · %s", message.From, inbox.KindOf(message.Message), message.CreatedAt.Local().Format("15:04:05")),
			message.Text,
		)
		if message.ThreadChanged {
			lines = append(lines, inbox.ThreadChangedWarning)
		}
	}
	return lines
}

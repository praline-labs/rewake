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
	// Unread counts the tasks and questions waiting unread, whoever sent
	// them: it says work is waiting, not that a report is. --owed shows only
	// what was read, and a session that asks it after a compaction must not
	// take "nothing owed" for "no work": a Codex worker skipped a new task
	// twice that way.
	Unread int `json:"unread"`
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
	// Unread first, then owed: a read by a parallel rewake inbox writes the
	// wait record before it moves the message out of unread/, so a message
	// read between the two looks lands in both lists, never in neither. The
	// other order could miss it twice and answer "nothing owed" with nothing
	// unread — the trap the count is there for. One in both is counted once,
	// as owed.
	unread, err := inbox.PeekUnread(dir, session.Name, epoch)
	if err != nil {
		return failf("could not look at the unread mail of %s: %v; run rewake inbox", session.Name, err)
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
	listed := make(map[string]bool, len(owed))
	for _, message := range owed {
		listed[message.ID] = true
	}
	for _, message := range unread {
		if inbox.AsksForWork(message) && !listed[message.ID] {
			model.Unread++
		}
	}
	return printValue(ctx, model, func() []string { return append(owedLines(model.Messages), unreadHint(model.Unread)...) })
}

// unreadHint is the line after what is owed that names the tasks and
// questions still waiting unread, and nothing when there are none.
func unreadHint(count int) []string {
	switch count {
	case 0:
		return nil
	case 1:
		return []string{"", "Rewake: 1 unread task or question — run rewake inbox."}
	}
	return []string{"", fmt.Sprintf("Rewake: %d unread tasks or questions — run rewake inbox.", count)}
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

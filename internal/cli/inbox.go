package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// inboxModel is the machine form of the messages a session fetched.
type inboxModel struct {
	Session  string        `json:"session"`
	Messages []messageView `json:"messages"`
}

// handleInbox hands the agent its unread mail. A harness is only told that mail
// is waiting; the text comes from here, so the agent reads it as the output of
// a tool it ran rather than as something its user typed.
//
// The text is written out before anything is marked read. A message marked
// first and lost on the way — a full disk, a killed process — would be gone
// for good; shown twice, it is merely shown twice.
func handleInbox(ctx *Context, call Call) error {
	mode, err := inboxSelection(call)
	if err != nil {
		return err
	}
	peek, selected := mode.peek, mode.selected
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	session, box, err := ownMail(ctx.access, dir)
	switch {
	case errors.Is(err, errNotASession):
		return &UsageError{
			Command: call.Command,
			Message: "This shell is not part of a rewake session, so it has no inbox.",
		}
	case errors.Is(err, errEarlierRun):
		return failf("%v; its mail is not this shell's to read", err)
	case err != nil:
		return failf("%v; its mail cannot be read", err)
	}
	if mode.awaited {
		return showAwaited(ctx, dir, box)
	}
	if mode.owed {
		return showOwed(ctx, call, dir, session, box)
	}
	request := mail.ReadRequest{Selected: selected, Peek: peek, Reports: mail.OwesReports(session.Role)}
	_, err = box.Read(context.Background(), request, func(messages []mail.Message) error {
		views := fromEarlierRuns(viewedMessages(dir, box, messages), box.Epoch())
		if peek {
			return writeInboxPeek(ctx, session.Name, views)
		}
		return writeInbox(ctx, inboxModel{Session: session.Name, Messages: withBriefs(dir, views, session.Name, box.Epoch())})
	})
	return readFailure(err, session.Name, selected, peek)
}

// readFailure is what a read that stopped tells its caller, by the step it
// stopped at.
func readFailure(err error, name, selected string, peek bool) error {
	var step *mail.StepError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, state.ErrMailboxBusy):
		return failf("the inbox of %s is being read by another command right now; run rewake inbox again in a moment", name)
	case !errors.As(err, &step) || step.Step == mail.StepLock:
		return failf("could not lock the inbox of %s: %v", name, err)
	case step.Step == mail.StepSelect:
		return failf("message %q is not available in this session's unread inbox (unknown, not yet announced, reserved or already read); run rewake inbox --peek", selected)
	case step.Step == mail.StepShow && peek:
		return failf("could not print the inbox overview: %v", err)
	case step.Step == mail.StepShow:
		return failf("could not print the messages, so none were marked read: %v", err)
	case step.Step == mail.StepMark:
		return failf("the messages were shown, but recording that failed, so they stay unread and show again next time: %v", err)
	}
	return failf("could not read the inbox of %s: %v", name, err)
}

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

func inboxLines(messages []messageView) []string {
	if len(messages) == 0 {
		return []string{"Rewake: no new messages."}
	}
	var lines []string
	for index, message := range messages {
		if index > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, letterHead(message)...)
		lines = append(lines, letterBody(message))
		lines = append(lines, briefLetterLines(message.BriefView)...)
	}
	return lines
}

// fromEarlierRuns marks the letters written for an earlier run of the name.
func fromEarlierRuns(views []messageView, epoch string) []messageView {
	for index := range views {
		views[index].EarlierRun = views[index].ToEpoch != epoch
	}
	return views
}

// earlierRunNote follows a task or a question written for an earlier run.
const earlierRunNote = "Rewake: written for an earlier run of this session, which never read it; no report is owed, and its sender counts it lost."

// letterHead is what a letter shows above its text: the sender's state for a
// main, then who sent what when.
func letterHead(message messageView) []string {
	var lines []string
	if message.Telemetry != nil {
		lines = append(lines, stateLine(message.From, message.Telemetry, message.Availability != nil || message.Departure != nil), "")
	}
	lines = append(lines,
		fmt.Sprintf("from %s · %s · %s%s%s", message.From, mail.KindOf(message.Message), message.CreatedAt.Local().Format("15:04:05"), relation(message.Message), earlierRun(message.EarlierRun)),
	)
	return lines
}

// letterBody is the letter's text, with the warning a report from another
// conversation carries, or the note on work an earlier run never read.
func letterBody(message messageView) string {
	text := message.Text
	if message.ThreadChanged {
		text += "\n" + mail.ThreadChangedWarning
	}
	if message.EarlierRun && mail.AsksForWork(message.Message) {
		text += "\n" + earlierRunNote
	}
	return text
}

// earlierRun is the heading's mark of a letter written for an earlier run.
func earlierRun(earlier bool) string {
	if earlier {
		return " · for an earlier run"
	}
	return ""
}

// relation names the message a letter belongs with, for the heading line:
// the task an addendum adds to, the letter an edit replaced.
func relation(message mail.Message) string {
	var parts []string
	if message.AddendumTo != "" {
		parts = append(parts, " · addendum to "+message.AddendumTo)
	}
	if message.Replaces != "" {
		parts = append(parts, " · replaces "+message.Replaces)
	}
	return strings.Join(parts, "")
}

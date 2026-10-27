package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

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
	session, epoch, err := ownRunIn(ctx.access, dir)
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
		return showAwaited(ctx, dir, session, epoch)
	}
	if mode.owed {
		return showOwed(ctx, call, dir, session, epoch)
	}
	// Held from looking to marking. Two readers at once — parallel tool calls,
	// a command run again while the first still prints — would otherwise both
	// show the same task, and the server must not record a delivery over a read
	// that is half done.
	var failure error
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	err = state.WithMailboxLock(wait, dir, session.Name, func() error {
		// Every run's unread mail, not only this run's: an earlier run of the
		// name that ended before reading what was announced to it leaves it in
		// unread/, and nobody else will ever read it.
		messages, err := mail.Open(dir, session.Name, "").AvailableUnread()
		if err != nil {
			failure = failf("could not read the inbox of %s: %v", session.Name, err)
			return nil
		}
		if selected != "" {
			var found bool
			for _, message := range messages {
				if message.ID == selected {
					messages, found = []mail.Message{message}, true
					break
				}
			}
			if !found {
				failure = failf("message %q is not available in this session's unread inbox (unknown, not yet announced, reserved or already read); run rewake inbox --peek", selected)
				return nil
			}
		}
		views := fromEarlierRuns(viewedMessages(dir, messages), epoch)
		if peek {
			if err := writeInboxPeek(ctx, session.Name, views); err != nil {
				failure = failf("could not print the inbox overview: %v", err)
				return nil
			}
			return nil
		}
		if err := writeInbox(ctx, inboxModel{Session: session.Name, Messages: withBriefs(dir, views, session.Name, epoch)}); err != nil {
			failure = failf("could not print the messages, so none were marked read: %v", err)
			return nil
		}
		for _, message := range messages {
			// A task an earlier run never read is lost to its sender, who may
			// have sent it again: reading it now owes nothing.
			reports := mail.OwesReports(session.Role) && message.ToEpoch == epoch
			if err := mail.Open(dir, session.Name, epoch).MarkRead(message, reports); err != nil {
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

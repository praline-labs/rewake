package claude

import (
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// notification wraps the notice in the tag Claude Code uses for its own
// background events. The interface picks how to draw a user message from its
// text, and this one it draws as a single "● <summary>" line — the same line a
// finished background task gets — instead of a block of pasted text.
//
// Only the summary and the status are shown, and nothing else goes in: the
// person watching the session should see exactly what the agent is told.
func notification(message mail.Message) string {
	status := "completed"
	if mail.NoticeKind(message) == mail.Stopped {
		status = "killed"
	}
	if mail.NoticeKind(message) == mail.Error {
		status = "failed"
	}
	if mail.NoticeKind(message) == mail.Interim {
		// Neither completed nor failed: the interface draws a status it has no
		// color for as a plain line, which is what "still going" should look like.
		status = "running"
	}
	summary := mail.Notice(message)
	return strings.Join([]string{
		"<task-notification>",
		fmt.Sprintf("<task-id>%s</task-id>", noticeID(message)),
		"<status>" + status + "</status>",
		fmt.Sprintf("<summary>%s</summary>", escape(summary)),
		"</task-notification>",
	}, "\n")
}

// escape keeps a sender name from closing the tag early. Names are already
// restricted to a safe alphabet; this is for the day that changes.
func escape(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

// noticeID is the part of the message id a notice carries. Claude Code drops
// identical text from the same sender within thirty seconds, and two notices of
// the same kind from the same session would otherwise be the same text. Eight
// hex characters of the random tail, not four: the id only has to make
// otherwise identical messages different, and a four-character tail repeats
// often enough to matter.
func noticeID(message mail.Message) string {
	id := message.ID
	if len(id) > 8 {
		id = id[len(id)-8:]
	}
	return "rewake-" + id
}

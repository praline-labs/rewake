package claude

import (
	"fmt"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// notification wraps the notice in the tag Claude Code uses for its own
// background events. The interface picks how to draw a user message from its
// text, and this one it draws as a single "● <summary>" line — the same line a
// finished background task gets — instead of a block of pasted text.
//
// Only the summary and the status are shown, and nothing else goes in: the
// person watching the session should see exactly what the agent is told.
func notification(message inbox.Message) string {
	status := "completed"
	if harness.NoticeKind(message) == inbox.Error {
		status = "failed"
	}
	return strings.Join([]string{
		"<task-notification>",
		fmt.Sprintf("<task-id>%s</task-id>", harness.NoticeID(message)),
		"<status>" + status + "</status>",
		fmt.Sprintf("<summary>%s</summary>", escape(harness.Notice(message))),
		"</task-notification>",
	}, "\n")
}

// escape keeps a sender name from closing the tag early. Names are already
// restricted to a safe alphabet; this is for the day that changes.
func escape(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

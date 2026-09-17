package harness

import (
	"fmt"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// Notice announces unread mail and shows the author's bounded first line.
// The full message stays in inbox; retries retain their original notice id.
func Notice(message inbox.Message) string {
	count := message.Unread
	if count < 1 {
		count = 1
	}
	noun := "messages"
	if count == 1 {
		noun = "message"
	}
	shown := message
	if message.Latest != nil {
		shown = *message.Latest
	}
	line := fmt.Sprintf("Rewake: %s %s, %d new %s", shown.From, inbox.KindOf(shown), count, noun)
	if first := preview(shown.Text); first != "" {
		line += "\n  ↳ " + first
	}
	return line
}

// NoticeID is the part of the message id a notice carries. Claude Code drops
// identical text from the same sender within thirty seconds, and two notices of
// the same kind from the same session would otherwise be the same text.
func NoticeID(message inbox.Message) string {
	return "rewake-" + shortID(message.ID)
}

// ShellSender is the sender name used when a message comes from a shell that is
// not a rewake session.
const ShellSender = "shell"

// shortID is the part of the id the receiver sees. Eight hex characters of the
// random tail, not four: the id only has to make otherwise identical messages
// different, and a four-character tail repeats often enough to matter.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[len(id)-8:]
}

// NoticeKind follows the displayed latest letter, including its failure color.
func NoticeKind(message inbox.Message) inbox.Kind {
	if message.Latest != nil {
		return inbox.KindOf(*message.Latest)
	}
	return inbox.KindOf(message)
}

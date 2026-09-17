package harness

import (
	"fmt"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// Notice is the one line that announces waiting mail, the same for every
// harness: "Rewake: codex finished, 1 new message". It carries no text of the
// message on purpose. The agent fetches that itself, so it knows the message
// came through a tool, not from the person at the keyboard.
func Notice(message inbox.Message) string {
	count := message.Unread
	if count < 1 {
		count = 1
	}
	noun := "messages"
	if count == 1 {
		noun = "message"
	}
	return fmt.Sprintf("Rewake: %s %s, %d new %s", message.From, inbox.KindOf(message), count, noun)
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

package harness

import (
	"fmt"
	"sort"

	"github.com/praline-labs/rewake/internal/inbox"
)

// Notice announces unread mail and shows the author's bounded first line.
// The full message stays in inbox; retries retain their original notice id.
//
// A correcting letter — a recall, telling the recipient not to act on a notice
// its sender withdrew, or a replacement sent by rewake edit — gets a line of
// its own, first, whatever else arrives with it: the notice otherwise shows one
// letter, and an agent that acts from the preview alone would never see a
// correction hidden behind a neighbor.
func Notice(message inbox.Message) string {
	if len(message.Batch) > 1 {
		var corrections []inbox.Message
		var latest *inbox.Message
		for i, member := range message.Batch {
			if correcting(member) {
				corrections = append(corrections, member)
				continue
			}
			if latest == nil || member.CreatedAt.After(latest.CreatedAt) || member.CreatedAt.Equal(latest.CreatedAt) && member.ID > latest.ID {
				latest = &message.Batch[i]
			}
		}
		line := fmt.Sprintf("Rewake: %d new messages", len(message.Batch))
		line += correctionLines(corrections)
		if latest != nil {
			line += "\n  ↳ " + preview(fmt.Sprintf("%s %s: %s", latest.From, inbox.KindOf(*latest), firstLine(*latest)))
		}
		return line
	}
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
	if correcting(message) && shown.ID != message.ID {
		line += correctionLines([]inbox.Message{message})
	}
	if first := preview(firstLine(shown)); first != "" {
		line += "\n  ↳ " + first
	}
	return line
}

// correcting reports whether a letter changes what the recipient should do
// about one it was shown before.
func correcting(message inbox.Message) bool {
	return message.Recall != nil || message.Replaces != ""
}

// correctionLines puts each correcting letter on a line of its own, oldest
// first. A recall's text leads with the instruction, so the bound on a line
// cuts the sender and the time, never what not to act on; a replacement's line
// is the one it would have as the newest letter, naming what it replaces.
func correctionLines(corrections []inbox.Message) string {
	sort.SliceStable(corrections, func(i, j int) bool {
		a, b := corrections[i], corrections[j]
		return a.CreatedAt.Before(b.CreatedAt) || a.CreatedAt.Equal(b.CreatedAt) && a.ID < b.ID
	})
	lines := ""
	for _, correction := range corrections {
		if correction.Recall != nil {
			lines += "\n  ↳ " + preview(correction.Text)
			continue
		}
		lines += "\n  ↳ " + preview(fmt.Sprintf("%s %s: %s", correction.From, inbox.KindOf(correction), firstLine(correction)))
	}
	return lines
}

// firstLine is what a letter's preview is made from. A replacement sent by
// rewake edit says first what it replaces: the old notice may be on screen,
// and the new one has to show the new work, not just more of the same.
func firstLine(message inbox.Message) string {
	if message.Replaces == "" {
		return message.Text
	}
	return fmt.Sprintf("Replaces %s (withdrawn): %s", inbox.ShortID(message.Replaces), message.Text)
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
	if len(message.Batch) > 1 {
		return inbox.Note
	}
	if message.Latest != nil {
		return inbox.KindOf(*message.Latest)
	}
	return inbox.KindOf(message)
}

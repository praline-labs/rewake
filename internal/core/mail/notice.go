package mail

import (
	"fmt"
	"sort"
)

// Notice announces unread mail and shows the author's bounded first line.
// The full message stays in inbox; retries retain their original notice id.
//
// A correcting letter — a recall, telling the recipient not to act on a notice
// its sender withdrew, or a replacement sent by rewake edit — gets a line of
// its own, first, whatever else arrives with it: the notice otherwise shows one
// letter, and an agent that acts from the preview alone would never see a
// correction hidden behind a neighbor.
func Notice(message Message) string {
	if len(message.Batch) > 1 {
		var corrections []Message
		var latest *Message
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
			line += "\n  ↳ " + shownPreview(fmt.Sprintf("%s %s: %s", latest.From, KindOf(*latest), firstLine(*latest)), KindOf(*latest))
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
	line := fmt.Sprintf("Rewake: %s %s, %d new %s", shown.From, KindOf(shown), count, noun)
	if correcting(message) && shown.ID != message.ID {
		line += correctionLines([]Message{message})
	}
	if first := shownPreview(firstLine(shown), KindOf(shown)); first != "" {
		line += "\n  ↳ " + first
	}
	return line
}

// previewOnly ends the preview of a letter that asks for work. A short task's
// first line is the whole task, and an agent shown it acted on it and never
// took the letter, so the task stayed unread and its report went to nobody
// (the owner, October 16, 2026; docs/delivery.md#the-notice).
const previewOnly = " (a preview only: run rewake inbox)"

// shownPreview is the preview line of the letter a notice shows. A task's or
// a question's ends with previewOnly, which the bound never cuts: the
// author's text is cut first.
func shownPreview(text string, kind Kind) string {
	if kind != Task && kind != Question {
		return preview(text)
	}
	first := previewWithin(text, previewCells-textCells(previewOnly))
	if first == "" {
		return ""
	}
	return first + previewOnly
}

// correcting reports whether a letter changes what the recipient should do
// about one it was shown before.
func correcting(message Message) bool {
	return message.Recall != nil || message.Replaces != ""
}

// correctionLines puts each correcting letter on a line of its own, oldest
// first. A recall's text leads with the instruction, so the bound on a line
// cuts the sender and the time, never what not to act on; a replacement's line
// is the one it would have as the newest letter, naming what it replaces.
func correctionLines(corrections []Message) string {
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
		lines += "\n  ↳ " + shownPreview(fmt.Sprintf("%s %s: %s", correction.From, KindOf(correction), firstLine(correction)), KindOf(correction))
	}
	return lines
}

// firstLine is what a letter's preview is made from. A replacement sent by
// rewake edit says first what it replaces: the old notice may be on screen,
// and the new one has to show the new work, not just more of the same.
func firstLine(message Message) string {
	if message.Replaces == "" {
		return message.Text
	}
	return fmt.Sprintf("Replaces %s (withdrawn): %s", ShortID(message.Replaces), message.Text)
}

// ShellSender is the sender name used when a message comes from a shell that is
// not a rewake session.
const ShellSender = "shell"

// NoticeKind follows the displayed latest letter, including its failure color.
func NoticeKind(message Message) Kind {
	if len(message.Batch) > 1 {
		return Note
	}
	if message.Latest != nil {
		return KindOf(*message.Latest)
	}
	return KindOf(message)
}

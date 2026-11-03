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
	if message.Reminder {
		return reminderNotice(message)
	}
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
			line += "\n  ↳ " + shownPreview(fmt.Sprintf("%s %s: %s", latest.From, KindOf(*latest), firstLine(*latest)), KindOf(*latest), latest.briefNote)
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
	line := fmt.Sprintf("Rewake: %s %s, %d new %s", message.From, KindOf(message), count, noun)
	if first := shownPreview(firstLine(message), KindOf(message), message.briefNote); first != "" {
		line += "\n  ↳ " + first
	}
	return line
}

// previewOnly ends the preview of a letter that asks for work. A short task's
// first line is the whole task, and an agent shown it acted on it and never
// took the letter, so the task stayed unread and its report went to nobody
// (the owner, October 16, 2026; docs/delivery.md#the-notice).
const previewOnly = " (a preview only: run rewake inbox)"

// maxBriefSuffix bounds the brief note's cells, a bit under half the line, so
// the author's text always keeps its first words; a longer note is left off.
const maxBriefSuffix = 44

// shownPreview is the preview line of the letter a notice shows. A task's or
// a question's ends with previewOnly, which the bound never cuts: the
// author's text is cut first.
//
// A report on a brief's task ends with the brief's note in brackets, bounded
// the same way: an open item shows without reading (docs/delivery.md#the-notice).
func shownPreview(text string, kind Kind, briefNote string) string {
	if suffix := " (" + briefNote + ")"; briefNote != "" && IsReportKind(kind) && textCells(suffix) <= maxBriefSuffix {
		first := previewWithin(text, previewCells-textCells(suffix))
		if first == "" {
			return ""
		}
		return first + suffix
	}
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
		lines += "\n  ↳ " + shownPreview(fmt.Sprintf("%s %s: %s", correction.From, KindOf(correction), firstLine(correction)), KindOf(correction), "")
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

// NoticeKind is the kind of the letter a notice shows, including its failure
// color; a group's is a note.
func NoticeKind(message Message) Kind {
	if len(message.Batch) > 1 {
		return Note
	}
	return KindOf(message)
}

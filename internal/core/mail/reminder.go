package mail

import "fmt"

// A worker once ended a turn while a task announced to it in that turn stayed
// unread, and nothing woke it again: what was announced is never announced
// again. Main found the task by --awaited and nudged the worker by hand
// (the owner, October 27, 2026; docs/delivery.md#the-notice). So a finished
// turn that leaves such letters unread gets one reminder. The window decides
// "once" with nothing kept: a letter is reminded at the end of the turn it was
// announced in, and the reminded turn's window opens after that end.

// Unattended answers the letters asking for work that this run still has
// unread after its finished turn end at ended (boot clock), of those announced
// inside that turn's window: after the run's previous end on record, up to
// ended. announced answers when a letter's notice was handed to the harness,
// on the boot clock; the delivery server keeps that for its run only.
//
// The window is TurnWindowStart's without the turn's own start: a task that
// woke an idle session was announced before its turn began, and is that
// turn's all the same. An end the interim record shows was interim, or that a
// later end of the run has superseded, reminds of nothing: a pending end says
// the work goes on, and the later end has its own window.
func (b Box) Unattended(ended int64, announced func(id string) (int64, bool)) ([]Message, error) {
	if ended == 0 {
		return nil, nil
	}
	record, err := readInterim(b.dir, b.name)
	if err != nil {
		return nil, err
	}
	if record != nil && record.Epoch == b.epoch && (record.Ended > ended || record.Ended == ended && !record.Settled) {
		return nil, nil
	}
	start, err := turnWindowStart(b.dir, b.name, b.epoch, 0, ended)
	if err != nil {
		return nil, err
	}
	unread, err := peekUnread(b.dir, b.name, b.epoch)
	if err != nil {
		return nil, err
	}
	var left []Message
	for _, message := range unread {
		if !AsksForWork(message) || message.Withdrawn != nil {
			continue
		}
		if at, ok := announced(message.ID); ok && at > start && at <= ended {
			left = append(left, message)
		}
	}
	return left, nil
}

// reminderNotice is the notice of a reminder: how many letters asking for work
// are still unread, and the latest one's preview, which says it is only one.
func reminderNotice(message Message) string {
	members := message.Batch
	if len(members) == 0 {
		members = []Message{message}
	}
	latest := members[0]
	for _, member := range members[1:] {
		if member.CreatedAt.After(latest.CreatedAt) || member.CreatedAt.Equal(latest.CreatedAt) && member.ID > latest.ID {
			latest = member
		}
	}
	noun := "task"
	if len(members) > 1 {
		noun = "tasks"
	}
	kind := KindOf(latest)
	return fmt.Sprintf("Rewake: %d unread %s still waiting\n  ↳ %s", len(members), noun,
		shownPreview(fmt.Sprintf("%s %s: %s", latest.From, kind, firstLine(latest)), kind))
}

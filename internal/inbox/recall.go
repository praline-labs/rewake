package inbox

import (
	"fmt"
	"strings"
	"time"
)

// RecallNotice marks the note that follows a withdrawal whose notice may have
// gone out. The tombstone answers the agent that reads its inbox; this answers
// the one that does not: a preview can itself be an instruction, and an agent
// seen on September 26, 2026 carried one out from the notice alone and never
// read the tombstone. So the note is announced like work, at once, and every
// notice shows it on a line of its own (harness.Notice). It stops work that
// has not happened yet; a step already taken from the preview stays taken.
//
// rewake edit sends none: its replacement's own preview names the message it
// replaces, so one line both sets the old work aside and shows the new.
type RecallNotice struct {
	// ID is the message withdrawn, and Kind what it was.
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
}

// ShortID is the part of an id a person copies: the random tail after the
// dash, which tells the messages of one run apart.
func ShortID(id string) string {
	if _, tail, ok := strings.Cut(id, "-"); ok && tail != "" {
		return tail
	}
	return id
}

// Recall tells the recipient of a withdrawn message not to act on its notice,
// in a note from the message's own sender and run. The instruction comes
// first: a notice line is cut at its end, and what must survive is which
// message not to act on.
func Recall(dir string, message Message) (Message, error) {
	kind := KindOf(message)
	if message.Withdrawn != nil {
		kind = message.Withdrawn.Kind
	}
	at := message.CreatedAt.Local().Format("15:04:05")
	note := Message{
		ID: NewID(), From: message.From, FromEpoch: message.FromEpoch, To: message.To, ToEpoch: message.ToEpoch,
		Kind: Note, CreatedAt: time.Now(),
		Text:   fmt.Sprintf("Do not act on %s %s from %s (%s): withdrawn unread.", kind, ShortID(message.ID), message.From, at),
		Recall: &RecallNotice{ID: message.ID, Kind: kind},
	}
	return note, Put(dir, note)
}

package workflow

// The heads-ups batch-arrival sends after its third letter, and what the case
// reads about them: the coalescing window's part of the scenario.

import (
	"fmt"
	"strings"
	"testing"
)

// The heads-ups, noteSpacing apart after the third letter.
var batchNotes = []string{"batch-arrival-note-one", "batch-arrival-note-two", "batch-arrival-note-three"}

// awaitNotes waits for the recipient's overviews to list every heads-up, and
// answers them as the recipient saw them.
func awaitNotes(c *Case, worker *scenarioSession) []peekedMessage {
	var notes []peekedMessage
	c.Await("the recipient to overview the heads-ups", func() bool {
		notes = notes[:0]
		for _, text := range batchNotes {
			note, seen := worker.peekedCarrying(text)
			if !seen {
				return false
			}
			notes = append(notes, note)
		}
		return true
	})
	return notes
}

// notesAlone answers whether a delivery announced a heads-up by itself, and
// what the deliveries carrying them were. Asked this way rather than "one
// delivery carried all three" because what else rides along is not the
// question: a letter that cannot wait takes waiting heads-ups with it.
func notesAlone(c *Case, col column, worker *scenarioSession, notes []peekedMessage) (bool, string) {
	var carried []string
	for _, note := range notes {
		delivery := deliveryContaining(c, col, worker, note.ID)
		if len(delivery.Members) < 2 {
			return true, fmt.Sprintf("%s was announced alone in %s", note.Preview, delivery.Turn)
		}
		carried = append(carried, fmt.Sprintf("%s in a notice of %d", note.ID, len(delivery.Members)))
	}
	return false, strings.Join(carried, "; ")
}

// noteSends logs what each heads-up's own send said. Not an observation: a
// breakage elsewhere — the widened collection — slows a delivery past the
// send's wait, and that says nothing about grouping; but a run read with -v
// should show whether the sends answered delivered.
func noteSends(t *testing.T, c *Case, sender *scenarioSession) {
	c.Await("the sender to record every heads-up", func() bool {
		records, err := sender.sendRecords()
		if err != nil {
			return false
		}
		for index := range batchNotes {
			if _, ok := sendOf(records, fmt.Sprintf("note-%d", index+1)); !ok {
				return false
			}
		}
		return true
	})
	records, _ := sender.sendRecords()
	for index := range batchNotes {
		record, _ := sendOf(records, fmt.Sprintf("note-%d", index+1))
		t.Logf("note-%d: %s %s", index+1, record.Outcome, record.Detail)
	}
}

package workflow

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestEditAfterNotice is `rewake edit` end to end: main sends a task, the
// worker's turn has started and not read it, and main replaces the text by the
// short form of the id. The worker must find the old id marked replaced by the
// new one and the new text under the new id, and never the old text; its report
// settles the replacement, and main is left waiting on nothing.
func TestEditAfterNotice(t *testing.T) {
	runInColumns(t, "edit-after-notice", func(t *testing.T, col column) {
		binary := enterScenario(t, "edit-after-notice")
		c := Start(t, Spec{
			Name:         "edit-after-notice",
			Harness:      col.harness,
			Observations: editObservations,
			Deadline:     90 * time.Second,
		})
		judge(c, playEditAfterNotice(t, c, Isolate(t, c, binary), col))
	})
}

const (
	obsEditAnswered = "rewake edit with the task's short id, after the notice, is accepted and prints the new letter's id"
	obsEditRead     = "the worker finds the old id only as a note marked withdrawn and replaced by the new id, the new text under the new id marked as replacing the old, and never the old text"
	obsEditNoticed  = "the notice that brings the replacement shows it replacing the old task by its short id, followed by the new text, and no recall is sent"
	obsEditReported = "the worker's report settles the replacement and names no old task, and main's --awaited lists nothing after it"
	editOldText     = "edit-probe-old: rebuild the index on the primary"
	editNewText     = "edit-probe-new: rebuild the index on the replica"
)

var editObservations = []string{obsEditAnswered, obsEditRead, obsEditNoticed, obsEditReported}

func playEditAfterNotice(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	gate := filepath.Join(iso.Home, "worker.gate")
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1", shimReadGate+"="+gate)
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)
	defer openReadGate(gate)

	code, sent, _ := asks.ask(c, "send", worker.name, editOldText)
	old := printedID(sent)
	if code != 0 || old == "" || !atReadGate(c, gate) {
		return unjudgedAll(editObservations, "the task never reached the worker's turn: send exit %d, %q", code, sent)
	}
	code, answer, _ := asks.ask(c, "edit", shortOf(old), editNewText)
	replacement := printedID(answer)
	// Exit 3 is an accepted letter whose notice has not gone out yet: the
	// worker is in a turn, and a harness may take the next notice after it.
	out := []telemetryFinding{judged(obsEditAnswered, (code == 0 || code == 3) && replacement != "" && replacement != old,
		"edit %s exited %d: %q", shortOf(old), code, answer)}
	openReadGate(gate)
	if replacement == "" {
		return append(out, unjudgedAll(editObservations[1:], "edit printed no new id: %q", answer)...)
	}

	var tombstones, replacements []sentView
	if !waitFor(c, 30*time.Second, func() bool { tombstones = readByID(worker, old); return len(tombstones) > 0 }) {
		return append(out, unjudgedAll(editObservations[1:], "the worker never read anything under %s; it read %+v", old, sentRead(worker))...)
	}
	// The replacement has its own notice and may come a turn later.
	arrived := waitFor(c, 15*time.Second, func() bool { replacements = readByID(worker, replacement); return len(replacements) > 0 })
	tombstone := tombstones[0]
	marked := arrived && len(tombstones) == 1 && tombstone.Kind == "notify" && tombstone.Withdrawn != nil &&
		tombstone.Withdrawn.Kind == "task" && tombstone.Withdrawn.ReplacedBy == replacement &&
		strings.HasSuffix(tombstone.Text, "read the replacement, "+replacement+".") &&
		len(replacements) == 1 && replacements[0].Kind == "task" && replacements[0].Text == editNewText && replacements[0].Replaces == old
	for _, message := range sentRead(worker) {
		marked = marked && !strings.Contains(message.Text, editOldText)
	}
	out = append(out, judged(obsEditRead, marked, "under %s: %+v; under %s: %+v", old, tombstones, replacement, replacements))

	// The replacement's own line is what tells a worker that acted on the old
	// preview to stop and take up the new one; a recall beside it would take
	// the place of the new work in the notice, as the reviewers saw.
	shown, noticed := noticeCarrying(c, worker, "Replaces "+shortOf(old))
	recalls := 0
	for _, message := range sentRead(worker) {
		if message.Recall != nil {
			recalls++
		}
	}
	deliveries, _ := worker.groupDeliveries()
	out = append(out, judged(obsEditNoticed, noticed && strings.Contains(shown.Notice, "(withdrawn): edit-probe-new") && recalls == 0,
		"the notice naming %s: %+v; recalls read: %d; every notice the worker was shown: %+v", shortOf(old), shown, recalls, deliveries))

	var report reportView
	found := arrived && waitFor(c, 30*time.Second, func() bool {
		for _, message := range readMessages(lead) {
			if message.From == worker.name && message.Kind == "finished" && slices.Contains(message.InReplyTo, replacement) {
				report = message
				return true
			}
		}
		return false
	})
	_, awaited, _ := asks.ask(c, "inbox", "--awaited")
	return append(out, judged(obsEditReported,
		found && !slices.Contains(report.InReplyTo, old) && len(aboutTo(iso, lead, old)) == 0 && awaited == "Rewake: nobody owes you a report.\n",
		"report on %s found: %v, in reply to %v; about the old %s: %v; --awaited printed %q; kinds main read: %v",
		replacement, found, report.InReplyTo, old, aboutTo(iso, lead, old), awaited, kindsFrom(lead, worker)))
}

// The controls, on the gate column. An edit that sends the old text
// again under the new id: everything is linked, and the worker reads what the
// edit was meant to replace.
var mutantEditKeepsOldText = mutation{
	name:  "edit-keeps-old-text",
	file:  "internal/cli/edit.go",
	edits: []edit{{"Kind: old.Kind, Text: text,", "Kind: old.Kind, Text: old.Text,"}},
}

func playEditOnGate(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	return playEditAfterNotice(t, c, iso, gateColumn())
}

// A tombstone that does not name its replacement: everything arrives, and the
// reader cannot tell from the old id where the task went.
var mutantEditUnlinked = mutation{
	name:  "edit-unlinked",
	file:  "internal/inbox/withdraw.go",
	edits: []edit{{"\tnotice.ReplacedBy = replacedBy\n", ""}},
}

// A replacement announced like any other letter: everything arrives and is
// linked, and its notice no longer says what it replaces.
var mutantReplacementUnmarked = mutation{
	name:  "replacement-unmarked",
	file:  "internal/harness/notice.go",
	edits: []edit{{"\tif message.Replaces == \"\" {\n", "\tif true {\n"}},
}

func TestAnUnmarkedReplacementFails(t *testing.T) {
	runGateControl(t, "edit-after-notice", playEditOnGate, mutantReplacementUnmarked, obsEditNoticed)
}

func TestAnEditThatKeepsTheOldTextFails(t *testing.T) {
	runGateControl(t, "edit-after-notice", playEditOnGate, mutantEditKeepsOldText, obsEditRead, obsEditNoticed)
}

func TestAnUnlinkedEditFails(t *testing.T) {
	runGateControl(t, "edit-after-notice", playEditOnGate, mutantEditUnlinked, obsEditRead)
}

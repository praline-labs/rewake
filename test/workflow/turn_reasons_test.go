package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestTurnReasons is a failed and an interrupted turn reported with the reason
// from the end (O2), on the fixture column: the end's outcome and text travel
// through the adapter's frame, the neutral completion and the wrapper's
// confirmation before they become a report, and this is where that whole path
// is held (docs/v2/stage3-fixture.md).
//
//   - A worker whose turn fails holds tasks from main and from a peer: each of
//     the two reads an error about its own task, carrying the turn's reason, and
//     a session that waits on nothing receives nothing from it.
//   - A worker whose first turn is interrupted holds a task from main: main
//     reads it stopped, carrying that turn's reason.
//
// The other columns hear these ends through their own harness's events; their
// reasons are held by their own scenarios.
func TestTurnReasons(t *testing.T) {
	runParallel(t)
	col := fixtureColumn
	t.Run(col.harness, func(t *testing.T) {
		binary := enterScenario(t, "turn-reasons")
		c := Start(t, Spec{
			Name:         "turn-reasons",
			Harness:      col.harness,
			Observations: turnReasonObservations,
			Deadline:     90 * time.Second,
		})
		for _, finding := range playTurnReasons(t, c, Isolate(t, c, binary)) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

const (
	obsFailedReason  = "a failed turn reaches each sender waiting on it as an error about its own task, carrying the turn's reason, and reaches no one else"
	obsStoppedReason = "an interrupted turn reaches its sender as stopped, carrying the turn's reason"
	failedReasonText = "turn-reasons: the build broke on a missing header"
	stopReasonText   = "turn-reasons: the owner pressed Esc mid-edit"
	reasonsMainTask  = "turn-reasons-main: build the package"
	reasonsPeerTask  = "turn-reasons-peer: build the docs"
	reasonsStopTask  = "turn-reasons-stop: edit the config"
)

var turnReasonObservations = []string{obsFailedReason, obsStoppedReason}

func playTurnReasons(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	harness := fixtureColumn.harness
	failing := startHarnessSession(t, c, iso, harness, "failing", "--general",
		shimInboxJSON+"=1", shimLateFailure+"=1", shimEndReason+"="+failedReasonText)
	defer stopSession(t, c, failing)
	stopping := startHarnessSession(t, c, iso, harness, "stopping", "--general",
		shimInboxJSON+"=1", shimInterruptFirst+"=1", shimEndReason+"="+stopReasonText)
	defer stopSession(t, c, stopping)
	bystander := startHarnessSession(t, c, iso, harness, "bystander", "--general", shimInboxJSON+"=1")
	defer stopSession(t, c, bystander)
	peerAsks := newRequests(iso, "peer")
	peer := startHarnessSession(t, c, iso, harness, "peer", "--general", shimInboxJSON+"=1", peerAsks.env())
	defer stopSession(t, c, peer)
	leadAsks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, harness, "lead", "--main", shimInboxJSON+"=1", leadAsks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range turnReasonObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	// from is what a reader has read from one session, as kind, the tasks it
	// answers and its text.
	from := func(reader, sender *scenarioSession) []reportView {
		var out []reportView
		for _, message := range readMessages(reader) {
			if message.From == sender.name && message.Kind != "notify" {
				out = append(out, message)
			}
		}
		return out
	}
	show := func(messages []reportView) string {
		var lines []string
		for _, message := range messages {
			lines = append(lines, fmt.Sprintf("%s %v %q", message.Kind, message.InReplyTo, message.Text))
		}
		return "[" + strings.Join(lines, "; ") + "]"
	}
	// answers says whether exactly the messages read answer the task, each
	// of the kind and carrying the reason.
	answers := func(messages []reportView, task, kind, reason string) bool {
		if len(messages) == 0 {
			return false
		}
		for _, message := range messages {
			if message.Kind != kind || !slices.Equal(message.InReplyTo, []string{task}) || !strings.Contains(message.Text, reason) {
				return false
			}
		}
		return true
	}

	var sends []string
	for _, send := range []struct {
		asks     *requests
		to, text string
	}{
		{leadAsks, failing.name, reasonsMainTask},
		{peerAsks, failing.name, reasonsPeerTask},
		{leadAsks, stopping.name, reasonsStopTask},
	} {
		code, sent, _ := send.asks.ask(c, "send", send.to, send.text)
		sends = append(sends, fmt.Sprintf("%s: exit %d, %s", send.text, code, firstLine(sent)))
	}
	var mainTask, peerTask, stopTask reportView
	if !waitFor(c, 30*time.Second, func() bool {
		var a, b, d bool
		mainTask, a = messageCarrying(failing, reasonsMainTask)
		peerTask, b = messageCarrying(failing, reasonsPeerTask)
		stopTask, d = messageCarrying(stopping, reasonsStopTask)
		return a && b && d
	}) {
		return unjudged(fmt.Sprintf("the workers did not read what was sent; sends: %v", sends))
	}

	var out []telemetryFinding
	failedToMain := func() []reportView { return from(lead, failing) }
	failedToPeer := func() []reportView { return from(peer, failing) }
	heard := waitFor(c, 20*time.Second, func() bool {
		return answers(failedToMain(), mainTask.ID, "error", failedReasonText) &&
			answers(failedToPeer(), peerTask.ID, "error", failedReasonText)
	})
	// Whatever else the failure sent would be on its way by the time both
	// senders read theirs; a moment more is enough to see it.
	time.Sleep(time.Second)
	stray := mailboxFrom(iso, bystander.name, failing.name)
	out = append(out, finding(obsFailedReason, heard && answers(failedToMain(), mainTask.ID, "error", failedReasonText) &&
		answers(failedToPeer(), peerTask.ID, "error", failedReasonText) && len(stray) == 0,
		"main read %s, the peer read %s; the bystander holds %v", show(failedToMain()), show(failedToPeer()), stray))

	stoppedToMain := func() []reportView { return from(lead, stopping) }
	stopped := waitFor(c, 20*time.Second, func() bool {
		return answers(stoppedToMain(), stopTask.ID, "stopped", stopReasonText)
	})
	out = append(out, finding(obsStoppedReason, stopped, "main read %s", show(stoppedToMain())))
	return out
}

// mailboxFrom is the kinds of the messages a session holds from another, read
// or not: a session no one wakes has read nothing, so only its mailbox shows
// what reached it.
func mailboxFrom(iso *Isolation, reader, sender string) []string {
	mailbox := filepath.Join(iso.StateDir, "rooms", "default", "inbox", reader)
	files, _ := filepath.Glob(filepath.Join(mailbox, "*.json"))
	unread, _ := filepath.Glob(filepath.Join(mailbox, "unread", "*.json"))
	var kinds []string
	for _, file := range append(files, unread...) {
		var message reportView
		raw, err := os.ReadFile(file)
		if err == nil && json.Unmarshal(raw, &message) == nil && message.From == sender {
			kinds = append(kinds, message.Kind)
		}
	}
	return kinds
}

// The controls: the reason lost on the way from the frame to the completion,
// and a failure or an interruption taken for a completed turn.
var (
	mutantReasonDropped = mutation{
		name:  "reason-dropped",
		file:  "internal/harness/fixture/turns.go",
		edits: []edit{{"Kind: kind, Text: frame.Text,", "Kind: kind, Text: \"\","}},
	}
	mutantFailureCompleted = mutation{
		name:  "failure-completed",
		file:  "internal/harness/fixture/turns.go",
		edits: []edit{{"\tcase OutcomeFailed:\n\t\treturn inbox.Error, nil\n", "\tcase OutcomeFailed:\n\t\treturn inbox.Finished, nil\n"}},
	}
	mutantStopCompleted = mutation{
		name:  "stop-completed",
		file:  "internal/harness/fixture/turns.go",
		edits: []edit{{"\tcase OutcomeInterrupted:\n\t\treturn inbox.Stopped, nil\n", "\tcase OutcomeInterrupted:\n\t\treturn inbox.Finished, nil\n"}},
	}
)

func TestTurnReasonControls(t *testing.T) {
	runParallel(t)
	for _, control := range []struct {
		mutant mutation
		breaks []string
	}{
		{mutantReasonDropped, []string{obsFailedReason, obsStoppedReason}},
		{mutantFailureCompleted, []string{obsFailedReason}},
		{mutantStopCompleted, []string{obsStoppedReason}},
	} {
		t.Run(control.mutant.name, func(t *testing.T) {
			runFindingsControlOn(t, fixtureColumn.harness, "turn-reasons", playTurnReasons, control.mutant, control.breaks...)
		})
	}
}

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

// TestWithdrawAfterNotice is `rewake withdraw` end to end, at the moment it
// exists for: main sends a task, the worker's harness has taken the notice and
// started a turn, and the worker has not read the text yet. main withdraws the
// task by the short form of the id its send printed. The worker, reading now,
// must find the task's id marked withdrawn and never its text, and a note from
// main telling it not to act on the notice; nothing is owed, so nothing is
// reported, and main is told nothing about a failed delivery.
func TestWithdrawAfterNotice(t *testing.T) {
	runInColumns(t, "withdraw-after-notice", func(t *testing.T, col column) {
		binary := enterScenario(t, "withdraw-after-notice")
		c := Start(t, Spec{
			Name:         "withdraw-after-notice",
			Harness:      col.harness,
			Observations: withdrawObservations,
			Deadline:     90 * time.Second,
		})
		judge(c, playWithdrawAfterNotice(t, c, Isolate(t, c, binary), col, false))
	})
}

// TestWithdrawMidTurn is the same on the Codex column with the worker's turn
// held open, as a session is while it works on what a preview told it: the
// recall must reach that running turn by steer rather than wait for the next
// one, and name the task it stops. The other column has no turn in progress to
// deliver into (capabilityMidTurn).
func TestWithdrawMidTurn(t *testing.T) {
	binary := enterScenario(t, "withdraw-mid-turn")
	c := Start(t, Spec{
		Name:         "withdraw-mid-turn",
		Harness:      codexColumn.harness,
		Observations: withdrawMidTurnObservations,
		Deadline:     90 * time.Second,
	})
	judge(c, playWithdrawMidTurn(t, c, Isolate(t, c, binary)))
}

const (
	obsWithdrawAnswered = "rewake send prints the task's id, and rewake withdraw with its short form, after the notice, exits 0 and says the worker is told not to act on it and will find it marked withdrawn"
	obsWithdrawRead     = "the worker, reading after the withdrawal, finds under the task's id only a note marked withdrawn that names who withdrew which kind, and never the task's text"
	obsWithdrawRecalled = "the worker reads a note from main marked as the recall of the task's id"
	obsWithdrawNoticed  = "a notice the worker is shown carries the recall on a line of its own that begins with the instruction not to act on the task, by its short id"
	obsWithdrawSteered  = "with the worker's turn held open after the task's notice, the recall is steered into that same turn, and its member entry names the task's id"
	obsWithdrawOwed     = "nothing is reported about the withdrawn task, main is told of no failed delivery, and main's --awaited lists nothing"
	withdrawTaskText    = "withdraw-probe: delete the staging bucket"
)

var (
	withdrawObservations        = []string{obsWithdrawAnswered, obsWithdrawRead, obsWithdrawRecalled, obsWithdrawNoticed, obsWithdrawOwed}
	withdrawMidTurnObservations = append(slices.Clone(withdrawObservations), obsWithdrawSteered)
)

// sentView is a message as a scenario about actions on sent mail reads it: the
// fields that tie a tombstone, a replacement or an addendum to what it is about.
type sentView struct {
	reportView
	AddendumTo string `json:"addendumTo"`
	Replaces   string `json:"replaces"`
	Withdrawn  *struct {
		Kind       string `json:"kind"`
		ReplacedBy string `json:"replacedBy"`
	} `json:"withdrawn"`
	Recall *struct {
		ID string `json:"id"`
	} `json:"recall"`
}

// sentRead is every message a session's own inbox calls returned, with those
// fields, in the order they were read.
func sentRead(reader *codexSession) []sentView {
	var all []sentView
	for _, chunk := range strings.Split(reader.mailboxRead(), "\n---\n") {
		var model struct {
			Messages []sentView `json:"messages"`
		}
		if strings.TrimSpace(chunk) != "" && json.Unmarshal([]byte(chunk), &model) == nil {
			all = append(all, model.Messages...)
		}
	}
	return all
}

// readByID picks out what a session read under one id.
func readByID(reader *codexSession, id string) []sentView {
	var out []sentView
	for _, message := range sentRead(reader) {
		if message.ID == id {
			out = append(out, message)
		}
	}
	return out
}

// printedID is the id a send or an edit printed on its own line.
func printedID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if id, ok := strings.CutPrefix(line, "id "); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

// shortOf is the short form a person copies: eight characters of the random
// tail, as a git hash is shortened.
func shortOf(id string) string {
	_, tail, _ := strings.Cut(id, "-")
	if len(tail) < 8 {
		return id
	}
	return tail[:8]
}

// mailboxPath is a file or directory of a session's mailbox.
func mailboxPath(iso *Isolation, session *codexSession, parts ...string) string {
	return filepath.Join(append([]string{iso.StateDir, "rooms", "default", "inbox", session.name}, parts...)...)
}

// atReadGate waits until the worker's turn has started and stopped at the
// gate, and returns the file that opens it.
func atReadGate(c *Case, gate string) bool {
	return waitFor(c, 30*time.Second, func() bool {
		_, err := os.Stat(gate + ".waiting")
		return err == nil
	})
}

func openReadGate(gate string) { _ = os.WriteFile(gate, nil, 0o600) }

// aboutTo lists the messages in a session's mailbox, read or not, that answer
// one of the ids or say it was not delivered: what a sender would learn about
// them.
func aboutTo(iso *Isolation, reader *codexSession, ids ...string) []string {
	var found []string
	for _, where := range [][]string{{}, {"unread"}, {"done"}} {
		files, _ := filepath.Glob(filepath.Join(mailboxPath(iso, reader, where...), "*.json"))
		for _, file := range files {
			raw, err := os.ReadFile(file)
			var message reportView
			if err != nil || json.Unmarshal(raw, &message) != nil {
				continue
			}
			for _, id := range ids {
				if slices.Contains(message.InReplyTo, id) || message.Undelivered != nil && message.Undelivered.ID == id {
					found = append(found, message.Kind+" "+message.ID)
					break
				}
			}
		}
	}
	return found
}

// judge turns findings into the case's verdict.
func judge(c *Case, findings []telemetryFinding) {
	for _, finding := range findings {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

func judged(observation string, held bool, detail string, args ...any) telemetryFinding {
	return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
}

func unjudgedAll(observations []string, detail string, args ...any) []telemetryFinding {
	var out []telemetryFinding
	for _, observation := range observations {
		out = append(out, telemetryFinding{observation: observation, detail: fmt.Sprintf(detail, args...)})
	}
	return out
}

// recallLine is how a notice shows the recall of a task: a line of its own,
// the instruction first. What the reviewers saw on September 26, 2026 was a
// recall hidden behind a newer letter's preview, or cut before its point.
func recallLine(task string) string { return "\n  ↳ Do not act on task " + shortOf(task) }

// noticeCarrying is the first notice a session was shown that holds text. A
// notice may be recorded after the read it announced — the text is readable
// before the notice goes — so it is waited for.
func noticeCarrying(c *Case, session *codexSession, text string) (groupDelivery, bool) {
	var found groupDelivery
	shown := waitFor(c, 10*time.Second, func() bool {
		deliveries, _ := session.groupDeliveries()
		for _, delivery := range deliveries {
			if strings.Contains(delivery.Notice, text) {
				found = delivery
				return true
			}
		}
		return false
	})
	return found, shown
}

func playWithdrawMidTurn(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	return playWithdrawAfterNotice(t, c, iso, codexColumn, true)
}

// playWithdrawAfterNotice plays the scenario; held keeps the worker's first
// turn open until something is steered into it.
func playWithdrawAfterNotice(t *testing.T, c *Case, iso *Isolation, col column, held bool) []telemetryFinding {
	t.Helper()
	gate := filepath.Join(iso.Home, "worker.gate")
	controls := []string{shimInboxJSON + "=1", shimReadGate + "=" + gate}
	observations := withdrawObservations
	if held {
		controls, observations = append(controls, shimHoldTurn+"=1"), withdrawMidTurnObservations
	}
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", controls...)
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)
	defer openReadGate(gate)

	code, sent, _ := asks.ask(c, "send", worker.name, withdrawTaskText)
	task := printedID(sent)
	if code != 0 || task == "" || !atReadGate(c, gate) {
		return unjudgedAll(observations, "the task never reached the worker's turn: send exit %d, %q", code, sent)
	}
	code, answer, _ := asks.ask(c, "withdraw", shortOf(task))
	out := []telemetryFinding{judged(obsWithdrawAnswered,
		code == 0 && strings.HasPrefix(answer, "Rewake: withdrew your task "+task+" from "+worker.name+"; its notice may have gone out, so "+worker.name+" is told not to act on it"),
		"send printed %q; withdraw %s exited %d: %q", sent, shortOf(task), code, answer)}
	openReadGate(gate)

	var read []sentView
	if !waitFor(c, 30*time.Second, func() bool { read = readByID(worker, task); return len(read) > 0 }) {
		return append(out, unjudgedAll(observations[1:], "the worker never read anything under %s; it read %v", task, sentRead(worker))...)
	}
	tombstone := read[0]
	marked := len(read) == 1 && tombstone.Kind == "notify" && tombstone.Withdrawn != nil && tombstone.Withdrawn.Kind == "task" &&
		tombstone.Withdrawn.ReplacedBy == "" && strings.HasPrefix(tombstone.Text, "Rewake: "+lead.name+" withdrew its task of ") &&
		strings.HasSuffix(tombstone.Text, " before you read it; disregard its notice.")
	for _, message := range sentRead(worker) {
		marked = marked && !strings.Contains(message.Text, withdrawTaskText)
	}
	out = append(out, judged(obsWithdrawRead, marked, "the worker read under %s: %+v; everything it read: %d messages", task, read, len(sentRead(worker))))

	// The recall is announced at once; it may come in the same read or start
	// a turn of its own.
	var recall *sentView
	recalled := waitFor(c, 15*time.Second, func() bool {
		for _, message := range sentRead(worker) {
			if message.Recall != nil && message.Recall.ID == task {
				recall = &message
				return true
			}
		}
		return false
	})
	out = append(out, judged(obsWithdrawRecalled,
		recalled && recall.Kind == "notify" && recall.From == lead.name,
		"the recall the worker read: %+v; everything it read: %+v", recall, sentRead(worker)))
	shown, noticed := noticeCarrying(c, worker, recallLine(task))
	deliveries, _ := worker.groupDeliveries()
	out = append(out, judged(obsWithdrawNoticed, noticed, "the notice with the recall: %+v; every notice the worker was shown: %+v", shown, deliveries))
	if held {
		out = append(out, steeredRecall(c, worker, task))
	}

	// The turn ends with nothing owed; a report or a note would come right
	// behind that, so the quiet is watched for a while rather than sampled.
	var about []string
	quiet := !waitFor(c, 3*time.Second, func() bool { about = aboutTo(iso, lead, task); return len(about) > 0 })
	_, awaited, _ := asks.ask(c, "inbox", "--awaited")
	status, _ := os.ReadFile(mailboxPath(iso, worker, task+".status"))
	var onDisk struct {
		State     string `json:"state"`
		Withdrawn bool   `json:"withdrawn"`
	}
	_ = json.Unmarshal(status, &onDisk)
	return append(out, judged(obsWithdrawOwed,
		quiet && awaited == "Rewake: nobody owes you a report.\n" && onDisk.State == "failed" && onDisk.Withdrawn,
		"main's mailbox about %s: %v; --awaited printed %q; the status on disk: %s", task, about, awaited, status))
}

// steeredRecall judges whether the recall came into the turn the task's notice
// started, while it was still open, as a member naming the task.
func steeredRecall(c *Case, worker *codexSession, task string) telemetryFinding {
	var events []turnEvent
	waitFor(c, 10*time.Second, func() bool {
		events, _ = worker.turnEvents()
		return eventOf(events, "operation-closed") != nil
	})
	deliveries, _ := worker.groupDeliveries()
	taskTurn := ""
	for _, delivery := range deliveries {
		if slices.Contains(delivery.Members, task) {
			taskTurn = delivery.Turn
		}
	}
	steered, into := false, false
	for _, event := range events {
		steered = steered || event.Kind == "steered" && event.Turn == taskTurn
		into = into || event.Kind == "recall" && event.Turn == taskTurn && strings.HasSuffix(event.Detail, " "+task)
	}
	return judged(obsWithdrawSteered, taskTurn != "" && steered && into,
		"the task came in turn %q; the worker's turn events: %+v", taskTurn, events)
}

// The control, on the Claude Code column: a withdrawal that writes its
// tombstone where nobody reads and no withdrawn status, so the reader has
// nothing to show the task as withdrawn by. The answer still says withdrawn
// and the recall still goes; the worker reads the task and reports on it.
var mutantWithdrawLeavesTask = mutation{
	name: "withdraw-leaves-task",
	file: "internal/inbox/withdraw.go",
	edits: []edit{
		{"\t\tresult, where = WithdrawnAnnounced, state.UnreadPath(dir, to)\n", "\t\tresult = WithdrawnAnnounced\n"},
		{"\tif err := writeStatus(dir, to, id, Result{State: Failed, Detail: detail, Withdrawn: true}); err != nil {\n", "\tif err := error(nil); err != nil {\n"},
	},
}

// The second control: a withdrawal that does not tell the recipient, as the
// design first had it. Only the recall is missing.
var mutantWithdrawSilent = mutation{
	name:  "withdraw-silent",
	file:  "internal/cli/withdraw.go",
	edits: []edit{{"\tif result == inbox.WithdrawnAnnounced {\n", "\tif false {\n"}},
}

func playWithdrawOnClaude(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	return playWithdrawAfterNotice(t, c, iso, claudeColumn, false)
}

// A recall whose text leads with its sender, as it first did: the note still
// goes, and the notice line no longer begins with what not to act on.
var mutantRecallSenderFirst = mutation{
	name:  "recall-sender-first",
	file:  "internal/inbox/recall.go",
	edits: []edit{{`"Do not act on %s %s from %s (%s): withdrawn unread.", kind, ShortID(message.ID), message.From, at`, `"Rewake: %[3]s withdrew its %[1]s of %[4]s (%[2]s) before you read it — do not act on it.", kind, ShortID(message.ID), message.From, at`}},
}

// On Codex, a recall member that does not name what it recalls: the recall is
// steered into the turn all the same, and only its entry is mute.
var mutantRecallUnnamed = mutation{
	name:  "recall-unnamed",
	file:  "internal/harness/codex/server_delivery.go",
	edits: []edit{{"\t\t\tentry.Recalls = member.Recall.ID\n", ""}},
}

func TestAWithdrawalThatLeavesTheTaskFails(t *testing.T) {
	runFindingsControl(t, "withdraw-after-notice", playWithdrawOnClaude, mutantWithdrawLeavesTask, obsWithdrawRead, obsWithdrawOwed)
}

func TestASilentWithdrawalFails(t *testing.T) {
	runFindingsControl(t, "withdraw-after-notice", playWithdrawOnClaude, mutantWithdrawSilent, obsWithdrawRecalled, obsWithdrawNoticed)
}

func TestARecallLedBySenderFails(t *testing.T) {
	runFindingsControl(t, "withdraw-after-notice", playWithdrawOnClaude, mutantRecallSenderFirst, obsWithdrawNoticed)
}

func TestAnUnnamedRecallFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "withdraw-mid-turn", playWithdrawMidTurn, mutantRecallUnnamed, obsWithdrawSteered)
}

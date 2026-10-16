package workflow

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// TestFixtureThreadChange is conversation tracking on the gate column
// (HF-10): a report from a conversation other than the one its task was
// delivered to carries threadChanged, so the sender knows the answer may not
// be about its task. Two workers, each with a sender of its own: one starts a
// new conversation after its task arrives, as a person's /clear does, tells
// its adapter, and works the task there; the other works its task where it
// landed.
//
// The fixture is the only harness reporting a change of conversation from S9
// until stage 4's Claude Code adapter does again. What it does not prove: that
// a real harness names its new conversation as the program does; Claude
// Code's session_id and its change on /clear are recorded in docs/research.md.
func TestFixtureThreadChange(t *testing.T) {
	binary := enterScenario(t, "thread-changed")
	c := Start(t, Spec{
		Name:         "thread-changed",
		Harness:      fixtureColumn.harness,
		Observations: threadObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playThreadChange(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsThreadMarked   = "a report from the conversation the worker started after its task arrived carries threadChanged"
	obsThreadUnmarked = "a report from the conversation the task was delivered to carries no threadChanged"
	obsClearedSettles = "the report from the new conversation still arrives once and settles the task"
	threadClearedText = "thread-cleared-probe"
	threadKeptText    = "thread-kept-probe"
)

var threadObservations = []string{obsThreadMarked, obsThreadUnmarked, obsClearedSettles}

// threadPair is a worker and the session that sends it one task.
type threadPair struct{ worker, sender *scenarioSession }

func startThreadPair(t *testing.T, c *Case, iso *Isolation, label, text string, controls ...string) threadPair {
	t.Helper()
	worker := startHarnessSession(t, c, iso, fixtureColumn.harness, label, "--general", append([]string{shimInboxJSON + "=1"}, controls...)...)
	// The sender waits for the session to be up, so its conversation has
	// been named before the task is delivered into it.
	sender := startHarnessSession(t, c, iso, fixtureColumn.harness, "to-"+label, "--general",
		shimSendTo+"="+worker.name, shimSendText+"="+text, shimInboxJSON+"=1", readinessSwitch(worker))
	return threadPair{worker: worker, sender: sender}
}

// reports are the sender's messages from its worker about one task.
func (p threadPair) reports(task string) []reportView {
	var out []reportView
	for _, message := range readMessages(p.sender) {
		if message.From == p.worker.name && slices.Contains(message.InReplyTo, task) {
			out = append(out, message)
		}
	}
	return out
}

func playThreadChange(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	cleared := startThreadPair(t, c, iso, "cleared", threadClearedText, shimNewThread+"=fixture-thread-0002")
	kept := startThreadPair(t, c, iso, "kept", threadKeptText)
	for _, pair := range []threadPair{cleared, kept} {
		defer stopSession(t, c, pair.worker)
		defer stopSession(t, c, pair.sender)
	}

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	var out []telemetryFinding
	// Each worker's report, anchored on the sender having read one about the
	// task its worker read. Unjudged when none came: that is a broken
	// delivery or turn end, which other scenarios own, not a verdict on the
	// conversation.
	report := func(pair threadPair, marker string, observations ...string) (reportView, string, bool) {
		var task reportView
		var reports []reportView
		if !waitFor(c, 30*time.Second, func() bool {
			var ok bool
			task, ok = messageCarrying(pair.worker, marker)
			if ok {
				reports = pair.reports(task.ID)
			}
			return len(reports) > 0
		}) {
			for _, observation := range observations {
				out = append(out, telemetryFinding{observation: observation, detail: pair.worker.name + " never reported on its task"})
			}
			return reportView{}, "", false
		}
		return reports[0], task.ID, true
	}

	if answer, task, ok := report(cleared, threadClearedText, obsThreadMarked, obsClearedSettles); ok {
		out = append(out, finding(obsThreadMarked, answer.ThreadChanged, "report %s on %s: kind %s, threadChanged %v", answer.ID, task, answer.Kind, answer.ThreadChanged))
		settled := waitFor(c, 5*time.Second, func() bool { return len(awaiting(iso, cleared.worker)) == 0 })
		// A second report would come right behind the first; give it a moment.
		time.Sleep(500 * time.Millisecond)
		count := len(cleared.reports(task))
		out = append(out, finding(obsClearedSettles, answer.Kind == "finished" && settled && count == 1,
			"kind %s; settled: %v; reports about it: %d", answer.Kind, settled, count))
	}
	if answer, task, ok := report(kept, threadKeptText, obsThreadUnmarked); ok {
		out = append(out, finding(obsThreadUnmarked, !answer.ThreadChanged, "report %s on %s: kind %s, threadChanged %v", answer.ID, task, answer.Kind, answer.ThreadChanged))
	}
	return out
}

// The controls: three product mutants, one for each half of the comparison
// and one for a comparison that marks everything. Each names what it must
// break and requires the rest to hold.

// The delivery is never pinned: the reservation records no conversation.
var mutantDeliveryUnpinned = mutation{
	name:  "delivery-unpinned",
	file:  "internal/host/delivery/prepare.go",
	edits: []edit{{"if thread != \"\" && mail.Owed(*message) {", "if false && thread != \"\" && mail.Owed(*message) {"}},
}

// The turn end does not carry the conversation it ended in.
var mutantEndThreadIgnored = mutation{
	name: "end-thread-ignored",
	file: "internal/cli/completion.go",
	edits: []edit{
		{"mail.CompleteTurn(ctx, dir, self, turnEndOf(end), end.Thread,", "mail.CompleteTurn(ctx, dir, self, turnEndOf(end), \"\","},
		{"mail.EndTurn(ctx, dir, self, turnEndOf(end), true, end.Thread,", "mail.EndTurn(ctx, dir, self, turnEndOf(end), true, \"\","},
	},
}

// Any known delivery conversation counts as changed.
var mutantThreadAlwaysChanged = mutation{
	name:  "thread-always-changed",
	file:  "internal/core/mail/thread.go",
	edits: []edit{{"before != \"\" && before != current {", "before != \"\" {"}},
}

func TestAnUnpinnedDeliveryFails(t *testing.T) {
	runThreadControl(t, mutantDeliveryUnpinned, obsThreadMarked)
}

func TestAnIgnoredEndConversationFails(t *testing.T) {
	runThreadControl(t, mutantEndThreadIgnored, obsThreadMarked)
}

func TestAConversationAlwaysChangedFails(t *testing.T) {
	runThreadControl(t, mutantThreadAlwaysChanged, obsThreadUnmarked)
}

func runThreadControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, fixtureColumn.harness, "thread-changed", playThreadChange, mutant, breaks...)
}

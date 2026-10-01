package workflow

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestAwaitedView is `rewake inbox --awaited` end to end: a main session hands
// a task to each of two workers. One reports at once; the other ends its turn
// with rewake pending, so its report is still owed. main's view must then list
// exactly the second task, as pending. Woken by a note, the second worker
// reports too, and the view must be empty.
//
// main runs every command itself, when the scenario asks: the view is about
// main's own run, and a test process sending in its name would prove nothing
// about what a session sees.
func TestAwaitedView(t *testing.T) {
	runInColumns(t, "awaited-view", func(t *testing.T, col column) {
		binary := enterScenario(t, "awaited-view")
		c := Start(t, Spec{
			Name:         "awaited-view",
			Harness:      col.harness,
			Observations: awaitedObservations,
			Deadline:     90 * time.Second,
		})
		for _, finding := range playAwaitedView(t, c, Isolate(t, c, binary), col) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

const (
	obsAwaitedOther = "after one worker reports, main's rewake inbox --awaited lists exactly the other worker's task, as pending"
	obsAwaitedEmpty = "after the other worker reports too, the view lists nothing"
	awaitedQuick    = "awaited-view-quick: fix the typo"
	awaitedSlow     = "awaited-view-slow: run the long check\nand report the tail"
	awaitedPending  = "the long check is running"
	awaitedWake     = "awaited-view-wake: the check finished; wrap up"
)

var awaitedObservations = []string{obsAwaitedOther, obsAwaitedEmpty}

// awaitedModelView is the machine form of the view, as far as the scenario
// judges it.
type awaitedModelView struct {
	Recipients []struct {
		Name     string `json:"name"`
		Messages []struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			State  string `json:"state"`
			Detail string `json:"detail"`
			Gone   string `json:"gone"`
			Text   string `json:"text"`
		} `json:"messages"`
	} `json:"recipients"`
}

func playAwaitedView(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	quick := startHarnessSession(t, c, iso, col.harness, "quick", "--general", shimInboxJSON+"=1")
	defer stopSession(t, c, quick)
	slow := startHarnessSession(t, c, iso, col.harness, "slow", "--general", shimInboxJSON+"=1", shimPendingOnce+"="+awaitedPending)
	defer stopSession(t, c, slow)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(observations []string, detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range observations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	// A report main has read about one task, of one kind.
	readAbout := func(worker *codexSession, task, kind string) bool {
		for _, message := range readMessages(lead) {
			if message.From == worker.name && message.Kind == kind && slices.Contains(message.InReplyTo, task) {
				return true
			}
		}
		return false
	}
	// The view in both forms, as main's own rewake printed it.
	view := func() (awaitedModelView, string, string) {
		codeJSON, machine, okJSON := asks.ask(c, "inbox", "--awaited", "--json")
		codePlain, plain, okPlain := asks.ask(c, "inbox", "--awaited")
		var model awaitedModelView
		parsed := okJSON && okPlain && codeJSON == 0 && codePlain == 0 && json.Unmarshal([]byte(machine), &model) == nil
		if !parsed {
			return awaitedModelView{Recipients: nil}, plain, fmt.Sprintf("the view did not come back: exit %d/%d, %q / %q", codeJSON, codePlain, machine, plain)
		}
		return model, plain, ""
	}

	var sends []string
	for _, letter := range []struct {
		to   *codexSession
		text string
	}{{quick, awaitedQuick}, {slow, awaitedSlow}} {
		code, out, _ := asks.ask(c, "send", letter.to.name, letter.text)
		sends = append(sends, fmt.Sprintf("to %s: exit %d, %s", letter.to.name, code, firstLine(out)))
	}
	var quickTask, slowTask reportView
	if !waitFor(c, 30*time.Second, func() bool {
		var readQuick, readSlow bool
		quickTask, readQuick = messageCarrying(quick, "awaited-view-quick")
		slowTask, readSlow = messageCarrying(slow, "awaited-view-slow")
		return readQuick && readSlow && readAbout(quick, quickTask.ID, "finished") && readAbout(slow, slowTask.ID, "pending")
	}) {
		return unjudged(awaitedObservations, fmt.Sprintf("main never read a report from quick and an interim from slow; sends: %v; kinds main read: %v, %v",
			sends, kindsFrom(lead, quick), kindsFrom(lead, slow)))
	}

	model, plain, failure := view()
	var out []telemetryFinding
	if failure != "" {
		out = append(out, finding(obsAwaitedOther, false, "%s", failure))
	} else {
		only := len(model.Recipients) == 1 && model.Recipients[0].Name == slow.name && len(model.Recipients[0].Messages) == 1
		shown := only
		if only {
			message := model.Recipients[0].Messages[0]
			shown = message.ID == slowTask.ID && message.Kind == "task" && message.State == "pending" &&
				// The interim report's text: the mark's line, then the turn's own.
				strings.HasPrefix(message.Detail, awaitedPending+"\n\n") && message.Text == awaitedSlow && message.Gone == ""
		}
		shown = shown && strings.HasPrefix(plain, "Rewake: waiting on 1 report:\n\nto "+slow.name+"\n"+slowTask.ID+" · task · ") &&
			strings.Contains(plain, " · pending: "+awaitedPending+"\nawaited-view-slow: run the long check\n") &&
			!strings.Contains(plain, quickTask.ID) && !strings.Contains(plain, "and report the tail")
		out = append(out, finding(obsAwaitedOther, shown, "quick's task %s, slow's task %s; --awaited printed %q", quickTask.ID, slowTask.ID, plain))
	}

	// The wake: a note owes nothing, so it adds nothing to the view, and the
	// turn it starts ends with no mark — the report that settles slow's task.
	code, sent, _ := asks.ask(c, "send", slow.name, awaitedWake, "--notify")
	if !waitFor(c, 30*time.Second, func() bool { return readAbout(slow, slowTask.ID, "finished") }) {
		return append(out, unjudged([]string{obsAwaitedEmpty}, fmt.Sprintf("main never read slow's report after the note (exit %d, %s); kinds main read: %v",
			code, firstLine(sent), kindsFrom(lead, slow)))...)
	}
	model, plain, failure = view()
	if failure != "" {
		return append(out, finding(obsAwaitedEmpty, false, "%s", failure))
	}
	return append(out, finding(obsAwaitedEmpty, len(model.Recipients) == 0 && plain == "Rewake: nobody owes you a report.\n",
		"--awaited printed %q", plain))
}

// The controls, on the Claude Code column only: the view reads files the same
// way whichever harness wrote them.

// A view blind to interim reports shows the slow task as plainly owed.
var mutantAwaitedInterimIgnored = mutation{
	name:  "awaited-interim-ignored",
	file:  "internal/inbox/awaited.go",
	edits: []edit{{"\t\tcase Interim, Stopped:\n", "\t\tcase \"\":\n"}},
}

// A view that never lets a task go: neither a report nor a cleared wait
// settles it.
var mutantAwaitedNeverSettled = mutation{
	name: "awaited-never-settled",
	file: "internal/inbox/awaited.go",
	edits: []edit{
		{"\tif err != nil || settled {\n", "\tif err != nil || settled && false {\n"},
		{"\tif item.read() && !slices.Contains(", "\tif false && item.read() && !slices.Contains("},
	},
}

func playAwaitedOnClaude(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	return playAwaitedView(t, c, iso, claudeColumn)
}

func TestAnAwaitedViewBlindToInterimsFails(t *testing.T) {
	runFindingsControl(t, "awaited-view", playAwaitedOnClaude, mutantAwaitedInterimIgnored, obsAwaitedOther)
}

func TestAnAwaitedViewThatNeverSettlesFails(t *testing.T) {
	runFindingsControl(t, "awaited-view", playAwaitedOnClaude, mutantAwaitedNeverSettled, obsAwaitedOther, obsAwaitedEmpty)
}

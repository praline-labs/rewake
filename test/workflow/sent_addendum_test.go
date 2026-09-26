package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestAddendumOwed is `rewake send --to` end to end, read the way a session
// reads its work after a context compaction: main sends a task, and while the
// worker's turn has not read it yet, an addendum to it by the task's short id.
// The worker reads both, then runs `rewake inbox --owed`, which must show the
// addendum under its task; one report at the turn's end settles the two.
func TestAddendumOwed(t *testing.T) {
	runInColumns(t, "addendum-owed", func(t *testing.T, col column) {
		binary := enterScenario(t, "addendum-owed")
		c := Start(t, Spec{
			Name:         "addendum-owed",
			Harness:      col.harness,
			Observations: addendumObservations,
			Deadline:     90 * time.Second,
		})
		judge(c, playAddendumOwed(t, c, Isolate(t, c, binary), col))
	})
}

const (
	obsAddendumSent     = "rewake send --to with the task's short id is accepted and prints the addendum's id"
	obsAddendumOwed     = "rewake inbox --owed in the worker's turn lists the addendum under its task, in both forms"
	obsAddendumReported = "one report settles the task and its addendum, and main's --awaited lists nothing after it"
	addendumTaskText    = "addendum-probe-task: rerun the migration check"
	addendumText        = "addendum-probe-more: and the rollback check too"
)

var addendumObservations = []string{obsAddendumSent, obsAddendumOwed, obsAddendumReported}

func playAddendumOwed(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	gate := filepath.Join(iso.Home, "worker.gate")
	record := filepath.Join(iso.Home, "owed.out")
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1",
		shimReadGate+"="+gate, shimOwedFile+"="+record)
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col.harness, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)
	defer openReadGate(gate)

	code, sent, _ := asks.ask(c, "send", worker.name, addendumTaskText)
	task := printedID(sent)
	if code != 0 || task == "" || !atReadGate(c, gate) {
		return unjudgedAll(addendumObservations, "the task never reached the worker's turn: send exit %d, %q", code, sent)
	}
	code, answer, _ := asks.ask(c, "send", worker.name, addendumText, "--to", shortOf(task))
	addendum := printedID(answer)
	// Exit 3 is an accepted letter whose notice has not gone out yet.
	out := []telemetryFinding{judged(obsAddendumSent, (code == 0 || code == 3) && addendum != "" && addendum != task,
		"send --to %s exited %d: %q", shortOf(task), code, answer)}
	if addendum == "" {
		return append(out, unjudgedAll(addendumObservations[1:], "send --to printed no id: %q", answer)...)
	}
	// The addendum is announced at once; its readable copy is made before its
	// notice, so once it is there one read takes the task and the addendum.
	readable := waitFor(c, 15*time.Second, func() bool {
		_, err := os.Stat(mailboxPath(iso, worker, "unread", addendum+".json"))
		return err == nil
	})
	openReadGate(gate)
	if !readable {
		return append(out, unjudgedAll(addendumObservations[1:], "the addendum %s never became readable while the worker's turn waited", addendum)...)
	}

	var raw []byte
	if !waitFor(c, 30*time.Second, func() bool {
		var err error
		raw, err = os.ReadFile(record)
		return err == nil && len(raw) > 0
	}) {
		return append(out, unjudgedAll(addendumObservations[1:], "the worker never ran rewake inbox --owed; it read %+v", sentRead(worker))...)
	}
	machine, plain, _ := strings.Cut(string(raw), owedSeparator)
	var model struct {
		Messages []sentView `json:"messages"`
	}
	parsed := json.Unmarshal([]byte(machine), &model) == nil
	var ids []string
	nested := false
	for _, message := range model.Messages {
		ids = append(ids, message.ID)
		nested = nested || message.ID == addendum && message.AddendumTo == task
	}
	under := "\n" + addendumTaskText + "\n\n+ addendum from " + lead.name + " · "
	shown := parsed && slices.Equal(ids, []string{task, addendum}) && nested &&
		strings.HasPrefix(plain, "Rewake: owed a report for 2 messages:\n\nfrom "+lead.name+" · task · ") &&
		strings.Contains(plain, under) && strings.HasSuffix(strings.TrimSpace(plain), addendumText)
	out = append(out, judged(obsAddendumOwed, shown, "task %s, addendum %s; --owed printed %q", task, addendum, raw))

	var reports []reportView
	found := waitFor(c, 30*time.Second, func() bool {
		reports = nil
		for _, message := range readMessages(lead) {
			if message.From == worker.name && (slices.Contains(message.InReplyTo, task) || slices.Contains(message.InReplyTo, addendum)) {
				reports = append(reports, message)
			}
		}
		return len(reports) > 0
	})
	// A second report would come right behind the first; give it a moment.
	time.Sleep(500 * time.Millisecond)
	settles := found && len(aboutTo(iso, lead, task, addendum)) == 1 && reports[0].Kind == "finished" &&
		slices.Contains(reports[0].InReplyTo, task) && slices.Contains(reports[0].InReplyTo, addendum)
	_, awaited, _ := asks.ask(c, "inbox", "--awaited")
	return append(out, judged(obsAddendumReported, settles && awaited == "Rewake: nobody owes you a report.\n",
		"reports read by main: %+v; in its mailbox: %v; --awaited printed %q", reports, aboutTo(iso, lead, task, addendum), awaited))
}

// The control, on the Claude Code column: --owed that places no addendum
// under its task, so a session re-reading its work takes the addendum for a
// task of its own.
var mutantOwedFlat = mutation{
	name:  "owed-flat",
	file:  "internal/cli/inbox_owed.go",
	edits: []edit{{"\t\tif message.AddendumTo == \"\" {\n\t\t\troots[message.ID] = true\n", "\t\tif false {\n\t\t\troots[message.ID] = true\n"}},
}

func playAddendumOnClaude(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	return playAddendumOwed(t, c, iso, claudeColumn)
}

func TestAFlatOwedFails(t *testing.T) {
	runFindingsControl(t, "addendum-owed", playAddendumOnClaude, mutantOwedFlat, obsAddendumOwed)
}

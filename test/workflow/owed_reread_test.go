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

// TestOwedReread is `rewake inbox --owed` end to end: a worker reads a task,
// then — as a session does after a context compaction — asks for the work it
// owes a report on, and must get that task back in full. Asking changes
// nothing, so the turn end then reports the task and settles it as usual.
func TestOwedReread(t *testing.T) {
	runInColumns(t, "owed-reread", func(t *testing.T, col column) {
		binary := enterScenario(t, "owed-reread")
		c := Start(t, Spec{
			Name:         "owed-reread",
			Harness:      col.harness,
			Observations: owedObservations,
			Deadline:     90 * time.Second,
		})
		for _, finding := range playOwedReread(t, c, Isolate(t, c, binary), col) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

const (
	obsOwedShown    = "rewake inbox --owed in the worker's turn prints the task it read, in full, with its id in the machine form"
	obsOwedReported = "the turn end after it reports the task once and settles it"
	// owedTaskText has several lines, so a shortened or preview copy shows.
	owedTaskText = "owed-reread-probe: rerun the check\nsecond line of the brief\nthird line, the last"
)

var owedObservations = []string{obsOwedShown, obsOwedReported}

func playOwedReread(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	record := filepath.Join(iso.Home, "owed.out")
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1", shimOwedFile+"="+record)
	defer stopSession(t, c, worker)
	sender := startHarnessSession(t, c, iso, col.harness, pendingSenderLabel, "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+owedTaskText, shimInboxJSON+"=1", readinessSwitch(worker))
	defer stopSession(t, c, sender)

	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range owedObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}

	var task reportView
	var raw []byte
	if !waitFor(c, 30*time.Second, func() bool {
		var ok bool
		task, ok = messageCarrying(worker, "owed-reread-probe")
		var err error
		raw, err = os.ReadFile(record)
		return ok && err == nil && len(raw) > 0
	}) {
		return unjudged("the worker never read the task and ran rewake inbox --owed")
	}
	machine, plain, _ := strings.Cut(string(raw), owedSeparator)
	var model struct {
		Messages []struct {
			reportView
			Kept bool `json:"kept"`
		} `json:"messages"`
	}
	parsed := json.Unmarshal([]byte(machine), &model) == nil
	shown := parsed && len(model.Messages) == 1 && model.Messages[0].ID == task.ID &&
		model.Messages[0].Text == owedTaskText && model.Messages[0].Kind == "task" && model.Messages[0].Kept &&
		strings.HasPrefix(plain, "Rewake: owed a report for 1 message:\n\nfrom "+sender.name+" · task · ") &&
		strings.Contains(plain, owedTaskText) && !strings.Contains(plain, task.ID)
	out := []telemetryFinding{finding(obsOwedShown, shown, "the task read was %s; --owed printed %q", task.ID, raw)}

	var reports []reportView
	found := waitFor(c, 30*time.Second, func() bool {
		reports = nil
		for _, message := range readMessages(sender) {
			if message.From == worker.name && message.Kind == "finished" && slices.Contains(message.InReplyTo, task.ID) {
				reports = append(reports, message)
			}
		}
		return len(reports) > 0
	})
	settled := found && waitFor(c, 5*time.Second, func() bool { return len(awaiting(iso, worker)) == 0 })
	// A second report would come right behind the first; give it a moment.
	time.Sleep(500 * time.Millisecond)
	count := 0
	for _, message := range readMessages(sender) {
		if message.From == worker.name && slices.Contains(message.InReplyTo, task.ID) {
			count++
		}
	}
	return append(out, finding(obsOwedReported, found && settled && count == 1,
		"a finished report on %s: %v; settled: %v; messages about it: %d; kinds read by the sender: %v",
		task.ID, found, settled, count, kindsFrom(sender, worker)))
}

// The control: --owed that finds nothing, which a re-read after a compaction
// would take for "no task".
var mutantOwedEmpty = mutation{
	name:  "owed-empty",
	file:  "internal/inbox/owed.go",
	edits: []edit{{"\tvar readings []reading\n", "\tvar readings []reading\n\tif name != \"\" {\n\t\treturn nil, nil\n\t}\n"}},
}

func TestAnEmptyOwedFails(t *testing.T) {
	runInColumns(t, "owed-reread-control-"+mutantOwedEmpty.name, func(t *testing.T, col column) {
		name := "owed-reread-control-" + mutantOwedEmpty.name
		enterScenario(t, name)
		want := "the " + mutantOwedEmpty.name + " mutant breaks " + obsOwedShown + ", and nothing else"
		c := Start(t, Spec{Name: name, Harness: col.harness, Observations: []string{want}, Deadline: 150 * time.Second})
		binary, err := buildMutant(c, mutantOwedEmpty)
		if err != nil {
			c.Contradicted(want, "the mutant could not be built: %v", err)
			return
		}
		var wrong []string
		for _, finding := range playOwedReread(t, c, Isolate(t, c, binary), col) {
			switch {
			case !finding.judged:
				wrong = append(wrong, "could not judge "+finding.observation+": "+finding.detail)
			case finding.observation == obsOwedShown && finding.held:
				wrong = append(wrong, finding.observation+" held anyway: "+finding.detail)
			case finding.observation != obsOwedShown && !finding.held:
				wrong = append(wrong, finding.observation+" broke too: "+finding.detail)
			}
		}
		if len(wrong) > 0 {
			c.Contradicted(want, "%s", strings.Join(wrong, "; "))
			return
		}
		c.Observed(want, obsOwedShown)
	})
}

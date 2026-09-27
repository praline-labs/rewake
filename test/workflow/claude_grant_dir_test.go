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

// TestClaudeGrantDir is a directory granted with a task to a Claude Code
// worker, which takes it through its permission hooks
// (docs/grants.md#claude-code). Main sends the task with `--grant-dir`; the
// worker writes in the directory, in its Git metadata and outside it, and
// reports; main sends the next task, in which the worker writes there again
// and reads its own directory.
//
//   - A command inside the grant goes to the person until a file tool writes
//     there; that write is allowed by the PermissionRequest hook, which adds
//     the directory for the session, and a command inside it then runs unasked.
//   - A write into `.git` inside the grant, and a write outside it, go to the
//     person, even once the grant is a working directory.
//   - After the report a write into the directory is denied, a command is
//     asked about and goes to the person, the next read is answered with the
//     directory's removal, a write after that goes to the person, and
//     `rewake list --json` shows the grant revoked.
//
// What it does not prove: what the real harness does with the answers. That
// was seen live on 2.1.280 (docs/research-claude-actions.md); the fixture
// keeps its working directories the way the harness was seen to, and the case
// reads what rewake's hook answered it.
func TestClaudeGrantDir(t *testing.T) {
	binary := enterScenario(t, "claude-grant-dir")
	c := Start(t, Spec{
		Name:         "claude-grant-dir",
		Harness:      claudeColumn.harness,
		Observations: claudeGrantObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playClaudeGrantDir(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsClaudeGrantGiven     = "a command in the granted directory goes to the person until a file tool writes there, which the permission hook allows, adding the directory for the session, and a command in it then runs unasked"
	obsClaudeGrantShielded  = "a write into the Git metadata inside the grant, and a write outside the grant, go to the person"
	obsClaudeGrantTakenBack = "after the report a write into the directory is denied, a command is asked about and goes to the person, the next read is answered with its removal, a write after that goes to the person, and the listing shows it revoked"
	claudeGrantTask         = "claude-grant-task: write in the granted directory"
	claudeGrantAfter        = "claude-grant-after: the task after the report"
)

var claudeGrantObservations = []string{obsClaudeGrantGiven, obsClaudeGrantShielded, obsClaudeGrantTakenBack}

func playClaudeGrantDir(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := claudeColumn.harness
	granted, grantErr := grantableDir(t)
	if grantErr != nil {
		return unjudgedAll(claudeGrantObservations, "cannot create the granted directory: %v", grantErr)
	}
	// Beside the grant, so a write there differs from one inside it by the
	// grant alone.
	other := filepath.Join(filepath.Dir(granted), "other")
	if err := os.MkdirAll(filepath.Join(granted, ".git"), 0o700); err != nil {
		return unjudgedAll(claudeGrantObservations, "cannot create the grant's metadata: %v", err)
	}
	if err := os.Mkdir(other, 0o700); err != nil {
		return unjudgedAll(claudeGrantObservations, "cannot create the directory beside the grant: %v", err)
	}
	worker := startHarnessSession(t, c, iso, col, "worker", "--write", shimTools+"=1", staysUp(iso, "worker"))
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", asks.env())
	defer stopSession(t, c, lead)
	sg := steering{c: c, asks: asks, lead: lead}

	calls := func(lines ...string) string { return strings.Join(lines, "\n") }
	early, written, run := filepath.Join(granted, "early.txt"), filepath.Join(granted, "a.txt"), filepath.Join(granted, "b.txt")
	git, outside := filepath.Join(granted, ".git", "config"), filepath.Join(other, "c.txt")
	code, sent, _ := asks.ask(c, "send", worker.name, "--grant-dir", granted, "--json", calls(claudeGrantTask,
		"tool Bash "+early, "tool Write "+written, "tool Bash "+run, "tool Write "+git, "tool Write "+outside))
	var view struct {
		ID        string   `json:"id"`
		GrantDirs []string `json:"grantDirs"`
	}
	if (code != 0 && code != 3) || json.Unmarshal([]byte(sent), &view) != nil || !slices.Equal(view.GrantDirs, []string{granted}) {
		return unjudgedAll(claudeGrantObservations, "the grant was not sent: exit %d, %q", code, sent)
	}
	task := view.ID
	if !waitFor(c, 30*time.Second, func() bool { return reportedOn(c, asks, task) }) {
		events, _ := worker.turnEvents()
		return unjudgedAll(claudeGrantObservations, "the worker did not finish the granted task %s, its status %q: %v", task, statusState(iso, worker, task), events)
	}
	denied, asked, again := filepath.Join(granted, "d.txt"), filepath.Join(granted, "f.txt"), filepath.Join(granted, "e.txt")
	_, sent, _ = asks.ask(c, "send", worker.name, calls(claudeGrantAfter,
		"tool Write "+denied, "tool Bash "+asked, "tool Read README", "tool Write "+again))
	after := printedID(sent)
	var events []turnEvent
	tools := map[string]string{}
	waitFor(c, 30*time.Second, func() bool {
		events, _ = worker.turnEvents()
		tools = toolOutcomes(events)
		return tools["Write "+again] != "" && reportedOn(c, asks, after)
	})
	record := fmt.Sprintf("the worker's calls %v; its log %+v", tools, events)

	var out []telemetryFinding
	out = append(out, judged(obsClaudeGrantGiven,
		tools["Bash "+early] == "prompted" && tools["Write "+written] == "ran +"+granted && tools["Bash "+run] == "ran", "%s", record))
	out = append(out, judged(obsClaudeGrantShielded,
		tools["Write "+git] == "prompted" && tools["Write "+outside] == "prompted", "%s", record))
	outcome := journaledOutcome(sg, worker, granted)
	taken := tools["Write "+denied] == "denied" && tools["Bash "+asked] == "prompted" && tools["Read README"] == "ran -"+granted &&
		tools["Write "+again] == "prompted" && outcome == "revoked"
	return append(out, judged(obsClaudeGrantTakenBack, taken, "the journal says %q; %s", outcome, record))
}

// toolOutcomes are the worker's tool calls by tool and path, each with what
// came of it (claudeshim_tools_test.go).
func toolOutcomes(events []turnEvent) map[string]string {
	outcomes := map[string]string{}
	for _, event := range allOf(events, "tool") {
		fields := strings.SplitN(event.Detail, " ", 3)
		if len(fields) == 3 {
			outcomes[fields[0]+" "+fields[1]] = fields[2]
		}
	}
	return outcomes
}

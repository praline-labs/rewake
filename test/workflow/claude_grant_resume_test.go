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

// TestClaudeGrantResume is a directory granted to a Claude Code worker whose
// run ends while the task is open, and a cold resume of its conversation
// (docs/grants.md#after-a-cold-resume). The worker writes in the grant, marks
// its turn pending and is stopped; it is started again with --resume; main
// sends a task in which it runs a command in the directory, and after the
// report of both, one in which it writes there again and reads its own
// directory. Then it is stopped and resumed once more.
//
//   - The resumed run is started with the directory, confirmed again by main:
//     a command in it runs unasked.
//   - The resumed run reports on the task, and the grant is taken back as any
//     other: a write is denied, the next read takes the directory out, and the
//     listing shows it revoked.
//   - A resume after the report is started without the directory: main no
//     longer holds a grant whose task is closed, though a copy on disk still
//     names it live.
//
// What it does not prove: what the real harness does with --add-dir beside
// --resume, seen live on 2.1.280 (docs/research-claude-actions.md), and that
// a real conversation keeps its id across a resume, which the fixture assumes.
func TestClaudeGrantResume(t *testing.T) {
	binary := enterScenario(t, "claude-grant-resume")
	c := Start(t, Spec{
		Name:         "claude-grant-resume",
		Harness:      claudeColumn.harness,
		Observations: claudeResumeObservations,
		Deadline:     120 * time.Second,
	})
	for _, finding := range playClaudeGrantResume(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsClaudeResumeGiven     = "a run resuming the conversation of an open task is started with its granted directory, confirmed again by main, and a command in it runs unasked"
	obsClaudeResumeTakenBack = "the resumed run reports on the task, and then a write in the directory is denied, the next read takes it out, and the listing shows it revoked"
	obsClaudeResumeClosed    = "a run resuming the conversation after the report is started without the directory"
	claudeResumeTask         = "claude-resume-task: write in the granted directory, then wait"
	claudeResumeNext         = "claude-resume-next: a command in the directory after the resume"
	claudeResumeAfter        = "claude-resume-after: the task after the report"
	claudeResumePending      = "stopped before the report"
)

var claudeResumeObservations = []string{obsClaudeResumeGiven, obsClaudeResumeTakenBack, obsClaudeResumeClosed}

func playClaudeGrantResume(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := claudeColumn.harness
	granted, grantErr := grantableDir(t)
	if grantErr != nil {
		return unjudgedAll(claudeResumeObservations, "cannot create the granted directory: %v", grantErr)
	}
	worker := startHarnessSession(t, c, iso, col, "worker", "--write", shimTools+"=1", shimPendingOnce+"="+claudeResumePending, staysUp(iso, "worker"))
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", asks.env())
	defer stopSession(t, c, lead)
	sg := steering{c: c, asks: asks, lead: lead}
	calls := func(lines ...string) string { return strings.Join(lines, "\n") }

	written := filepath.Join(granted, "a.txt")
	code, sent, _ := asks.ask(c, "send", worker.name, "--grant-dir", granted, "--json", calls(claudeResumeTask, "tool Write "+written))
	var view struct {
		ID        string   `json:"id"`
		GrantDirs []string `json:"grantDirs"`
	}
	if (code != 0 && code != 3) || json.Unmarshal([]byte(sent), &view) != nil || !slices.Equal(view.GrantDirs, []string{granted}) {
		stopSession(t, c, worker)
		return unjudgedAll(claudeResumeObservations, "the grant was not sent: exit %d, %q", code, sent)
	}
	task := view.ID
	// The turn that marked itself pending has ended: its line follows the mark.
	if !waitFor(c, 30*time.Second, func() bool {
		turns := worker.acceptedTurns()
		at := strings.Index(turns, "pending ok\n")
		return at >= 0 && strings.TrimSpace(turns[at+len("pending ok\n"):]) != ""
	}) {
		stopSession(t, c, worker)
		return unjudgedAll(claudeResumeObservations, "the worker did not end its granted turn pending: %q", worker.acceptedTurns())
	}
	events, _ := worker.turnEvents()
	if got := toolOutcomes(events)["Write "+written]; got != "ran +"+granted {
		stopSession(t, c, worker)
		return unjudgedAll(claudeResumeObservations, "the grant was not taken before the resume: the write %q", got)
	}
	stopSession(t, c, worker)

	worker = restartSession(t, c, iso, worker, col, "worker", "--write", []string{"--resume", worker.name}, shimTools+"=1", staysUp(iso, "worker"))
	run := filepath.Join(granted, "b.txt")
	_, sent, _ = asks.ask(c, "send", worker.name, calls(claudeResumeNext, "tool Bash "+run))
	next := printedID(sent)
	tools := map[string]string{}
	waitFor(c, 30*time.Second, func() bool {
		events, _ = worker.turnEvents()
		tools = toolOutcomes(events)
		return tools["Bash "+run] != "" && reportedOn(c, asks, next)
	})
	launch := launchedWith(worker.acceptedTurns())
	var out []telemetryFinding
	out = append(out, judged(obsClaudeResumeGiven, launch == "resume="+worker.name+" add-dir="+granted && tools["Bash "+run] == "ran",
		"launched %q; the command %q", launch, tools["Bash "+run]))

	reported := waitFor(c, 30*time.Second, func() bool { return reportedOn(c, asks, task) })
	denied := filepath.Join(granted, "d.txt")
	_, sent, _ = asks.ask(c, "send", worker.name, calls(claudeResumeAfter, "tool Write "+denied, "tool Read README"))
	after := printedID(sent)
	waitFor(c, 30*time.Second, func() bool {
		events, _ = worker.turnEvents()
		tools = toolOutcomes(events)
		return tools["Read README"] != "" && reportedOn(c, asks, after)
	})
	outcome := journaledOutcome(sg, worker, granted)
	out = append(out, judged(obsClaudeResumeTakenBack,
		reported && tools["Write "+denied] == "denied" && tools["Read README"] == "ran -"+granted && outcome == "revoked",
		"the task reported on: %v; the write %q, the read %q; the journal says %q", reported, tools["Write "+denied], tools["Read README"], outcome))
	stopSession(t, c, worker)

	worker = restartSession(t, c, iso, worker, col, "worker", "--write", []string{"--resume", worker.name}, shimTools+"=1", staysUp(iso, "worker"))
	defer stopSession(t, c, worker)
	var again string
	waitFor(c, 30*time.Second, func() bool {
		again = launchedWith(worker.acceptedTurns())
		return strings.Count(worker.acceptedTurns(), "launched ") >= 2
	})
	return append(out, judged(obsClaudeResumeClosed, again == "resume="+worker.name+" add-dir=",
		"the third run was launched with %q", again))
}

// launchedWith is what the last launch of a session said of its resume.
func launchedWith(turns string) string {
	lines := strings.Split(turns, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if rest, ok := strings.CutPrefix(lines[index], "launched "); ok {
			return rest
		}
	}
	return ""
}

// restartSession starts a session again under the name it had, with harness
// arguments, once the earlier run has ended, and waits for it to come up: the
// Claude Code fixture mounted, the Codex one holding its conversation. The
// files the earlier run left that would stop or announce the new one are
// cleared first.
func restartSession(t *testing.T, c *Case, iso *Isolation, earlier *codexSession, harness, name, role string, harnessArgs []string, controls ...string) *codexSession {
	t.Helper()
	up := earlier.ready + ".mounted"
	if harness == codexColumn.harness {
		up = earlier.accepted
	}
	for _, path := range []string{earlier.exitFile, earlier.ready, up} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatalf("clearing %s before the restart: %v", path, err)
		}
	}
	session := startSessionWith(t, c, iso, harness, name, role, nil, harnessArgs, controls...)
	if !waitFor(c, 30*time.Second, func() bool { _, err := os.Stat(up); return err == nil }) {
		t.Fatalf("%s did not come up again: %s", name, fmt.Sprint(session.acceptedTurns()))
	}
	return session
}

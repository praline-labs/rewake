package workflow

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestCodexGrantResume is a directory granted to a Codex worker on the first
// turn of its conversation, a run that ends while the task is open, and cold
// resumes of that conversation (docs/grants.md#after-a-cold-resume). The
// fixture's server keeps the thread's roots between runs the way the real one
// was seen to: those a start names and those of every completed turn after
// the first, restored by a resume that names none — so the grant, given on
// the first turn, is lost by the resume.
//
//   - The first delivery after the resume names the directory among the roots
//     again, confirmed again by main, and the listing shows it granted.
//   - The resumed run reports on the task, and the first delivery after that
//     sends the roots without the directory; the listing shows it revoked.
//   - A resume after the report does not bring it back: the first delivery
//     after it names no roots with the directory, and the run journals nothing
//     of it, though a copy on disk still names it live. A grant main confirmed
//     wrongly would be journaled, and taken out again in the same delivery by
//     the report the run finds.
//
// What it does not prove: that the real server restores a root the way the
// fixture keeps it beyond what the probe of September 26, 2026 saw
// (docs/research-codex.md#runtime-workspace-roots).
func TestCodexGrantResume(t *testing.T) {
	binary := enterScenario(t, "codex-grant-resume")
	c := Start(t, Spec{
		Name:         "codex-grant-resume",
		Harness:      codexColumn.harness,
		Observations: codexResumeObservations,
		Deadline:     120 * time.Second,
	})
	for _, finding := range playCodexGrantResume(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsCodexResumeGiven     = "the first delivery after a resume of the conversation of an open task names its granted directory among the roots again, confirmed again by main, and the listing shows it granted"
	obsCodexResumeTakenBack = "the resumed run reports on the task, and the first delivery after that sends the roots without the directory, and the listing shows it revoked"
	obsCodexResumeClosed    = "after a resume past the report, the first delivery names no roots with the directory, and the resumed run journals nothing of it"
	codexResumeTask         = "codex-resume-task: write in the granted directory, then wait"
	codexResumeNext         = "codex-resume-next: the task after the resume"
	codexResumeAfter        = "codex-resume-after: the task after the report"
	codexResumeLast         = "codex-resume-last: the task after the second resume"
	codexResumePending      = "stopped before the report"
)

var codexResumeObservations = []string{obsCodexResumeGiven, obsCodexResumeTakenBack, obsCodexResumeClosed}

func playCodexGrantResume(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := codexColumn.harness
	granted, grantErr := grantableDir(t)
	if grantErr != nil {
		return unjudgedAll(codexResumeObservations, "cannot create the granted directory: %v", grantErr)
	}
	store := shimThreadStore + "=" + filepath.Join(iso.Home, "worker.thread.json")
	// A resumed terminal sends no roots, as 0.157.1's does, so the server
	// restores what it saved.
	resumed := []string{shimResume + "=1", shimTUIShape + "=0.157.1"}
	worker := startHarnessSession(t, c, iso, col, "worker", "--write", shimInboxJSON+"=1", shimPendingOnce+"="+codexResumePending, store, staysUp(iso, "worker"))
	asks := newRequests(iso, "lead")
	// Main runs Claude Code: a Codex main cannot grant (docs/grants.md#who-can-grant).
	lead := startHarnessSession(t, c, iso, claudeColumn.harness, "lead", "--main", asks.env())
	defer stopSession(t, c, lead)
	sg := steering{c: c, asks: asks, lead: lead}

	code, sent, _ := asks.ask(c, "send", worker.name, "--grant-dir", granted, codexResumeTask, "--json")
	var view struct {
		ID        string   `json:"id"`
		GrantDirs []string `json:"grantDirs"`
	}
	if (code != 0 && code != 3) || json.Unmarshal([]byte(sent), &view) != nil || !slices.Equal(view.GrantDirs, []string{granted}) {
		stopSession(t, c, worker)
		return unjudgedAll(codexResumeObservations, "the grant was not sent: exit %d, %q", code, sent)
	}
	task := view.ID
	var events []turnEvent
	if !waitFor(c, 30*time.Second, func() bool {
		events, _ = worker.turnEvents()
		return rootsFor(events, task) != nil && eventOf(events, "completed") != nil
	}) {
		stopSession(t, c, worker)
		return unjudgedAll(codexResumeObservations, "the worker did not work the granted task: %v", events)
	}
	if carried := rootsFor(events, task); !slices.Contains(carriedRoots(carried), granted) {
		stopSession(t, c, worker)
		return unjudgedAll(codexResumeObservations, "the grant did not reach the first turn: %+v", carried)
	}
	stopSession(t, c, worker)

	worker = restartSession(t, c, iso, worker, col, "worker", "--write", nil, append([]string{shimInboxJSON + "=1", store, staysUp(iso, "worker")}, resumed...)...)
	_, sent, _ = asks.ask(c, "send", worker.name, codexResumeNext)
	next := printedID(sent)
	waitFor(c, 30*time.Second, func() bool { return reportedOn(c, asks, next) })
	events, _ = worker.turnEvents()
	restored := rootsFor(events, next)
	listed := journaledOutcome(sg, worker, granted)
	var out []telemetryFinding
	out = append(out, judged(obsCodexResumeGiven, restored != nil && slices.Contains(carriedRoots(restored), granted) && listed == "granted",
		"the delivery after the resume carried %+v; the journal says %q", restored, listed))

	reported := waitFor(c, 30*time.Second, func() bool { return reportedOn(c, asks, task) })
	_, sent, _ = asks.ask(c, "send", worker.name, codexResumeAfter)
	after := printedID(sent)
	waitFor(c, 20*time.Second, func() bool { return arrived(statusState(iso, worker, after)) })
	events, _ = worker.turnEvents()
	dropped := rootsFor(events, after)
	outcome := journaledOutcome(sg, worker, granted)
	out = append(out, judged(obsCodexResumeTakenBack,
		reported && dropped != nil && !slices.Contains(carriedRoots(dropped), granted) && outcome == "revoked",
		"the task reported on: %v; the delivery after the report carried %+v; the journal says %q", reported, dropped, outcome))
	waitFor(c, 20*time.Second, func() bool { return reportedOn(c, asks, after) })
	stopSession(t, c, worker)

	worker = restartSession(t, c, iso, worker, col, "worker", "--write", nil, append([]string{shimInboxJSON + "=1", store, staysUp(iso, "worker")}, resumed...)...)
	defer stopSession(t, c, worker)
	_, sent, _ = asks.ask(c, "send", worker.name, codexResumeLast)
	last := printedID(sent)
	delivered := waitFor(c, 20*time.Second, func() bool { return arrived(statusState(iso, worker, last)) })
	events, _ = worker.turnEvents()
	again := rootsFor(events, last)
	journaled := journaledOutcome(sg, worker, granted)
	return append(out, judged(obsCodexResumeClosed, delivered && (again == nil || !slices.Contains(carriedRoots(again), granted)) && journaled == "not journaled",
		"delivered: %v; the delivery after the second resume carried %+v; the journal says %q", delivered, again, journaled))
}

// arrived says a message reached its session: delivered, or read already.
func arrived(status string) bool { return status == "delivered" || status == "read" }

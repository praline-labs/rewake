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

// TestCodexGrantDir is a directory granted with a task to a Codex worker that
// is busy when the task arrives: main sends `--grant-dir` during the worker's
// turn, the worker reports on it, and main sends the next task.
//
//   - The grant waits for the turn to end and goes on a turn of its own, never
//     steered into the running one: a directory added in the middle of a turn
//     would hold only from the turn after (docs/grants.md).
//   - The turn that carries it names the directory among the workspace roots,
//     beside the roots the thread already had.
//   - The first delivery after the report sends the roots without it, and
//     `rewake list --json` shows it taken back.
//
// What it does not prove: that a command on the real server can write where
// the roots say. What was seen live on 0.155.1 and 0.157.1 is the server's
// policy — the snapshot names the added root, and the persisted permission
// profile carries it — while the write itself was never shown, the container
// keeping restricted execution from running
// (docs/research-codex.md#runtime-workspace-roots). Here the fixture keeps
// the roots the way the server was seen to, and the case reads what rewake
// sent it.
func TestCodexGrantDir(t *testing.T) {
	binary := enterScenario(t, "codex-grant-dir")
	c := Start(t, Spec{
		Name:         "codex-grant-dir",
		Harness:      codexColumn.harness,
		Observations: grantDirObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playGrantDir(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsGrantWaitsIdle  = "a task granting a directory sent during a turn waits for it to end and starts a turn of its own"
	obsGrantReachRoots = "the turn carrying a directory grant names the directory among the workspace roots, beside the thread's own"
	obsGrantTakenBack  = "the first delivery after the granted task is reported on sends the roots without the directory, and the listing shows it revoked"
	grantDirBusy       = "grant-dir-busy: a turn in progress"
	grantDirTask       = "grant-dir-task: write in the granted directory"
	grantDirAfter      = "grant-dir-after: the task after the report"
)

var grantDirObservations = []string{obsGrantWaitsIdle, obsGrantReachRoots, obsGrantTakenBack}

func playGrantDir(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := codexColumn.harness
	granted, grantErr := grantableDir(t)
	if grantErr != nil {
		return unjudgedAll(grantDirObservations, "cannot create the granted directory: %v", grantErr)
	}
	worker := startHarnessSession(t, c, iso, col, "worker", "--write", shimInboxJSON+"=1", shimHoldTurn+"=1", staysUp(iso, "worker"))
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "lead")
	// Main runs Claude Code: a Codex main cannot grant, its sandbox does not
	// reach its wrapper (docs/grants.md#who-can-grant).
	lead := startHarnessSession(t, c, iso, claudeColumn.harness, "lead", "--main", asks.env())
	defer stopSession(t, c, lead)
	sg := steering{c: c, asks: asks, lead: lead}

	_, sent, _ := asks.ask(c, "send", worker.name, grantDirBusy)
	busy := printedID(sent)
	var events []turnEvent
	if !waitFor(c, 15*time.Second, func() bool {
		events, _ = worker.turnEvents()
		return eventOf(events, "operation-open") != nil
	}) {
		return unjudgedAll(grantDirObservations, "the worker never held its first turn open for %s: %v", busy, events)
	}
	code, sent, _ := asks.ask(c, "send", worker.name, "--grant-dir", granted, grantDirTask, "--json")
	var view struct {
		ID        string   `json:"id"`
		GrantDirs []string `json:"grantDirs"`
	}
	// Exit 3 is the wait itself: accepted, and not delivered while the turn
	// runs.
	if (code != 0 && code != 3) || json.Unmarshal([]byte(sent), &view) != nil || !slices.Equal(view.GrantDirs, []string{granted}) {
		return unjudgedAll(grantDirObservations, "the grant was not sent: exit %d, %q", code, sent)
	}
	task := view.ID
	if !waitFor(c, 30*time.Second, func() bool { return reportedOn(c, asks, task) }) {
		events, _ = worker.turnEvents()
		return unjudgedAll(grantDirObservations, "the worker did not finish the granted task %s, its status %q: %v", task, statusState(iso, worker, task), events)
	}
	_, sent, _ = asks.ask(c, "send", worker.name, grantDirAfter)
	after := printedID(sent)
	// Delivered is when its roots, if it carried any, are on record: a
	// delivery that carried none records nothing to wait for.
	waitFor(c, 20*time.Second, func() bool { return statusState(iso, worker, after) == "delivered" })
	events, err := worker.turnEvents()
	record := fmt.Sprintf("the worker's log %+v %v", events, err)

	// The turn the grant reached is read from the deliveries, not from its
	// roots: a grant that went without them still went somewhere.
	var out []telemetryFinding
	deliveries, groupErr := worker.groupDeliveries()
	reachedTurn := ""
	for _, delivery := range deliveries {
		if slices.Contains(delivery.Members, task) {
			reachedTurn = delivery.Turn
		}
	}
	held := eventOf(events, "operation-open")
	steered := slices.ContainsFunc(allOf(events, "steered"), func(event turnEvent) bool { return event.Detail == task })
	waited := held != nil && reachedTurn != "" && reachedTurn != held.Turn && !steered
	out = append(out, judged(obsGrantWaitsIdle, err == nil && groupErr == nil && waited,
		"the grant reached turn %q, the held one was %+v, steered: %v; deliveries %+v %v; %s", reachedTurn, held, steered, deliveries, groupErr, record))

	carried := rootsFor(events, task)

	reached := carried != nil && slices.Contains(carriedRoots(carried), granted) && slices.Contains(carriedRoots(carried), "/work")
	out = append(out, judged(obsGrantReachRoots, reached, "%s", record))

	next := rootsFor(events, after)
	outcome := journaledOutcome(sg, worker, granted)
	taken := next != nil && slices.Contains(carriedRoots(next), "/work") &&
		!slices.Contains(carriedRoots(next), granted) && outcome == "revoked"
	return append(out, judged(obsGrantTakenBack, taken, "the journal says %q; %s", outcome, record))
}

// rootsFor is the roots event of the delivery that named one message.
func rootsFor(events []turnEvent, message string) *turnEvent {
	for index, event := range events {
		if event.Kind == "roots" && strings.HasPrefix(event.Detail, message+" ") {
			return &events[index]
		}
	}
	return nil
}

// carriedRoots are the roots a roots event records, the fixture's own join.
func carriedRoots(event *turnEvent) []string {
	_, joined, _ := strings.Cut(event.Detail, " ")
	return strings.Split(joined, ":")
}

// journaledOutcome is what main's listing says became of a granted directory
// on the worker.
func journaledOutcome(sg steering, worker *codexSession, directory string) string {
	code, machine, ok := sg.asks.ask(sg.c, "list", "--json")
	var listed struct {
		Sessions []struct {
			Name   string `json:"name"`
			Grants []struct {
				Path    string `json:"path"`
				Outcome string `json:"outcome"`
			} `json:"grants"`
		} `json:"sessions"`
	}
	if !ok || code != 0 || json.Unmarshal([]byte(machine), &listed) != nil {
		return fmt.Sprintf("the listing did not come back: exit %d, %q", code, firstLine(machine))
	}
	for _, entry := range listed.Sessions {
		if entry.Name != worker.name {
			continue
		}
		for _, granted := range entry.Grants {
			if granted.Path == directory {
				return granted.Outcome
			}
		}
	}
	return "not journaled"
}

// grantableDir makes a directory a grant may name: outside the worker's
// workspace, not directly in HOME, and not in a temporary directory, which
// the hard tier refuses (docs/grants.md#the-hard-tier) and where every case's
// own directories lie. So it goes in the user's cache, beside the harness
// versions the suite keeps there, and is removed with the case.
func grantableDir(t *testing.T) (string, error) {
	t.Helper()
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(cache, "rewake", "workflow-grants")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(base, "case-")
	if err != nil {
		return "", err
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	granted := filepath.Join(dir, "lib")
	if err := os.Mkdir(granted, 0o700); err != nil {
		return "", err
	}
	// Resolved, as rewake resolves it: the case compares the path it sent
	// with the roots the fixture received.
	return filepath.EvalSymlinks(granted)
}

// reportedOn says whether main no longer waits on a task: its report came.
// Asked of main's own listing, since a Claude Code main keeps no record of
// what it read that the case could look at.
func reportedOn(c *Case, asks *requests, id string) bool {
	code, machine, ok := asks.ask(c, "inbox", "--awaited", "--json")
	var awaited struct {
		Recipients []struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
		} `json:"recipients"`
	}
	if !ok || code != 0 || json.Unmarshal([]byte(machine), &awaited) != nil {
		return false
	}
	for _, recipient := range awaited.Recipients {
		for _, message := range recipient.Messages {
			if message.ID == id {
				return false
			}
		}
	}
	return true
}

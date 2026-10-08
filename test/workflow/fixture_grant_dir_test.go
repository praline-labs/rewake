package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// TestFixtureGrantDir is the core's part of a directory granted with a task,
// on the gate column: main registers the grant with its wrapper and sends the
// letter, the worker's wrapper checks it again and has main's confirm it before
// the program is offered it, and main's wrapper holds it while the task is open
// (docs/grants.md).
//
//   - A task main sends with --grant-dir reaches the worker with the directory
//     offered beside that letter; a plain task steered into the same turn
//     offers none.
//   - Main's wrapper confirms the grant while the task is open and has
//     forgotten it once the task is reported on.
//   - A letter whose delivery waits — the program refuses reservations until
//     the scenario lets it take one — is checked again at each attempt: its
//     directory removed meanwhile, it fails before the program is offered it,
//     and main is told why.
//
// What it does not prove: anything an adapter does with a grant. Waiting for
// an idle session, adding the directory to what the harness may write and
// taking it out after the report are the Permissions capability's
// (docs/v2/design-api.md#permissions); on Claude Code its hooks and the keeper
// hold them in claude-grant-dir. The fixture only records what it was offered.
func TestFixtureGrantDir(t *testing.T) {
	binary := enterScenario(t, "fixture-grant-dir")
	c := Start(t, Spec{
		Name:         "fixture-grant-dir",
		Harness:      fixtureColumn.harness,
		Observations: fixtureGrantObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playFixtureGrantDir(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsFixtureGrantOffered   = "a task main sends with --grant-dir is delivered with the directory offered beside that letter, and a plain task steered into the same turn offers none"
	obsFixtureGrantLifetime  = "main's wrapper confirms the grant while its task is open, and has forgotten it once the task is reported on"
	obsFixtureGrantRechecked = "a granted letter whose delivery waits is checked again when it is tried again: its directory gone, it fails before the session is offered it, and main is told why"
	fixtureGrantTask         = "fixture-grant-task: write in the granted directory"
	fixtureGrantSteered      = "fixture-grant-steered: the task beside it"
	fixtureGrantHeld         = "fixture-grant-held: a grant whose directory goes"
)

var fixtureGrantObservations = []string{obsFixtureGrantOffered, obsFixtureGrantLifetime, obsFixtureGrantRechecked}

func playFixtureGrantDir(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := fixtureColumn.harness
	granted, err := grantableDir(t)
	if err != nil {
		return unjudgedAll(fixtureGrantObservations, "cannot create the granted directory: %v", err)
	}
	doomed, err := grantableDir(t)
	if err != nil {
		return unjudgedAll(fixtureGrantObservations, "cannot create the directory that goes: %v", err)
	}
	release := filepath.Join(iso.Home, "held.release")
	worker := startHarnessSession(t, c, iso, col, "worker", "--write", shimInboxJSON+"=1", shimHoldTurn+"=1", staysUp(iso, "worker"))
	defer stopSession(t, c, worker)
	held := startHarnessSession(t, c, iso, col, "held", "--write", shimInboxJSON+"=1", shimHoldReserve+"="+release, staysUp(iso, "held"))
	defer stopSession(t, c, held)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)

	room := filepath.Join(iso.StateDir, "rooms", "default")
	var leadRun, workerRun string
	if !waitFor(c, 20*time.Second, func() bool {
		leadRecord, leadErr := registry.Load(room, lead.name)
		workerRecord, workerErr := registry.Load(room, worker.name)
		if leadErr != nil || workerErr != nil || leadRecord.ServicePID == 0 || workerRecord.ServicePID == 0 {
			return false
		}
		leadRun, workerRun = leadRecord.Epoch(), workerRecord.Epoch()
		return true
	}) {
		return unjudgedAll(fixtureGrantObservations, "the sessions were never registered")
	}
	pid, start, _ := registry.ParseEpoch(leadRun)
	// Asked from this process, which is not the worker's run: the answer
	// says what main's wrapper holds and marks nothing delivered.
	holds := func(id string) (bool, string) {
		confirmed, err := grantauth.Confirm(state.AuthorityAddress(room, leadRun), grantauth.Expect{PID: pid, Start: start}, id, worker.name, workerRun, "")
		if err != nil {
			return false, err.Error()
		}
		return slices.Equal(confirmed.Dirs, []string{granted}), strings.Join(confirmed.Dirs, " ")
	}

	// A letter written is judged whatever send's exit said of its delivery:
	// a grant refused on delivery is a finding, not a case that never ran.
	sendGranted := func(to *scenarioSession, directory, text string) (string, int, string) {
		code, sent, _ := asks.ask(c, "send", to.name, "--grant-dir", directory, "--json", text)
		var view struct {
			ID        string   `json:"id"`
			GrantDirs []string `json:"grantDirs"`
		}
		if json.Unmarshal([]byte(sent), &view) != nil || view.ID == "" || !slices.Equal(view.GrantDirs, []string{directory}) {
			return "", code, sent
		}
		return view.ID, code, sent
	}
	task, code, sent := sendGranted(worker, granted, fixtureGrantTask)
	if task == "" {
		return unjudgedAll(fixtureGrantObservations, "the grant was not sent: exit %d, %q", code, sent)
	}
	var events []turnEvent
	waitFor(c, 30*time.Second, func() bool {
		events, _ = worker.turnEvents()
		return eventOf(events, grantEventKind) != nil
	})
	heldOpen, openDetail := holds(task)
	_, sent, _ = asks.ask(c, "send", worker.name, fixtureGrantSteered)
	steered := printedID(sent)
	reported := waitFor(c, 30*time.Second, func() bool { return reportedOn(c, asks, task) && reportedOn(c, asks, steered) })
	events, eventsErr := worker.turnEvents()
	offers := allOf(events, grantEventKind)
	var out []telemetryFinding
	out = append(out, judged(obsFixtureGrantOffered,
		(code == 0 || code == 3) && eventsErr == nil && len(offers) == 1 && offers[0].Turn == task && offers[0].Detail == granted &&
			steered != "" && slices.Contains(worker.deliveredIDs(), steered),
		"the task %s sent with exit %d, the steered task %s; offered %+v; delivered %v; the log's error %v", task, code, steered, offers, worker.deliveredIDs(), eventsErr))
	// Main forgets a closed task's grant when it is next asked.
	heldAfter, afterDetail := holds(task)
	out = append(out, judged(obsFixtureGrantLifetime, heldOpen && reported && !heldAfter,
		"while open: %v (%s); reported on: %v; after the report: %v (%s)", heldOpen, openDetail, reported, heldAfter, afterDetail))

	return append(out, rechecked(c, iso, lead, held, doomed, release, sendGranted))
}

// rechecked is the letter whose delivery the program holds back while its
// directory is taken away.
func rechecked(c *Case, iso *Isolation, lead, held *scenarioSession, doomed, release string,
	sendGranted func(*scenarioSession, string, string) (string, int, string),
) telemetryFinding {
	id, code, sent := sendGranted(held, doomed, fixtureGrantHeld)
	if id == "" {
		return judged(obsFixtureGrantRechecked, false, "the grant was not sent: exit %d, %q", code, sent)
	}
	waiting := waitFor(c, 20*time.Second, func() bool { return statusState(iso, held, id) == "pending" })
	removeErr := os.RemoveAll(doomed)
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		return judged(obsFixtureGrantRechecked, false, "cannot let the program take a reservation: %v", err)
	}
	failed := waitFor(c, 20*time.Second, func() bool { return statusState(iso, held, id) == "failed" })
	detail := statusDetail(iso, held, id)
	var told reportView
	toldOK := waitFor(c, 30*time.Second, func() bool {
		for _, message := range readMessages(lead) {
			if message.Undelivered != nil && message.Undelivered.ID == id {
				told = message
				return true
			}
		}
		return false
	})
	events, err := held.turnEvents()
	reached := offered(events, doomed)
	return judged(obsFixtureGrantRechecked,
		waiting && removeErr == nil && failed && strings.Contains(detail, "its grant does not pass") && strings.Contains(detail, "is gone") &&
			!reached && err == nil && toldOK && strings.Contains(told.Text, "is gone"),
		"waited pending: %v; removed: %v; failed: %v, %s; offered: %v; main told: %v, %q; the log %+v %v",
		waiting, removeErr, failed, detail, reached, toldOK, firstLine(told.Text), events, err)
}

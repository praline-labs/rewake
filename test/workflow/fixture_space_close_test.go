package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFixtureSpaceClose is main closing a worker it raised, on the fixture:
// main opens a space, hands its worker a task the worker leaves pending, and
// runs rewake space close; the space host ends the worker, the record closes
// with main's reason, and main's rewake inbox --awaited reads the task as
// lost, its worker ended — while main itself runs on.
func TestFixtureSpaceClose(t *testing.T) {
	binary := enterScenario(t, "fixture-space-close")
	c := Start(t, Spec{
		Name:         "fixture-space-close",
		Harness:      fixtureColumn.harness,
		Observations: fixtureSpaceCloseObservations,
		Deadline:     90 * time.Second,
	})
	iso := Isolate(t, c, binary)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, fixtureColumn.harness, "lead", "--main", asks.env(), shimInboxJSON+"=1")
	defer stopSession(t, c, lead)
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil }) {
		c.Contradicted(obsSpaceCloseAnswered, "main never became ready")
		return
	}
	worker, env := spaceWorker(iso, "api", fixtureColumn.harness)
	env = append(env, shimInboxJSON+"=1", shimPendingOnce+"="+spaceClosePending)
	aliasDir := filepath.Join(iso.Home, ".config", "rewake")
	if err := os.MkdirAll(aliasDir, 0o700); err != nil {
		t.Fatal(err)
	}
	aliases := fmt.Sprintf("[alias.worker]\nharness = %q\nrole = \"write\"\n", fixtureColumn.harness)
	if err := os.WriteFile(filepath.Join(aliasDir, "aliases.toml"), []byte(aliases), 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "api", "--alias", "worker", "--json"}, Env: env})
	code, out, answered := asks.send(c, encoded, "space open api")
	var opened struct {
		Session string `json:"session"`
		ID      string `json:"id"`
	}
	if !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil || opened.Session != worker.name {
		c.Contradicted(obsSpaceCloseAnswered, "space open answered %d, %v: %s", code, answered, out)
		return
	}

	if code, out, _ := asks.ask(c, "send", worker.name, spaceCloseTask); code != 0 {
		c.Contradicted(obsSpaceCloseAnswered, "the task was not sent: %d %s", code, out)
		return
	}
	if !waitFor(c, 30*time.Second, func() bool { _, found := reportOfKind(lead, worker, "pending"); return found }) {
		c.Contradicted(obsSpaceCloseAnswered, "main never read the worker's interim report; kinds main read: %v", kindsFrom(lead, worker))
		return
	}

	began := time.Now()
	code, out, _ = asks.ask(c, "space", "close", "api", "--json")
	took := time.Since(began).Round(100 * time.Millisecond)
	var closed struct {
		Space, Session, State, Why string
	}
	switch {
	case code != 0 || json.Unmarshal([]byte(out), &closed) != nil:
		c.Contradicted(obsSpaceCloseAnswered, "space close answered %d: %s", code, out)
	case closed.Session != worker.name || closed.State != "closed" || !strings.HasPrefix(closed.Why, "main closed it with rewake space close; "):
		c.Contradicted(obsSpaceCloseAnswered, "space close answered %+v", closed)
	default:
		c.Observed(obsSpaceCloseAnswered, fmt.Sprintf("space close answered after %s: %s", took, closed.Why))
	}

	record := spaceRecordOf(iso, opened.ID)
	gone := waitFor(c, 5*time.Second, func() bool { return !runAlive(record.Worker.Run) && !runAlive(record.Host) })
	switch {
	case record.State != "closed" || record.End.Worker != "ended" || !strings.HasPrefix(record.End.Why, "main closed it with rewake space close"):
		c.Contradicted(obsSpaceCloseEnded, "the record is %q with %+v", record.State, record.End)
	case !gone:
		c.Contradicted(obsSpaceCloseEnded, "the worker's wrapper %s or the space host %s still runs", record.Worker.Run, record.Host)
	case !lead.alive():
		c.Contradicted(obsSpaceCloseEnded, "main ended with its worker")
	default:
		c.Observed(obsSpaceCloseEnded, "the worker and its space host are gone, main runs on: "+record.End.Why)
	}

	code, out, _ = asks.ask(c, "inbox", "--awaited", "--json")
	var model awaitedModelView
	lost := code == 0 && json.Unmarshal([]byte(out), &model) == nil && len(model.Recipients) == 1 && model.Recipients[0].Name == worker.name &&
		len(model.Recipients[0].Messages) == 1 && model.Recipients[0].Messages[0].Gone == "ended" && model.Recipients[0].Messages[0].Text == spaceCloseTask
	if lost {
		c.Observed(obsSpaceCloseLost, "--awaited reads the worker's task as lost, its worker ended")
	} else {
		c.Contradicted(obsSpaceCloseLost, "--awaited answered %d: %s", code, out)
	}
}

const (
	obsSpaceCloseAnswered = "rewake space close, run by main, answers once the space is closed, with main's reason"
	obsSpaceCloseEnded    = "after the close the worker and the space host are gone, the record closed from the observed end, and main runs on"
	obsSpaceCloseLost     = "the task the closed worker still owed reads as lost in main's rewake inbox --awaited"
	spaceCloseTask        = "fixture-space-close-probe: run the long check"
	spaceClosePending     = "the long check is running"
)

var fixtureSpaceCloseObservations = []string{obsSpaceCloseAnswered, obsSpaceCloseEnded, obsSpaceCloseLost}

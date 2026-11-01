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

// TestFixtureReexec is the probe of a wrapper that replaces its own image
// (.scratch/reexec-probe.md), on the fixture: main and one worker in a space,
// the worker's wrapper sent SIGUSR2. While a turn of the worker is open the
// exec is refused and the turn's letter is reported once; once the worker is
// idle the wrapper execs the same build and goes on as the same run — its
// pid, epoch, harness and program, a socket that answers — and takes a new
// task with exactly one report; main's end then ends everything as usual.
func TestFixtureReexec(t *testing.T) {
	binary := enterScenario(t, "fixture-reexec")
	c := Start(t, Spec{Name: "fixture-reexec", Harness: fixtureColumn.harness, Observations: fixtureReexecObservations, Deadline: 120 * time.Second})
	iso := Isolate(t, c, binary)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, fixtureColumn.harness, "lead", "--main", asks.env(), shimInboxJSON+"=1")
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil }) {
		c.Contradicted(obsReexecRefused, "main never became ready")
		stopSession(t, c, lead)
		return
	}
	worker, env := spaceWorker(iso, "api", fixtureColumn.harness)
	release := filepath.Join(iso.Home, "end-release")
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	env = append(env, shimHoldEnd+"="+release)
	aliasDir := filepath.Join(iso.Home, ".config", "rewake")
	if err := os.MkdirAll(aliasDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliasDir, "aliases.toml"), fmt.Appendf(nil, "[alias.worker]\nharness = %q\nrole = \"write\"\n", fixtureColumn.harness), 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "api", "--alias", "worker", "--json"}, Env: env})
	var opened struct {
		Session string `json:"session"`
		ID      string `json:"id"`
	}
	if code, out, answered := asks.send(c, encoded, "space open api"); !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil {
		c.Contradicted(obsReexecRefused, "space open answered %d, %v: %s", code, answered, out)
		stopSession(t, c, lead)
		return
	}
	reports := func() int { return countReports(lead, worker, "finished") }
	task := func(n int) {
		if code, out, _ := asks.ask(c, "send", worker.name, fmt.Sprintf("%s %d", reexecTask, n)); code != 0 {
			t.Fatalf("task %d was not sent: %d %s", n, code, out)
		}
	}
	task(1)
	c.Await("the first report", func() bool { return reports() == 1 })
	before := reexecRecordOf(iso, worker.name)
	children := childrenOf(before.ServicePID)
	mark := filepath.Join(filepath.Dir(filepath.Dir(before.path)), "run", worker.name+"."+before.epoch(), "reexec")

	// #3: a turn is open — read, its end held by the scenario — and the exec
	// is refused; the end goes out once released, as one report.
	_ = os.Remove(release)
	task(2)
	c.Await("the worker to read task 2", func() bool { return strings.Contains(worker.mailboxRead(), reexecTask+" 2") })
	outcome := signalReexec(c, before.ServicePID, mark)
	_ = os.WriteFile(release, nil, 0o600)
	c.Await("the second report", func() bool { return reports() >= 2 })
	switch {
	case !strings.HasPrefix(outcome, "refused: a turn"):
		c.Contradicted(obsReexecRefused, "the exec during a turn answered %q", outcome)
	case replaced(before.ServicePID):
		c.Contradicted(obsReexecRefused, "refused, but the image was replaced")
	default:
		c.Observed(obsReexecRefused, outcome)
	}

	// #1: idle, the exec goes; the same run serves on.
	// The turn's end may still be looked at by the delivery server's
	// reminder: a refusal then is the probe working, and the signal is sent
	// again.
	var waited []string
	for range 50 {
		if outcome = signalReexec(c, before.ServicePID, mark); !strings.HasPrefix(outcome, "refused") {
			break
		}
		waited = append(waited, outcome)
		time.Sleep(100 * time.Millisecond)
	}
	after := reexecRecordOf(iso, worker.name)
	switch {
	case outcome != "adopted 1" || !replaced(before.ServicePID):
		c.Contradicted(obsReexecAdopted, "the exec when idle answered %q", outcome)
	case after.epoch() != before.epoch() || after.HarnessPID != before.HarnessPID || after.HarnessStart != before.HarnessStart:
		c.Contradicted(obsReexecAdopted, "the record moved: %+v, was %+v", after, before)
	case !allAlive(children) || !dials(after.Socket):
		c.Contradicted(obsReexecAdopted, "children %v alive %v, socket %s answers %v", children, allAlive(children), after.Socket, dials(after.Socket))
	default:
		c.Observed(obsReexecAdopted, fmt.Sprintf("pid %d, epoch kept, children %v running, the socket answers; refused first %d time(s) %v", before.ServicePID, children, len(waited), waited))
	}
	task(3)
	c.Await("the third report", func() bool { return reports() >= 3 })
	time.Sleep(time.Second)
	if n, distinct := reports(), distinctReplies(lead, worker); n != 3 || distinct != 3 {
		c.Contradicted(obsReexecReported, "%d reports answering %d distinct tasks, for three tasks", n, distinct)
	} else {
		c.Observed(obsReexecReported, "three tasks, three reports, one each")
	}

	stopSession(t, c, lead)
	var record spaceRecordView
	closed := waitFor(c, 20*time.Second, func() bool {
		record = spaceRecordOf(iso, opened.ID)
		return record.State == "closed"
	})
	gone := closed && waitFor(c, 5*time.Second, func() bool { return !runAlive(record.Worker.Run) && !runAlive(record.Host) && !anyAlive(children) })
	if !gone {
		c.Contradicted(obsReexecClosed, "space %q; wrapper, host or children %v still run", record.State, children)
	} else {
		c.Observed(obsReexecClosed, "closed: "+record.End.Why)
	}
}

const (
	obsReexecRefused  = "a worker's wrapper asked to re-execute during an open turn refuses, and that turn is reported once"
	obsReexecAdopted  = "an idle worker's wrapper re-executes the same build and keeps its pid, epoch, children and socket"
	obsReexecReported = "after the exec, a new task is delivered and every task has exactly one report"
	obsReexecClosed   = "main's end ends the re-executed worker, its children and the space host as usual"
	reexecTask        = "fixture-reexec-probe: report back"
)

var fixtureReexecObservations = []string{obsReexecRefused, obsReexecAdopted, obsReexecReported, obsReexecClosed}

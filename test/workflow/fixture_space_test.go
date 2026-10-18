package workflow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestFixtureSpace is a space on its main path, on the fixture: main opens
// one with rewake space open, the worker it starts joins main's room as an
// ordinary session, takes a task and reports it, and once main's run ends the
// space host stops the worker and closes the record from what it observed.
// The space host is a detached rewake process, the worker an ordinary launch
// of the fixture in its terminal; nothing of it is the fixture's but the
// offer of spaces a main run proves.
func TestFixtureSpace(t *testing.T) {
	binary := enterScenario(t, "fixture-space")
	c := Start(t, Spec{
		Name:         "fixture-space",
		Harness:      fixtureColumn.harness,
		Observations: fixtureSpaceObservations,
		Deadline:     90 * time.Second,
	})
	iso := Isolate(t, c, binary)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, fixtureColumn.harness, "lead", "--main", asks.env(), shimInboxJSON+"=1")
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil }) {
		c.Contradicted(obsSpaceOpened, "main never became ready")
		stopSession(t, c, lead)
		return
	}
	worker, env := spaceWorker(iso, "api", fixtureColumn.harness)

	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "api", "--harness", fixtureColumn.harness, "--json"}, Env: env})
	code, out, answered := asks.send(c, encoded, "space open api")
	var opened struct {
		Session string `json:"session"`
		ID      string `json:"id"`
	}
	if !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil || opened.Session != worker.name {
		c.Contradicted(obsSpaceOpened, "space open answered %d, %v: %s", code, answered, out)
		stopSession(t, c, lead)
		return
	}
	c.Observed(obsSpaceOpened, "space open answered "+opened.Session+", registered in main's room")

	if code, out, _ := asks.ask(c, "send", worker.name, fixtureSpaceTask); code != 0 {
		c.Contradicted(obsSpaceReported, "the task was not sent: %d %s", code, out)
	}
	c.Await("the worker to read the task", func() bool { return strings.Contains(worker.mailboxRead(), fixtureSpaceTask) })
	c.Await("main to read the worker's report", func() bool { _, found := reportOfKind(lead, worker, "finished"); return found })
	if _, found := reportOfKind(lead, worker, "finished"); found {
		c.Observed(obsSpaceReported, "the worker read the task with its own inbox, and main read its finished report")
	} else {
		c.Contradicted(obsSpaceReported, "no finished report from %s reached main", worker.name)
	}

	stopSession(t, c, lead)
	var record spaceRecordView
	closed := waitFor(c, 20*time.Second, func() bool {
		record = spaceRecordOf(iso, opened.ID)
		return record.State == "closed" || record.State == "kept"
	})
	// The record is closed before the space host leaves: give it the moment.
	gone := closed && waitFor(c, 5*time.Second, func() bool { return !runAlive(record.Worker.Run) && !runAlive(record.Host) })
	switch {
	case !closed:
		c.Contradicted(obsSpaceClosed, "the space is %q after main ended", record.State)
	case record.End.Worker != "ended" || !strings.Contains(record.End.Why, "main's run ended"):
		c.Contradicted(obsSpaceClosed, "closed with %+v", record.End)
	case !gone:
		c.Contradicted(obsSpaceClosed, "the worker's wrapper %s or the space host %s still runs", record.Worker.Run, record.Host)
	default:
		c.Observed(obsSpaceClosed, "closed: "+record.End.Why)
	}
}

const (
	obsSpaceOpened   = "rewake space open, run by main, answers once the worker's session is registered in main's room"
	obsSpaceReported = "the worker takes a task from main and its report reaches main"
	obsSpaceClosed   = "once main's run ends, the worker and the space host are gone and the record is closed from the observed end"
	fixtureSpaceTask = "fixture-space-probe: report back"
)

var fixtureSpaceObservations = []string{obsSpaceOpened, obsSpaceReported, obsSpaceClosed}

// spaceWorker is the worker a space will start, as a scenario session with
// files of its own, and the environment changes that give it them: it
// inherits the environment of main's command, whose shim files are main's.
func spaceWorker(iso *Isolation, name, harness string) (*scenarioSession, []string) {
	session := &scenarioSession{
		name:      name + "-" + harness,
		state:     filepath.Join(iso.Home, name+".state.json"),
		mailbox:   filepath.Join(iso.Home, name+".mailbox"),
		exitFile:  filepath.Join(iso.Home, name+".exit"),
		turns:     filepath.Join(iso.Home, name+".turns"),
		delivered: filepath.Join(iso.Home, name+".delivered"),
		groups:    filepath.Join(iso.Home, name+".groups"),
		reads:     filepath.Join(iso.Home, name+".reads"),
		sends:     filepath.Join(iso.Home, name+".sends"),
		ready:     filepath.Join(iso.Home, name+".listening"),
		home:      iso.Home,
	}
	return session, []string{
		shimRequestDir,
		shimStateFile + "=" + session.state, shimReadyFile + "=" + session.ready,
		shimMailboxFile + "=" + session.mailbox, shimExitFile + "=" + session.exitFile,
		shimTurnsFile + "=" + session.turns, shimDeliveredFile + "=" + session.delivered,
		shimGroupsFile + "=" + session.groups, shimReadsFile + "=" + session.reads,
		shimSendsFile + "=" + session.sends,
	}
}

type spaceRecordView struct {
	State  string `json:"state"`
	Host   string `json:"host"`
	Worker struct {
		Run string `json:"run"`
	} `json:"worker"`
	End struct {
		Why    string `json:"why"`
		Worker string `json:"worker"`
	} `json:"end"`
}

// spaceRecordOf reads the space's record as rewake space list prints it.
func spaceRecordOf(iso *Isolation, id string) spaceRecordView {
	out, err := iso.Output(iso.Command("space", "list", "--json"))
	if err != nil {
		return spaceRecordView{}
	}
	var listing struct {
		Spaces []json.RawMessage `json:"spaces"`
	}
	if json.Unmarshal(out, &listing) != nil {
		return spaceRecordView{}
	}
	for _, raw := range listing.Spaces {
		var probe struct {
			ID string `json:"id"`
		}
		var record spaceRecordView
		if json.Unmarshal(raw, &probe) == nil && probe.ID == id && json.Unmarshal(raw, &record) == nil {
			return record
		}
	}
	return spaceRecordView{}
}

// runAlive says whether the process a run names still runs: its pid is
// there with the start time the run names, and not a zombie — the space host
// leaves as one, since the suite adopts the orphans it buries itself.
func runAlive(run string) bool {
	pid, rest, ok := strings.Cut(run, ".")
	if !ok {
		return false
	}
	start, _, _ := strings.Cut(rest, ".")
	number, err := strconv.Atoi(pid)
	if err != nil || errors.Is(syscall.Kill(number, 0), syscall.ESRCH) {
		return false
	}
	stat, err := os.ReadFile(filepath.Join("/proc", pid, "stat"))
	if err != nil {
		return false
	}
	_, after, _ := strings.Cut(string(stat), ") ")
	fields := strings.Fields(after)
	return len(fields) > 19 && fields[0] != "Z" && fields[19] == start
}

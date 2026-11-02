package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFixtureSwitch is the switch of a running room to another build
// (docs/rules/state.md#switching-a-running-room), on the fixture column, whose
// wrappers can hand their runs over.
//
//   - A plain main and a plain worker run build A from an install of their
//     own; the install is then replaced by build B, the same tree linked with
//     another build time, as an update replaces it.
//   - The person's rewake restart rewake switches the room: both wrappers
//     keep their pid, epoch and harness and run B, and the room's build and
//     copy are B. A new task gets exactly one report.
//   - With a space open, a switch back to A is refused naming the space host,
//     which cannot hand over; the room stays on B and a task gets one report.
//
// What it does not prove: a real harness. A worker of Claude Code or pi hands
// its run over when idle, proven by its adapter's tests; main's wrapper and
// main's and a space's hosts cannot yet, so every real room is refused as the
// space host is here.
func TestFixtureSwitch(t *testing.T) {
	binary := enterScenario(t, "fixture-switch")
	c := Start(t, Spec{Name: "fixture-switch", Harness: fixtureColumn.harness, Observations: fixtureSwitchObservations, Deadline: 180 * time.Second})
	iso := Isolate(t, c, binary)
	other, err := buildRewakeWith(t.TempDir(), "-X github.com/praline-labs/rewake/internal/cli.built="+switchBuilt)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(iso.Home, "install", "rewake")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(installed, original); err != nil {
		t.Fatal(err)
	}
	iso.binary = installed
	room := filepath.Join(iso.StateDir, "v2", "rooms", iso.HomeRoom())

	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, fixtureColumn.harness, "lead", "--main", asks.env(), shimInboxJSON+"=1")
	worker := startHarnessSession(t, c, iso, fixtureColumn.harness, "worker", "--general")
	if err := sessionsUp(c, lead, worker); err != nil {
		stopSession(t, c, worker)
		stopSession(t, c, lead)
		c.Contradicted(obsSwitchSwitched, "%v", err)
		return
	}
	reports := func() int { return countReports(lead, worker, "finished") }
	task := func(n int) {
		if code, out, _ := asks.ask(c, "send", worker.name, fmt.Sprintf("%s %d", switchTask, n)); code != 0 {
			t.Fatalf("task %d was not sent: %d %s", n, code, out)
		}
		c.Await(fmt.Sprintf("report %d", n), func() bool { return reports() >= n })
	}
	task(1)
	before := map[string]reexecRecord{lead.name: reexecRecordOf(iso, lead.name), worker.name: reexecRecordOf(iso, worker.name)}

	if err := replaceBinary(installed, newer); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := switchRoom(c, iso)
	var moved []string
	for name, was := range before {
		now := reexecRecordOf(iso, name)
		exe, _ := os.ReadFile(fmt.Sprintf("/proc/%d/exe", was.ServicePID))
		if now.epoch() != was.epoch() || now.HarnessPID != was.HarnessPID || idOf(exe) != idOf(newer) {
			moved = append(moved, fmt.Sprintf("%s: epoch %s, was %s; harness %d, was %d; runs %.12s", name, now.epoch(), was.epoch(), now.HarnessPID, was.HarnessPID, idOf(exe)))
		}
	}
	copied, _ := os.ReadFile(filepath.Join(room, "bin", "rewake"))
	switch {
	case code != 0:
		c.Contradicted(obsSwitchSwitched, "restart rewake exited %d: %s %s", code, out, errOut)
	case len(moved) > 0:
		c.Contradicted(obsSwitchSwitched, "%s", strings.Join(moved, "; "))
	case recordOf(room) != idOf(newer) || idOf(copied) != idOf(newer):
		c.Contradicted(obsSwitchSwitched, "the room's build %.12s and its copy %.12s; B is %.12s", recordOf(room), idOf(copied), idOf(newer))
	default:
		c.Observed(obsSwitchSwitched, firstLine(out))
	}
	task(2)
	time.Sleep(time.Second)
	if n, distinct := reports(), distinctReplies(lead, worker); n != 2 || distinct != 2 {
		c.Contradicted(obsSwitchReported, "%d reports answering %d distinct tasks, for two tasks", n, distinct)
	} else {
		c.Observed(obsSwitchReported, "two tasks, one across the switch, one report each")
	}

	opened := openSpace(t, c, iso, asks)
	if err := replaceBinary(installed, original); err != nil {
		t.Fatal(err)
	}
	// The committed mark holds the room until its deadline, whoever adopted.
	c.Await("the first switch's mark to end", func() bool { return !switchHolds(room) })
	code, _, errOut = switchRoom(c, iso)
	task(3)
	switch {
	case opened == "":
		c.Contradicted(obsSwitchRefused, "the space did not open")
	case code != 1 || !strings.Contains(errOut, "the space host of "+opened+" cannot"):
		c.Contradicted(obsSwitchRefused, "restart rewake with a space open exited %d: %s", code, errOut)
	case recordOf(room) != idOf(newer):
		c.Contradicted(obsSwitchRefused, "the refused switch left the build %.12s", recordOf(room))
	case reports() != 3 || distinctReplies(lead, worker) != 3:
		c.Contradicted(obsSwitchRefused, "%d reports for three tasks", reports())
	default:
		c.Observed(obsSwitchRefused, strings.TrimSpace(errOut))
	}
	stopSession(t, c, worker)
	stopSession(t, c, lead)
	if opened != "" {
		// Main's end ends the space, as usual.
		var record spaceRecordView
		waitFor(c, 20*time.Second, func() bool {
			record = spaceRecordOf(iso, opened)
			return record.State == "closed" && !runAlive(record.Worker.Run) && !runAlive(record.Host)
		})
	}
}

// switchHolds says a switch's mark in the room has not reached its deadline.
func switchHolds(room string) bool {
	data, err := os.ReadFile(filepath.Join(room, "switch"))
	if err != nil {
		return false
	}
	var mark struct{ Deadline int64 }
	return json.Unmarshal(data, &mark) == nil && time.Now().UnixMilli() < mark.Deadline
}

// switchRoom runs the person's rewake restart rewake, again while a session
// is busy handing over — a notice just delivered is held in its wrapper's
// memory a while — as the refusal says to.
func switchRoom(c *Case, iso *Isolation) (int, string, string) {
	var code int
	var out, errOut string
	for range 5 {
		stdout, err := c.OutputAllowingFailure(iso.Command("restart", "rewake"))
		code, out, errOut = 0, string(stdout), ""
		if err != nil {
			// The error carries the command's stderr.
			code, errOut = -1, err.Error()
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = exit.ExitCode()
			}
		}
		if code == 0 || !strings.Contains(errOut, "busy") {
			break
		}
		c.Note("the switch was refused as busy, and is run again: " + strings.TrimSpace(errOut))
		time.Sleep(2 * time.Second)
	}
	return code, out, errOut
}

// openSpace opens a space of a fixture worker from main, answering its id,
// or "" when it did not open.
func openSpace(t *testing.T, c *Case, iso *Isolation, asks *requests) string {
	t.Helper()
	_, env := spaceWorker(iso, "api", fixtureColumn.harness)
	aliasDir := filepath.Join(iso.Home, ".config", "rewake")
	if err := os.MkdirAll(aliasDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliasDir, "aliases.toml"), fmt.Appendf(nil, "[alias.worker]\nharness = %q\nrole = \"write\"\n", fixtureColumn.harness), 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "api", "--alias", "worker", "--json"}, Env: env})
	var opened struct {
		ID string `json:"id"`
	}
	if code, out, answered := asks.send(c, encoded, "space open api"); !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil {
		return ""
	}
	return opened.ID
}

const (
	obsSwitchSwitched = "rewake restart rewake switches an idle fixture room to another build: main's and the worker's wrappers keep their pid, epoch and harness and run the new build, which is the room's build and copy"
	obsSwitchReported = "after the switch, a new task is delivered and every task has exactly one report"
	obsSwitchRefused  = "with a space open, the switch is refused naming the space host, the room keeps its build, and a task gets one report"
	switchTask        = "fixture-switch: report back"
	// switchBuilt is build B's build time: the same tree, another image.
	switchBuilt = "2026-11-02T00:00:00Z"
)

var fixtureSwitchObservations = []string{obsSwitchSwitched, obsSwitchReported, obsSwitchRefused}

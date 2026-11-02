package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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
//   - With a space open, a switch back to A goes through: the space host and
//     the space's worker each keep their pid and run A, the space stays open
//     and its screen is the same, and a task gets one report.
//
// What it does not prove: a real harness. A worker of Claude Code or pi hands
// its run over when idle, proven by its adapter's tests; main's wrapper and
// main's host cannot yet, so every real room is refused on them.
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
	var was spaceRecordView
	var shown string
	if opened != "" {
		// A session of the suite lives 25 seconds (shimReportState): the
		// worker's screen is taken once it shows anything.
		waitFor(c, 5*time.Second, func() bool {
			was = spaceRecordOf(iso, opened)
			shown = screenText(iso, "api")
			return was.State == "open" && strings.TrimSpace(shown) != ""
		})
	}
	if err := replaceBinary(installed, original); err != nil {
		t.Fatal(err)
	}
	// The committed mark holds the room until its deadline, whoever adopted.
	c.Await("the first switch's mark to end", func() bool { return !switchHolds(room) })
	code, out, errOut = switchRoom(c, iso)
	now := spaceRecordOf(iso, opened)
	hostExe, _ := os.ReadFile(fmt.Sprintf("/proc/%d/exe", runPID(now.Host)))
	workerExe, _ := os.ReadFile(fmt.Sprintf("/proc/%d/exe", runPID(now.Worker.Run)))
	shownAfter := screenText(iso, "api")
	task(3)
	switch {
	case opened == "":
		c.Contradicted(obsSwitchSpace, "the space did not open")
	case code != 0:
		c.Contradicted(obsSwitchSpace, "restart rewake with a space open exited %d: %s %s", code, out, errOut)
	case now.State != "open" || now.Host != was.Host || now.Worker.Run != was.Worker.Run || !runAlive(now.Host) || !runAlive(now.Worker.Run):
		c.Contradicted(obsSwitchSpace, "the space is %s, its host %s and worker %s; they were %s and %s", now.State, now.Host, now.Worker.Run, was.Host, was.Worker.Run)
	case idOf(hostExe) != idOf(original) || idOf(workerExe) != idOf(original):
		c.Contradicted(obsSwitchSpace, "the space host runs %.12s and the worker %.12s; A is %.12s", idOf(hostExe), idOf(workerExe), idOf(original))
	case recordOf(room) != idOf(original):
		c.Contradicted(obsSwitchSpace, "the switch left the build %.12s", recordOf(room))
	case !sameScreen(shown, shownAfter):
		c.Contradicted(obsSwitchSpace, "the worker's screen was\n%s\nand is\n%s", shown, shownAfter)
	case reports() != 3 || distinctReplies(lead, worker) != 3:
		c.Contradicted(obsSwitchSpace, "%d reports for three tasks", reports())
	default:
		c.Observed(obsSwitchSpace, firstLine(out))
	}
	// A committed switch's leader cannot remove its socket, its lease
	// revoked at the commit; the next attempt clears it. Nobody listens on it.
	leftover := filepath.Join(room, "sock", "switch.sock")
	if conn, err := net.Dial("unix", leftover); err == nil {
		_ = conn.Close()
		t.Errorf("something still listens on %s after the switch", leftover)
	}
	_ = os.Remove(leftover)
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

// runPID is the pid of a run's epoch, 0 for none.
func runPID(run string) int {
	pid, _, _ := strings.Cut(run, ".")
	number, _ := strconv.Atoi(pid)
	return number
}

// screenText is rewake screen's text of the space's worker by its name, from
// its space host's snapshot: no shell runs here.
func screenText(iso *Isolation, name string) string {
	out, err := iso.Output(iso.Command("screen", name, "--text"))
	if err != nil {
		return ""
	}
	return string(out)
}

// sameScreen says two screens show the same lines: the worker wrote nothing
// meanwhile, and its wrapper adopting its run announces nothing again.
func sameScreen(before, after string) bool {
	lines := func(screen string) []string {
		var out []string
		for _, line := range strings.Split(screen, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				out = append(out, line)
			}
		}
		return out
	}
	was, is := lines(before), lines(after)
	return len(was) > 0 && slices.Equal(is, was)
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
	obsSwitchSpace    = "with a space open, the switch goes through: the space host and its worker keep their pid and run the new build, the space stays open with the same screen, and a task gets one report"
	switchTask        = "fixture-switch: report back"
	// switchBuilt is build B's build time: the same tree, another image.
	switchBuilt = "2026-11-02T00:00:00Z"
)

var fixtureSwitchObservations = []string{obsSwitchSwitched, obsSwitchReported, obsSwitchSpace}

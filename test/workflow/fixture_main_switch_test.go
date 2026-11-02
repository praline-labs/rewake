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

// TestFixtureMainSwitch is a switch of the room's build that main asks for
// from its own turn, and one the person's window ends
// (docs/rules/state.md#switching-a-running-room).
//
//   - Main, launched on a terminal as a person does, runs behind its host and
//     holds each turn open a few seconds. From a turn the worker's notice
//     began, it runs rewake restart rewake: the command answers at once, the
//     room keeps its build while main's turn is open, and the switch goes
//     through once main is idle; its outcome comes to main as a note.
//   - The shell drew on through it: rewake restart shell, asked of the
//     adopted host, answers generation 2.
//   - The person closes the window while main's host is parked by a switch
//     back: what the shell then says waits in its channel for the image
//     that takes the run on, and main's host, main and the shell end as on
//     any closed window; no switch holds the room, and the worker beside
//     main runs on.
func TestFixtureMainSwitch(t *testing.T) {
	binary := enterScenario(t, "fixture-main-switch")
	c := Start(t, Spec{Name: "fixture-main-switch", Harness: fixtureColumn.harness, Observations: mainSwitchObservations, Deadline: 180 * time.Second})
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
	shell, shellSaid := fakeShell(t, iso)
	lead, window := startOnTerminal(t, c, iso, "lead", "--main", asks.env(), shimInboxJSON+"=1", shimHoldTurn+"=1", "REWAKE_SHELL="+shell)
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil && shellSaid("1 attached main") }) {
		c.Contradicted(obsMainSwitchDeferred, "main never became ready under its host, or the shell never attached to it")
		closeWindow(c, window, lead)
		return
	}
	host := lead.process.cmd.Process.Pid
	// The worker's notice of its arrival begins main's first turn, which
	// main holds open.
	worker := startHarnessSession(t, c, iso, fixtureColumn.harness, "worker", "--general", staysUp(iso, "worker"))
	if err := sessionsUp(c, worker); err != nil {
		c.Contradicted(obsMainSwitchDeferred, "%v", err)
		stopSession(t, c, worker)
		closeWindow(c, window, lead)
		return
	}
	c.Await("main to hold its turn open", func() bool { return turnEventSeen(lead, "operation-open") })
	before := reexecRecordOf(iso, lead.name)
	if err := replaceBinary(installed, newer); err != nil {
		t.Fatal(err)
	}
	code, out, _ := asks.ask(c, "restart", "rewake", "--json")
	var asked struct {
		Deferred bool `json:"deferred"`
	}
	heldOpen := !turnEventSeen(lead, "operation-closed")
	keptBuild := recordOf(room) == idOf(original)
	var note string
	noted := waitFor(c, 30*time.Second, func() bool {
		for _, m := range readMessages(lead) {
			if strings.HasPrefix(m.Text, "rewake restart rewake: ") {
				note = m.Text
				return true
			}
		}
		return false
	})
	now := reexecRecordOf(iso, lead.name)
	hostExe, _ := os.ReadFile(fmt.Sprintf("/proc/%d/exe", host))
	wrapperExe, _ := os.ReadFile(fmt.Sprintf("/proc/%d/exe", now.ServicePID))
	switch {
	case code != 0 || json.Unmarshal([]byte(out), &asked) != nil || !asked.Deferred:
		c.Contradicted(obsMainSwitchDeferred, "main's rewake restart rewake answered %d: %s", code, out)
	case !heldOpen || !keptBuild:
		c.Contradicted(obsMainSwitchDeferred, "when the command answered, main's turn was open %v and the room kept its build %v", heldOpen, keptBuild)
	case !noted:
		c.Contradicted(obsMainSwitchDeferred, "no outcome reached main; the room's build is %.12s", recordOf(room))
	case !strings.Contains(note, "the room switched from build"):
		c.Contradicted(obsMainSwitchDeferred, "main was told %q", note)
	case recordOf(room) != idOf(newer) || idOf(hostExe) != idOf(newer) || idOf(wrapperExe) != idOf(newer) || now.epoch() != before.epoch():
		c.Contradicted(obsMainSwitchDeferred, "the room's build %.12s, main's host runs %.12s and its wrapper %.12s, epoch %s, was %s; B is %.12s",
			recordOf(room), idOf(hostExe), idOf(wrapperExe), now.epoch(), before.epoch(), idOf(newer))
	default:
		c.Observed(obsMainSwitchDeferred, note)
	}

	restart, err := iso.Output(iso.Command("restart", "shell", "--json"))
	var answer struct {
		Generation int `json:"generation"`
	}
	switch {
	case err != nil || json.Unmarshal(restart, &answer) != nil || answer.Generation != 2:
		c.Contradicted(obsMainSwitchShell, "rewake restart shell after the switch answered %v: %s", err, restart)
	case !waitFor(c, 5*time.Second, func() bool { return shellSaid("2 attached main") }):
		c.Contradicted(obsMainSwitchShell, "the new shell never attached to main")
	default:
		c.Observed(obsMainSwitchShell, "generation 2, attached to main's stream")
	}

	if err := replaceBinary(installed, original); err != nil {
		t.Fatal(err)
	}
	c.Await("the first switch's mark to end", func() bool { return !switchHolds(room) })
	// Main idle, so the switch goes on to its drop at once.
	c.Await("main's turns to end", func() bool { return turnsClosed(lead) })
	switched := make(chan string, 1)
	go func() {
		out, err := c.OutputAllowingFailure(iso.Command("restart", "rewake"))
		if err != nil {
			switched <- err.Error()
			return
		}
		switched <- string(out)
	}()
	// The window closes while main's host is parked, its image waiting for
	// the leader's word with the shell's channel unread.
	parked := false
	for deadline := time.Now().Add(20 * time.Second); !parked && time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		parked = isParked(host)
	}
	_ = window.Close()
	ended := waitFor(c, 25*time.Second, func() bool { return lead.process.finished() && !lead.process.groupAlive() })
	var refused string
	select {
	case refused = <-switched:
	case <-time.After(90 * time.Second):
		refused = "the person's rewake restart rewake never returned"
	}
	settled := waitFor(c, 30*time.Second, func() bool { return !switchHolds(room) })
	workerNow := reexecRecordOf(iso, worker.name)
	switch {
	case !parked:
		c.Contradicted(obsMainSwitchClosed, "main's host was never seen parked by the switch back")
	case !ended:
		c.Contradicted(obsMainSwitchClosed, "main's host or a process of its group still runs after its window closed")
	case runAlive(now.epoch()):
		c.Contradicted(obsMainSwitchClosed, "main's wrapper %s still runs", now.epoch())
	case !settled:
		c.Contradicted(obsMainSwitchClosed, "a switch still holds the room")
	case !runAlive(workerNow.epoch()):
		c.Contradicted(obsMainSwitchClosed, "the worker %s did not run on", workerNow.epoch())
	default:
		lines := strings.Split(strings.TrimSpace(refused), "\n")
		c.Observed(obsMainSwitchClosed, "the switch answered: "+lines[len(lines)-1])
	}
	clearSwitchSocket(t, room)
	stopSession(t, c, worker)
}

const (
	obsMainSwitchDeferred = "rewake restart rewake asked from main's open turn answers at once, the room keeps its build while the turn is open, and the switch goes through once main is idle: main's host and wrapper keep their pid and epoch and run the new build, and main is told the outcome as a note"
	obsMainSwitchShell    = "the shell drew on through the switch: rewake restart shell, asked of the adopted host, answers generation 2 and the new shell attaches to main"
	obsMainSwitchClosed   = "a window closed while main's host is parked by a switch ends main's host, main and the shell, no switch holds the room afterwards, and the worker beside main runs on"
)

var mainSwitchObservations = []string{obsMainSwitchDeferred, obsMainSwitchShell, obsMainSwitchClosed}

// isParked says main's host runs as an image a switch parked: its handover,
// in the environment it was started with, names the park, which the image
// that takes the run on no longer carries.
func isParked(pid int) bool {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return false
	}
	for _, entry := range strings.Split(string(raw), "\x00") {
		if handover, ok := strings.CutPrefix(entry, "REWAKE_MAIN_HOST="); ok {
			return strings.Contains(handover, `"park":{`)
		}
	}
	return false
}

// turnsClosed says every turn the session opened has closed.
func turnsClosed(s *scenarioSession) bool {
	events, _ := s.turnEvents()
	open := 0
	for _, e := range events {
		switch e.Kind {
		case "operation-open":
			open++
		case "operation-closed":
			open--
		}
	}
	return open == 0
}

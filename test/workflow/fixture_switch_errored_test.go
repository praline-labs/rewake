package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFixtureSwitchErrored is a switch of the room's build right after a
// session's turn ended in an error (docs/rules/state.md#switching-a-running-room),
// as a turn does on an API error, a limit or the person's Esc: the case seen
// live was main's pi, whose turn ended with no model, refused as busy for the
// whole of the switch's ten minutes.
//
//   - A worker whose turns all fail takes a task in a turn that reads
//     nothing and ends in an error: the task stays announced and unread, and
//     no finished end looks at it.
//   - The person's rewake restart rewake goes through: the worker's wrapper
//     keeps its pid and epoch and runs the new build, and the room's build is
//     the new one.
//
// What it does not prove: the reminder across the switch, which the delivery
// server's own test holds (TestAnAnnouncedTaskGoesOverWithTheRunAndIsRemindedOnce).
func TestFixtureSwitchErrored(t *testing.T) {
	binary := enterScenario(t, "fixture-switch-errored")
	c := Start(t, Spec{Name: "fixture-switch-errored", Harness: fixtureColumn.harness, Observations: []string{obsSwitchErrored}, Deadline: 120 * time.Second})
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

	leave := filepath.Join(iso.Home, "worker.leave-unread")
	if err := os.WriteFile(leave, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, fixtureColumn.harness, "lead", "--main", asks.env(), shimInboxJSON+"=1")
	defer stopSession(t, c, lead)
	worker := startHarnessSession(t, c, iso, fixtureColumn.harness, "worker", "--general", staysUp(iso, "worker"),
		shimInboxJSON+"=1", shimLateFailure+"=1", shimLeaveUnread+"="+leave)
	defer stopSession(t, c, worker)
	if err := sessionsUp(c, lead, worker); err != nil {
		c.Contradicted(obsSwitchErrored, "%v", err)
		return
	}
	if code, out, _ := asks.ask(c, "send", worker.name, "fixture-switch-errored: build the package"); code != 0 {
		c.Contradicted(obsSwitchErrored, "the task was not sent: %d %s", code, out)
		return
	}
	failed := func() bool {
		events, _ := worker.turnEvents()
		for _, e := range events {
			if e.Kind == "completed" && strings.Contains(e.Detail, "failed") {
				return true
			}
		}
		return false
	}
	c.Await("the worker's turn to end in an error", failed)
	before := reexecRecordOf(iso, worker.name)

	if err := replaceBinary(installed, newer); err != nil {
		t.Fatal(err)
	}
	// Refused as busy, the person's command would wait out the switch's ten
	// minutes: the scenario looks for a bounded while.
	switched := make(chan string, 1)
	go func() {
		code, out, errOut := switchRoom(c, iso)
		switched <- fmt.Sprintf("exit %d: %s%s", code, out, errOut)
	}()
	var answer string
	select {
	case answer = <-switched:
	case <-time.After(45 * time.Second):
		answer = "still waiting after 45s"
	}
	now := reexecRecordOf(iso, worker.name)
	exe, _ := os.ReadFile(fmt.Sprintf("/proc/%d/exe", now.ServicePID))
	switch {
	case !strings.HasPrefix(answer, "exit 0"):
		c.Contradicted(obsSwitchErrored, "rewake restart rewake after the errored turn: %s", strings.TrimSpace(answer))
	case recordOf(room) != idOf(newer) || idOf(exe) != idOf(newer) || now.epoch() != before.epoch():
		c.Contradicted(obsSwitchErrored, "the room's build %.12s, the worker's wrapper %s runs %.12s, was %s; B is %.12s",
			recordOf(room), now.epoch(), idOf(exe), before.epoch(), idOf(newer))
	default:
		c.Observed(obsSwitchErrored, strings.TrimSpace(answer))
	}
	clearSwitchSocket(t, room)
}

const obsSwitchErrored = "a switch asked right after a worker's turn ended in an error, a task it announced left unread, goes through: the worker's wrapper keeps its pid and epoch and runs the new build"

package workflow

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// TestFixtureShellRestart is a restart of rewake's shell on its main path
// (docs/shell.md, "Restart, crash and close"): main launched on a terminal
// runs under its host, the pane's process, with the shell as the host's
// child; main opens a space, the worker takes a task and holds its turn
// open, and rewake restart shell replaces the shell meanwhile. Main and the
// worker are the same runs afterwards, the task's one report reaches main,
// and closing the terminal ends everything as before. The shell is this test
// binary speaking the host's channel; the host, main and the worker are
// rewake's own.
func TestFixtureShellRestart(t *testing.T) {
	binary := enterScenario(t, "fixture-shell-restart")
	c := Start(t, Spec{
		Name:         "fixture-shell-restart",
		Harness:      fixtureColumn.harness,
		Observations: shellRestartObservations,
		Deadline:     120 * time.Second,
	})
	iso := Isolate(t, c, binary)
	record := filepath.Join(iso.Home, "shell.record")
	shell := filepath.Join(iso.ShimDir, "fake-shell")
	script := "#!/bin/sh\nexec env " + fakeShellEnv + "=" + record + " \"$RW_SHIM_TEST_EXE\" -test.run=TestShimHelper -- \"$@\"\n"
	if err := replaceScript(shell, script); err != nil {
		t.Fatal(err)
	}
	shellSaid := func(line string) bool {
		raw, _ := os.ReadFile(record)
		return strings.Contains(string(raw), line)
	}
	asks := newRequests(iso, "lead")
	lead, window := startOnTerminal(t, c, iso, "lead", "--main", asks.env(), shimInboxJSON+"=1", "REWAKE_SHELL="+shell)
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil && shellSaid("1 attached main") }) {
		c.Contradicted(obsShellRestarted, "main never became ready under its host, or the first shell never attached to it")
		closeWindow(c, window, lead)
		return
	}

	worker, env := spaceWorker(iso, "api", fixtureColumn.harness)
	aliasDir := filepath.Join(iso.Home, ".config", "rewake")
	if err := os.MkdirAll(aliasDir, 0o700); err != nil {
		t.Fatal(err)
	}
	aliases := "[alias.worker]\nharness = \"" + fixtureColumn.harness + "\"\nrole = \"write\"\n"
	if err := os.WriteFile(filepath.Join(aliasDir, "aliases.toml"), []byte(aliases), 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "api", "--alias", "worker", "--json"}, Env: append(env, shimHoldTurn+"=1")})
	code, out, answered := asks.send(c, encoded, "space open api")
	var opened struct {
		ID string `json:"id"`
	}
	if !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil {
		c.Contradicted(obsShellRestarted, "space open answered %d, %v: %s", code, answered, out)
		closeWindow(c, window, lead)
		return
	}
	if code, out, _ := asks.ask(c, "send", worker.name, shellRestartTask); code != 0 {
		c.Contradicted(obsShellKept, "the task was not sent: %d %s", code, out)
	}
	c.Await("the worker to hold its turn open", func() bool { return turnEventSeen(worker, "operation-open") })
	before := identities(iso)

	restart, err := iso.Output(iso.Command("restart", "shell", "--json"))
	var answer struct {
		Generation int `json:"generation"`
	}
	working := !turnEventSeen(worker, "operation-closed")
	switch {
	case err != nil || json.Unmarshal(restart, &answer) != nil || answer.Generation != 2:
		c.Contradicted(obsShellRestarted, "rewake restart shell answered %v: %s", err, restart)
	case !shellSaid("1 released") || !waitFor(c, 5*time.Second, func() bool { return shellSaid("2 attached main") }):
		c.Contradicted(obsShellRestarted, "the old shell was not let go, or the new one never attached to main")
	case !working:
		c.Contradicted(obsShellRestarted, "the worker's turn had ended before the restart answered")
	default:
		c.Observed(obsShellRestarted, "generation 2 answered while the worker's turn was open; the first shell released, the second attached to main's stream")
	}

	c.Await("main to read the worker's report", func() bool { _, found := reportOfKind(lead, worker, "finished"); return found })
	// A second report would follow the first within the poll; give it the time.
	time.Sleep(2 * time.Second)
	after := identities(iso)
	reports := 0
	for _, message := range readMessages(lead) {
		if message.From == worker.name && message.Kind == "finished" {
			reports++
		}
	}
	switch {
	case before[lead.name] == "" || before[worker.name] == "":
		c.Contradicted(obsShellKept, "main or the worker was not listed before the restart: %v", before)
	case before[lead.name] != after[lead.name] || before[worker.name] != after[worker.name]:
		c.Contradicted(obsShellKept, "the runs changed across the restart: before %v, after %v", before, after)
	case reports != 1:
		c.Contradicted(obsShellKept, "%d finished reports from %s reached main", reports, worker.name)
	default:
		c.Observed(obsShellKept, "main "+after[lead.name]+" and the worker "+after[worker.name]+" kept their runs, and one report reached main")
	}

	closeWindow(c, window, lead)
	var space spaceRecordView
	closed := waitFor(c, 25*time.Second, func() bool {
		space = spaceRecordOf(iso, opened.ID)
		return space.State == "closed" || space.State == "kept"
	})
	gone := closed && waitFor(c, 5*time.Second, func() bool { return !runAlive(space.Worker.Run) && !runAlive(space.Host) })
	switch {
	case !lead.process.finished():
		c.Contradicted(obsShellClosed, "the host still runs after its terminal closed")
	case !closed || !gone:
		c.Contradicted(obsShellClosed, "the space is %q, the worker %s or its host %s may still run", space.State, space.Worker.Run, space.Host)
	case !shellSaid("2 lost"):
		c.Contradicted(obsShellClosed, "the shell did not tell the host its terminal was gone")
	default:
		c.Observed(obsShellClosed, "the host, main, the worker and its space host ended: "+space.End.Why)
	}
}

const (
	obsShellRestarted = "rewake restart shell, asked while a worker works, answers the new shell's generation once the old one let go and the new one attached to main's stream"
	obsShellKept      = "main and the worker keep their pid, start and epoch across the restart, and the task's one report reaches main"
	obsShellClosed    = "closing the terminal afterwards ends the host, main, the worker and its space host"
	shellRestartTask  = "fixture-shell-restart-probe: report back"
)

var shellRestartObservations = []string{obsShellRestarted, obsShellKept, obsShellClosed}

// startOnTerminal launches a session as a person does, on a terminal: its
// standard input and output are a pty the scenario holds, the window. The
// window is not the launch's controlling terminal — the case's process group
// is the launch's own — so closing it reaches the launch as the end of its
// terminal's input, which the shell hears.
func startOnTerminal(t *testing.T, c *Case, iso *Isolation, name, role string, controls ...string) (*scenarioSession, *terminal.Plain) {
	t.Helper()
	session, launch := sessionLaunch(t, c, iso, "", fixtureColumn.harness, name, role, nil, nil, controls...)
	window, err := terminal.OpenPlain(terminal.Size{Cols: 120, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	if err := window.Prepare(launch); err != nil {
		t.Fatal(err)
	}
	launch.SysProcAttr.Setsid, launch.SysProcAttr.Setctty = false, false
	release := c.captureStderr(launch, iso.Home, name)
	// A launch whose window is closed ends with the code of its main, hung
	// up, as before the host: not 0, and not a failure of the session.
	process, err := c.start(launch, true)
	release()
	_ = window.ReleaseSide()
	if err != nil {
		_ = window.Close()
		t.Fatalf("launching the session: %v", err)
	}
	// What the host draws goes nowhere, but has to be read.
	go func() { _, _ = io.Copy(io.Discard, window.Master) }()
	session.home, session.process = iso.Home, process
	return session, window
}

// closeWindow closes the terminal and waits for the launch on it to end.
func closeWindow(c *Case, window *terminal.Plain, session *scenarioSession) {
	_ = window.Close()
	waitFor(c, 20*time.Second, func() bool { return session.process.finished() && !session.process.groupAlive() })
}

// identities is each listed session's epoch — its wrapper's pid, start and boot —
// as rewake list gives it.
func identities(iso *Isolation) map[string]string {
	out, err := iso.Output(iso.Command("list", "--json"))
	var listed struct {
		Sessions []registry.Session `json:"sessions"`
	}
	runs := map[string]string{}
	if err != nil || json.Unmarshal(out, &listed) != nil {
		return runs
	}
	for _, s := range listed.Sessions {
		runs[s.Name] = s.Epoch()
	}
	return runs
}

// turnEventSeen says whether the session recorded an event of kind.
func turnEventSeen(s *scenarioSession, kind string) bool {
	events, _ := s.turnEvents()
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

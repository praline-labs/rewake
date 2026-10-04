package harness

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
)

// script writes a shell program for a check to run.
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "program")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// alive says whether a process runs: a zombie was ended, whoever reaps it.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	state, err := proc.State(pid)
	return err == nil && state != "Z"
}

// pidFrom reads a pid a program wrote, waiting for it.
func pidFrom(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && perr == nil && pid > 0 {
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no pid in %s", path)
	return 0
}

func TestACheckThatExitsGivesItsCodeAndBoundedOutput(t *testing.T) {
	program := script(t, `head -c 200000 /dev/zero | tr '\0' x; echo; exit 3`)
	check, err := StartCheck(CheckSpec{Label: "test", Program: program, Bound: 5 * time.Second, Capture: true})
	if err != nil {
		t.Fatal(err)
	}
	code, ok := check.Wait()
	if err := check.End(); err != nil {
		t.Fatal(err)
	}
	if !ok || code != 3 {
		t.Fatalf("code %d ok %v", code, ok)
	}
	if got := len(check.Output()); got != maxCheckOutput {
		t.Fatalf("kept %d bytes, want the bound %d", got, maxCheckOutput)
	}
}

func TestACheckPastItsBoundIsEnded(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"hangs", "sleep 60"},
		{"ignores SIGTERM", "trap '' TERM; while :; do sleep 1; done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check, err := StartCheck(CheckSpec{Label: "test", Program: script(t, tc.body), Bound: 200 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := check.Wait(); ok {
				t.Fatal("a hanging check answered within its bound")
			}
			pid := check.pid
			if err := check.End(); err != nil {
				t.Fatal(err)
			}
			if alive(pid) {
				t.Fatalf("process %d outlived the check", pid)
			}
		})
	}
}

// A server the check started that left its group — a session of its own,
// as a daemon does — is adopted and ended with the check.
func TestWhatACheckLeftInASessionOfItsOwnIsEnded(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	body := `setsid sh -c 'echo $$ > ` + marker + `; trap "" TERM; while :; do sleep 1; done' </dev/null >/dev/null 2>&1 &
exit 0`
	check, err := StartCheck(CheckSpec{Label: "test", Program: script(t, body), Bound: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if code, ok := check.Wait(); !ok || code != 0 {
		t.Fatalf("code %d ok %v", code, ok)
	}
	left := pidFrom(t, marker)
	if !alive(left) {
		t.Fatal("the left process is not running, so the test proves nothing")
	}
	if err := check.End(); err != nil {
		t.Fatal(err)
	}
	if alive(left) {
		_ = syscall.Kill(left, syscall.SIGKILL)
		t.Fatalf("process %d, which the check left in its own session, outlived it", left)
	}
}

// A descendant that moved to a group of its own within the check's session,
// and whose parent exited, is ended as well: its group does not tell it from
// a child of this process, its session does.
func TestWhatACheckLeftInAnotherGroupOfItsSessionIsEnded(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	body := `perl -e 'setpgrp(0, 0); open(my $f, ">", "` + marker + `"); print $f "$$\n"; close($f); $SIG{TERM} = "IGNORE"; sleep 30' </dev/null >/dev/null 2>&1 &
exit 0`
	check, err := StartCheck(CheckSpec{Label: "test", Program: script(t, body), Bound: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	check.Wait()
	left := pidFrom(t, marker)
	_, group, _, _ := procIDs(left)
	if !alive(left) || group != left {
		t.Fatalf("process %d in group %d: the test proves nothing", left, group)
	}
	if err := check.End(); err != nil {
		t.Fatal(err)
	}
	if alive(left) {
		_ = syscall.Kill(left, syscall.SIGKILL)
		t.Fatalf("process %d, in another group of the check's session, outlived it", left)
	}
}

// What the check left in a session of its own, below a parent that lives
// on, is ended with it: the holder finds its tree by parent links, and does
// not wait for an orphan to come to it.
func TestWhatACheckLeftBelowALiveParentIsEnded(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	body := `perl -e 'use POSIX; POSIX::setsid(); $SIG{TERM} = "IGNORE"; if (fork() == 0) { open(my $f, ">", "` + marker + `"); print $f "$$\n"; close($f); sleep 30; exit } sleep 30' </dev/null >/dev/null 2>&1 &
exit 0`
	check, err := StartCheck(CheckSpec{Label: "test", Program: script(t, body), Bound: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	check.Wait()
	left := pidFrom(t, marker)
	parent, _, session, _ := procIDs(left)
	if !alive(left) || !alive(parent) || session == check.pid {
		t.Fatalf("process %d, parent %d, session %d: the test proves nothing", left, parent, session)
	}
	if err := check.End(); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{left, parent} {
		if alive(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("process %d, in a session of its own below the check, outlived it", pid)
		}
	}
}

// A check whose holder is gone cannot say its tree ended: End says so, and
// the launch is refused rather than run beside what may be left.
func TestACheckWhoseHolderIsLostSaysSo(t *testing.T) {
	check, err := StartCheck(CheckSpec{Label: "test", Program: script(t, "sleep 30"), Bound: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	program := check.pid
	t.Cleanup(func() { _ = syscall.Kill(program, syscall.SIGKILL) })
	_ = syscall.Kill(check.holder.Process.Pid, syscall.SIGKILL)
	if err := check.End(); err == nil {
		t.Fatal("End answered nil with the holder killed")
	}
}

// A holder whose starter is gone ends the check's tree: the order pipe
// closing is all it takes.
func TestAHolderEndsTheTreeWhenItsStarterIsGone(t *testing.T) {
	check, err := StartCheck(CheckSpec{Label: "test", Program: script(t, "trap '' TERM; while :; do sleep 1; done"), Bound: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	program, holder := check.pid, check.holder.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-holder, syscall.SIGKILL)
		_ = syscall.Kill(program, syscall.SIGKILL)
	})
	_ = check.control.Close()
	select {
	case <-check.held:
	case <-time.After(10 * time.Second):
		t.Fatal("the holder outlived its order pipe")
	}
	if alive(program) {
		_ = syscall.Kill(program, syscall.SIGKILL)
		t.Fatalf("process %d outlived its holder's end", program)
	}
	_ = check.End()
}

// A process of this one that the check did not start is left alone, in
// this process's group, in a group of its own or in a session of its own.
func TestACheckLeavesOtherChildrenAlone(t *testing.T) {
	var sleepers []int
	for _, own := range []*syscall.SysProcAttr{{}, {Setpgid: true}, {Setsid: true}} {
		other := syscall.ProcAttr{Files: []uintptr{0, 1, 2}, Sys: own}
		sleeper, err := syscall.ForkExec("/bin/sleep", []string{"sleep", "30"}, &other)
		if err != nil {
			t.Fatal(err)
		}
		sleepers = append(sleepers, sleeper)
	}
	defer func() {
		for _, sleeper := range sleepers {
			_ = syscall.Kill(sleeper, syscall.SIGKILL)
			var s syscall.WaitStatus
			_, _ = syscall.Wait4(sleeper, &s, 0, nil)
		}
	}()
	check, err := StartCheck(CheckSpec{Label: "test", Program: script(t, "exit 0"), Bound: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	check.Wait()
	if err := check.End(); err != nil {
		t.Fatal(err)
	}
	for _, sleeper := range sleepers {
		if !alive(sleeper) {
			t.Fatalf("the check ended process %d, which it did not start", sleeper)
		}
	}
}

func TestACheckThatCannotStartSaysSo(t *testing.T) {
	if _, err := StartCheck(CheckSpec{Label: "test", Program: filepath.Join(t.TempDir(), "absent"), Bound: time.Second}); err == nil {
		t.Fatal("a missing program started")
	}
}

func TestGatesAssumedAcceptsOnlyTableNames(t *testing.T) {
	names, err := ParseAssumedGates(" g7, G2 ,G2,")
	if err != nil || strings.Join(names, ",") != "G2,G7" {
		t.Fatalf("%v %v", names, err)
	}
	if _, err := ParseAssumedGates("G2,G10"); err == nil || !strings.Contains(err.Error(), `"G10"`) {
		t.Fatalf("an unknown gate was accepted: %v", err)
	}
	gates := ResolveGates("codex", "", names)
	if gates.Open(GateG2) || gates.Open(GateG7) || !gates.Open(GateG9) {
		t.Fatal("assumed gates are not taken as closed, or others are")
	}
}

// A gate recorded closed for a version is closed only for a launch whose
// program answers that version; any other answer, or none, leaves it open.
func TestAGateClosedForAVersionTakesTheProgramsAnswer(t *testing.T) {
	saved := closedGates
	t.Cleanup(func() { closedGates = saved })
	closedGates = map[string]map[string][]string{GateG2: {"codex": {"0.159.0"}}}
	for _, tc := range []struct {
		body   string
		closed bool
	}{
		{`echo "codex-cli 0.159.0"`, true},
		{`echo "codex-cli 0.159.1"`, false},
		{`echo "codex-cli 0.159.0"; exit 1`, false},
		{`echo nothing`, false},
		{`sleep 30`, false},
	} {
		program := script(t, tc.body)
		if got := !GatesFor("codex", program, nil, "", nil).Open(GateG2); got != tc.closed {
			t.Fatalf("%q: G2 closed %v, want %v", tc.body, got, tc.closed)
		}
		if !GatesFor("claude", program, nil, "", nil).Open(GateG2) {
			t.Fatalf("%q: a gate closed for codex closed it for claude", tc.body)
		}
	}
	closedGates = map[string]map[string][]string{}
	// With nothing recorded, no version is read, and an assumed gate is taken
	// as closed all the same.
	marker := filepath.Join(t.TempDir(), "asked")
	asked := script(t, "touch "+marker)
	if GatesNeedVersion("codex") || GatesFor("codex", asked, nil, "", []string{GateG2}).Open(GateG2) {
		t.Fatal("an empty table needs a version, or an assumed gate stayed open")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the program was asked for its version with nothing recorded")
	}
}

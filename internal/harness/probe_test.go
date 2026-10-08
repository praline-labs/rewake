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

// script writes a shell program for a probe to run.
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

func TestAProbeThatExitsGivesItsCodeAndBoundedOutput(t *testing.T) {
	program := script(t, `head -c 200000 /dev/zero | tr '\0' x; echo; exit 3`)
	check, err := StartProbe(ProbeSpec{Label: "test", Program: program, Bound: 5 * time.Second, Capture: true})
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
	if got := len(check.Output()); got != maxProbeOutput {
		t.Fatalf("kept %d bytes, want the bound %d", got, maxProbeOutput)
	}
}

func TestAProbePastItsBoundIsEnded(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"hangs", "sleep 60"},
		{"ignores SIGTERM", "trap '' TERM; while :; do sleep 1; done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check, err := StartProbe(ProbeSpec{Label: "test", Program: script(t, tc.body), Bound: 200 * time.Millisecond})
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
				t.Fatalf("process %d outlived the probe", pid)
			}
		})
	}
}

// A server the probe started that left its group — a session of its own,
// as a daemon does — is adopted and ended with the probe.
func TestWhatAProbeLeftInASessionOfItsOwnIsEnded(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	body := `setsid sh -c 'echo $$ > ` + marker + `; trap "" TERM; while :; do sleep 1; done' </dev/null >/dev/null 2>&1 &
exit 0`
	check, err := StartProbe(ProbeSpec{Label: "test", Program: script(t, body), Bound: 5 * time.Second})
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
		t.Fatalf("process %d, which the probe left in its own session, outlived it", left)
	}
}

// A descendant that moved to a group of its own within the probe's session,
// and whose parent exited, is ended as well: its group does not tell it from
// a child of this process, its session does.
func TestWhatAProbeLeftInAnotherGroupOfItsSessionIsEnded(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	body := `perl -e 'setpgrp(0, 0); open(my $f, ">", "` + marker + `"); print $f "$$\n"; close($f); $SIG{TERM} = "IGNORE"; sleep 30' </dev/null >/dev/null 2>&1 &
exit 0`
	check, err := StartProbe(ProbeSpec{Label: "test", Program: script(t, body), Bound: 5 * time.Second})
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
		t.Fatalf("process %d, in another group of the probe's session, outlived it", left)
	}
}

// What the probe left in a session of its own, below a parent that lives
// on, is ended with it: the holder finds its tree by parent links, and does
// not wait for an orphan to come to it.
func TestWhatAProbeLeftBelowALiveParentIsEnded(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	body := `perl -e 'use POSIX; POSIX::setsid(); $SIG{TERM} = "IGNORE"; if (fork() == 0) { open(my $f, ">", "` + marker + `"); print $f "$$\n"; close($f); sleep 30; exit } sleep 30' </dev/null >/dev/null 2>&1 &
exit 0`
	check, err := StartProbe(ProbeSpec{Label: "test", Program: script(t, body), Bound: 5 * time.Second})
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
			t.Fatalf("process %d, in a session of its own below the probe, outlived it", pid)
		}
	}
}

// A probe whose holder is gone cannot say its tree ended: End says so, and
// the launch is refused rather than run beside what may be left.
func TestAProbeWhoseHolderIsLostSaysSo(t *testing.T) {
	check, err := StartProbe(ProbeSpec{Label: "test", Program: script(t, "sleep 30"), Bound: 5 * time.Second})
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

// A holder whose starter is gone ends the probe's tree: the order pipe
// closing is all it takes.
func TestAHolderEndsTheTreeWhenItsStarterIsGone(t *testing.T) {
	check, err := StartProbe(ProbeSpec{Label: "test", Program: script(t, "trap '' TERM; while :; do sleep 1; done"), Bound: 5 * time.Second})
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

// A process of this one that the probe did not start is left alone, in
// this process's group, in a group of its own or in a session of its own.
func TestAProbeLeavesOtherChildrenAlone(t *testing.T) {
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
	check, err := StartProbe(ProbeSpec{Label: "test", Program: script(t, "exit 0"), Bound: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	check.Wait()
	if err := check.End(); err != nil {
		t.Fatal(err)
	}
	for _, sleeper := range sleepers {
		if !alive(sleeper) {
			t.Fatalf("the probe ended process %d, which it did not start", sleeper)
		}
	}
}

func TestAProbeThatCannotStartSaysSo(t *testing.T) {
	if _, err := StartProbe(ProbeSpec{Label: "test", Program: filepath.Join(t.TempDir(), "absent"), Bound: time.Second}); err == nil {
		t.Fatal("a missing program started")
	}
}

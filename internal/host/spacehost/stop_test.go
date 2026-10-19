package spacehost

import (
	"bufio"
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// startGroup starts a shell as the leader of its own process group, with a
// child in it, and waits until both are set up.
func startGroup(t *testing.T, script string) (*exec.Cmd, chan struct{}) {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := make(chan struct{})
	go func() { _ = cmd.Wait(); close(reaped) }()
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-reaped
	})
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("the group did not start: %q, %v", line, err)
	}
	return cmd, reaped
}

// The close sequence ends a worker that ignores the hangup and the request
// to end: SIGKILL comes once they have had their time, and the whole group is
// gone after it — the harness the wrapper started as well as the wrapper. A
// worker that ends on the hangup gets nothing more.
func TestTheCloseSequenceEndsAWorkerThatIgnoresHangupAndTerm(t *testing.T) {
	times := stopTimes{hangup: 100 * time.Millisecond, term: 200 * time.Millisecond, kill: 2 * time.Second}

	stubborn, reaped := startGroup(t, `trap "" HUP TERM; sleep 60 & echo ready; wait`)
	began := time.Now()
	if took := stopGroup(stubborn.Process.Pid, reaped, times); took != "SIGKILL" {
		t.Fatalf("a worker ignoring SIGHUP and SIGTERM ended after %q", took)
	}
	if elapsed := time.Since(began); elapsed < times.hangup+times.term {
		t.Fatalf("SIGKILL came after %s, before the hangup and the request had their time", elapsed)
	}
	if err := syscall.Kill(-stubborn.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("something of the group is left: %v", err)
	}

	plain, reaped := startGroup(t, `sleep 60 & echo ready; wait`)
	if took := stopGroup(plain.Process.Pid, reaped, times); took != "SIGHUP" {
		t.Fatalf("a worker that ends on the hangup ended after %q", took)
	}
	if took := stopGroup(plain.Process.Pid, reaped, times); took != "nothing" {
		t.Fatalf("a group already gone was sent %q", took)
	}
}

// prSetChildSubreaper is PR_SET_CHILD_SUBREAPER.
const prSetChildSubreaper = 36

// An orphan of the worker that ended is a zombie until whoever adopted it
// reaps it, and kill(2) still finds it in the group: seen in the workflow
// suite, whose subreaper reaps in batches, as a space left closing with
// nothing of its worker running. The group is gone once only zombies are left.
func TestTheCloseSequenceTakesAGroupOfZombiesAsGone(t *testing.T) {
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0); errno != 0 {
		t.Skipf("no subreaper: %v", errno)
	}
	var pgid int
	t.Cleanup(func() {
		_, _, _ = syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 0, 0)
		var status syscall.WaitStatus
		for pgid != 0 {
			if pid, _ := syscall.Wait4(-pgid, &status, syscall.WNOHANG, nil); pid <= 0 {
				return
			}
		}
	})
	// The hangup ends the leader and its child; the child, orphaned, is
	// adopted by this process and stays a zombie.
	times := stopTimes{hangup: 2 * time.Second, term: 100 * time.Millisecond, kill: 100 * time.Millisecond}
	worker, reaped := startGroup(t, `sleep 60 & echo ready; wait`)
	pgid = worker.Process.Pid
	if took := stopGroup(pgid, reaped, times); took != "SIGHUP" {
		t.Fatalf("a group left with a zombie ended after %q", took)
	}
	if err := syscall.Kill(-pgid, 0); err != nil {
		t.Fatalf("the zombie the test needs was not there: %v", err)
	}
}

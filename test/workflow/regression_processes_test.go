package workflow

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The half of the reproductions that is about processes: descendants that
// outlive their parent, leave their process group, hold the output open or
// refuse to stop. The other half, about how a case is classified, is in
// regression_test.go.

// exec.CommandContext kills the immediate child only, so a descendant holding
// the pipe open decides when the call returns. The case ends the whole process
// group instead, which is what makes its deadline mean anything.
func TestDeadlineBoundsDescendantsToo(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "descendants", Observations: []string{"a"}, Deadline: 30 * time.Millisecond})
	base := t.TempDir()
	iso := &Isolation{binary: "/bin/sh", forCase: c, Home: base, StateDir: base, CodexHome: base, ShimDir: base}

	started := time.Now()
	_, err := iso.Output(iso.Command("-c", "sleep 5 & wait"))
	elapsed := time.Since(started)

	if err == nil {
		t.Error("a command that outlived the deadline reported no error")
	}
	if elapsed > 3*time.Second {
		t.Errorf("a 30ms deadline returned only after %s, so a descendant was holding it open", elapsed)
	}

	// The observation is made deliberately: without it the case would be
	// incomplete anyway, and the test would prove nothing about the deadline.
	c.Observed("a", "made despite the deadline")
	rec.finish()
	outcome, reason := c.Result()
	if outcome.Green() {
		t.Errorf("a case that ran past its deadline is green: %s", outcome)
	}
	if !strings.Contains(reason, "deadline") {
		t.Errorf("the case failed for some other reason than the deadline: %q", reason)
	}
}

// A socket is caught because of what it is. The wrapper's upstream socket is
// named `<name>.sock.up`, so anything matching on a `.sock` suffix misses the
// one file that most reliably means a session is still up.
func TestLiveUpstreamSocketIsCaught(t *testing.T) {
	iso := &Isolation{StateDir: t.TempDir()}
	dir := filepath.Join(iso.StateDir, "rooms", "default", "sock")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(dir, "worker.sock.up"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if err := iso.noLiveSockets(); err == nil {
		t.Error("a live upstream socket was accepted as a clean state directory")
	}
}

// A parent that exits says nothing about the group it started. What the case
// owns is the group, so that is what has to be gone.
func TestParentExitDoesNotHideDescendants(t *testing.T) {
	// No prctl here: TestMain makes the whole process a subreaper for the
	// life of the run. Turning it on and off around one test would switch it
	// off for whatever ran next, which under -shuffle is a different test each
	// time — and a test that quietly depends on adoption would then fail at
	// random.
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "exited-parent", Observations: []string{"a"}})
	pidPath := filepath.Join(t.TempDir(), "pid")
	process, err := c.start(exec.Command("/bin/sh", "-c", `sleep 30 >/dev/null 2>&1 & echo $! > "$1"`, "sh", pidPath), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.wait(); err != nil {
		t.Fatalf("the parent should exit cleanly: %v", err)
	}
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		var status syscall.WaitStatus
		_, _ = syscall.Wait4(pid, &status, 0, nil)
	}()

	c.Observed("a", "done")
	rec.finish()
	if syscall.Kill(pid, 0) == nil {
		outcome, reason := c.Result()
		t.Errorf("a descendant survived finalization: result=%s reason=%q", outcome, reason)
	}
}

// Preparation has to ask about descendants on every path, not only when it
// timed out: a parent that exits normally can leave a background process
// running, and preparation that returned nil then lied about it.
func TestPreparationChecksDescendantsOnSuccess(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "pid")
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	_, err := outputBounded(ctx, "prepare",
		exec.Command("/bin/sh", "-c", `sleep 30 >/dev/null 2>&1 & echo $! > "$1"`, "sh", pidPath))
	pid := readPID(t, pidPath)
	defer reap(pid)
	if syscall.Kill(pid, 0) == nil {
		t.Errorf("preparation returned with a living descendant: err=%v", err)
	}
}

// setsid moves a process out of the group it was started in, after which the
// original group id proves nothing. The Codex adapter does exactly this for
// the native app-server, so a session could otherwise keep running while the
// case reported a clean finish.
func TestDescendantInItsOwnGroupCannotKeepGreen(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "separate-group", Observations: []string{"a"}})
	pidPath := filepath.Join(t.TempDir(), "pid")
	process, err := c.start(exec.Command("/bin/sh", "-c",
		`setsid /bin/sh -c 'echo $$ > "$1"; exec sleep 30' sh "$1" >/dev/null 2>&1 &`, "sh", pidPath), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.wait(); err != nil {
		t.Fatalf("the parent should exit cleanly: %v", err)
	}
	pid := readPID(t, pidPath)
	defer reap(pid)

	c.Observed("a", "done")
	rec.finish()
	if syscall.Kill(pid, 0) == nil {
		outcome, reason := c.Result()
		t.Errorf("a descendant in its own group survived: result=%s reason=%q", outcome, reason)
	}
}

// readPID waits for the file a helper writes its pid into. The helper is
// started in the background, so the write and the read race.
func readPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, err := os.ReadFile(path)
		if err == nil && len(strings.TrimSpace(string(raw))) > 0 {
			pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatalf("unreadable pid in %s: %v", path, err)
			}
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("the helper never wrote its pid to %s", path)
		}
		time.Sleep(time.Millisecond)
	}
}

// reap makes sure a test leaves nothing running, whatever it proved.
func reap(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
	var status syscall.WaitStatus
	_, _ = syscall.Wait4(pid, &status, 0, nil)
}

// A descendant that left the group inherits the pipe, so waiting for the
// parent before cleaning up means waiting on exactly what nothing has cleaned
// up. Measured before the fix: a 100 ms deadline needed an outside rescue
// after 14 seconds.
func TestEscapedDescendantHoldingOutputDoesNotBlockTheDeadline(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "pid")
	ctx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()

	started := time.Now()
	done := make(chan error, 1)
	go func() {
		_, err := outputBounded(ctx, "escaped-pipe", exec.Command("/bin/sh", "-c",
			`setsid /bin/sh -c 'echo $$ > "$1"; exec sleep 30' sh "$1" &`, "sh", pidPath))
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("preparation returned success with a descendant holding its output")
		}
		if elapsed := time.Since(started); elapsed > terminationBudget {
			t.Errorf("cleanup took %s, past its own %s budget", elapsed, terminationBudget)
		}
	case <-time.After(terminationBudget + 5*time.Second):
		// Rescue the fixture rather than leave it running for the next test.
		pid := readPID(t, pidPath)
		reap(pid)
		t.Fatalf("cleanup never returned; it needed an outside rescue after %s", time.Since(started))
	}
	pid := readPID(t, pidPath)
	defer reap(pid)
	if syscall.Kill(pid, 0) == nil {
		t.Error("the escaped descendant is still running")
	}
}

// A descendant that ignores SIGTERM has to be killed, not merely reported. The
// case fails either way, but a red result with a process still running is not
// the same as a red result without one: the suite exists partly so that a run
// leaves nothing behind.
//
// This one came from a refactor that collapsed two cleanup passes into one and
// left through the first exit: the process was gone, so the loop returned in
// 12 ms and the descendant never got the SIGKILL the budget was holding for it.
// Fourteen earlier reproductions missed it, because every one of them had a
// descendant that either died on SIGTERM or held the output open.
func TestEscapedDescendantIgnoringSigtermIsStillKilled(t *testing.T) {
	// The path under test is SIGTERM, wait, SIGKILL — not how long the wait
	// is. Shortening only the loop's pacing keeps this regression in the
	// ordinary gate instead of costing twelve seconds in every run.
	shortenTerminationBudget(t, 3*time.Second)

	pidPath := filepath.Join(t.TempDir(), "pid")
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()

	started := time.Now()
	_, err := outputBounded(ctx, "escaped-term", exec.Command("/bin/sh", "-c",
		`setsid /bin/sh -c 'trap "" TERM; echo $$ > "$1"; exec sleep 30' sh "$1" >/dev/null 2>&1 & `+
			`while [ ! -s "$1" ]; do sleep .01; done`, "sh", pidPath))
	pid := readPID(t, pidPath)
	defer reap(pid)

	// A delivered SIGKILL still needs a moment to take effect, and an ignored
	// SIGTERM never will.
	time.Sleep(50 * time.Millisecond)
	if state, ok := processState(pid); ok && state != "Z" {
		t.Errorf("cleanup returned after %s with the escaped child still running: pid=%d state=%s err=%v",
			time.Since(started), pid, state, err)
	}
}

// shortenTerminationBudget speeds up the cleanup loop for one test and puts
// the real budget back afterwards. It changes the pacing only: the stages the
// real budget is built from, and the comment explaining them, stay as they are.
func shortenTerminationBudget(t *testing.T, budget time.Duration) {
	t.Helper()
	previous := terminationBudget
	terminationBudget = budget
	t.Cleanup(func() { terminationBudget = previous })
}

// processState reads the state field of /proc/<pid>/stat: "Z" for a process
// that has been reaped by nobody yet, "S"/"R" for one still running.
func processState(pid int) (string, bool) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(raw)[strings.LastIndex(string(raw), ")")+1:])
	if len(fields) == 0 {
		return "", false
	}
	return fields[0], true
}

// Not being able to read /proc is not evidence that nothing is running there.
// This is the class of mistake the package exists to prevent, one level below
// the classifier: at the point where the data is gathered.
func TestProcReadFailureCannotKeepGreen(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "proc-unavailable", Observations: []string{"a"}})
	pidPath := filepath.Join(t.TempDir(), "pid")
	process, err := c.start(exec.Command("/bin/sh", "-c",
		`setsid /bin/sh -c 'echo $$ > "$1"; exec sleep 30' sh "$1" >/dev/null 2>&1 &`, "sh", pidPath), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.wait(); err != nil {
		t.Fatal(err)
	}
	pid := readPID(t, pidPath)
	defer reap(pid)
	c.Observed("a", "done")

	var original syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &original); err != nil {
		t.Fatal(err)
	}
	limited := original
	limited.Cur = 0
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limited); err != nil {
		t.Fatal(err)
	}
	_, readErr := os.ReadDir("/proc")
	rec.finish()
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &original); err != nil {
		t.Fatal(err)
	}

	if readErr == nil {
		t.Fatal("the fixture did not manage to make /proc unreadable")
	}
	if outcome, reason := c.Result(); outcome.Green() {
		t.Errorf("a case is green although /proc could not be read: result=%s reason=%q", outcome, reason)
	}
}

// A process the case owns that failed is a failure of the case, unless the
// scenario said it expected one.
func TestFailedOwnedProcessIsNotGreen(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "failed-process", Observations: []string{"a"}})
	process, err := c.start(exec.Command("/bin/sh", "-c", "exit 7"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.wait(); err == nil {
		t.Fatal("precondition: the process was supposed to fail")
	}
	c.Observed("a", "done")
	rec.finish()
	if outcome, reason := c.Result(); outcome.Green() {
		t.Errorf("a failed owned process was discarded: result=%s reason=%q", outcome, reason)
	}
}

// The other half of the same rule: a scenario whose point is a refusal must be
// able to observe it without the case failing over the exit status.
func TestExpectedFailureDoesNotFailTheCase(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "expected-failure", Observations: []string{"a"}})
	base := t.TempDir()
	iso := &Isolation{binary: "/bin/sh", forCase: c, Home: base, StateDir: base, CodexHome: base, ShimDir: base}
	if _, err := c.OutputAllowingFailure(iso.Command("-c", "exit 7")); err == nil {
		t.Fatal("precondition: the command was supposed to fail")
	}
	c.Observed("a", "the refusal was observed")
	rec.finish()
	if outcome, reason := c.Result(); !outcome.Green() {
		t.Errorf("an expected refusal failed the case: result=%s reason=%q", outcome, reason)
	}
}

// The finaliser joins processes before the cleanup checks run, and publishes
// the verdict after both. Swapping those two was tried during review and no
// test noticed, so the order is asserted here rather than left to reading.
func TestFinishJoinsProcessesBeforeCheckingCleanup(t *testing.T) {
	rec := &recorder{}
	c := newCase(rec, Spec{Name: "ordering", Observations: []string{"a"}})
	base := t.TempDir()
	iso := &Isolation{binary: "/bin/sh", forCase: c, Home: base, StateDir: base, CodexHome: base, ShimDir: base}

	process, err := c.start(iso.Command("-c", "sleep 30"), false)
	if err != nil {
		t.Fatal(err)
	}
	var sawRunning, sawVerdict bool
	c.CheckCleanup("observes the state at cleanup time", func() error {
		sawRunning = process.groupAlive()
		outcome, _ := c.Result()
		sawVerdict = outcome != NotRun
		return nil
	})
	c.Observed("a", "done")
	rec.finish()

	if sawRunning {
		t.Error("the cleanup check ran while the case's process group was still alive")
	}
	if sawVerdict {
		t.Error("the verdict was published before the cleanup checks ran")
	}
}

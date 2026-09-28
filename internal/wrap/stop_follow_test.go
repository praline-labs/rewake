package wrap

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
)

// stopFollowerEnv makes the test binary a wrapper that follows one harness.
const stopFollowerEnv = "REWAKE_STOP_FOLLOWER_TEST"

func init() {
	// In the wrapper, waitForHarness runs on some thread other than the main
	// one once its goroutine has been through a blocking wait. Pinning the main
	// goroutine to the main thread keeps every other goroutine off it, so the
	// follower here is where the wrapper's is — and where the old stop lost the
	// race. On the main thread it never did.
	if os.Getenv(stopFollowerEnv) != "" {
		runtime.LockOSThread()
	}
}

// A harness stopped from outside must stay stopped while its wrapper is: the
// wrapper continues it only once it is continued itself. Before the fix the
// wrapper's stop of itself took effect after it had already continued the
// harness, most of the time — so the rounds are repeated.
func TestAStoppedHarnessStaysStoppedWithItsWrapper(t *testing.T) {
	if os.Getenv(stopFollowerEnv) != "" {
		followOneHarness(t)
		return
	}
	for round := range 10 {
		wrapper, harness := startFollower(t)
		if err := syscall.Kill(harness, syscall.SIGSTOP); err != nil {
			t.Fatalf("stop the harness: %v", err)
		}
		waitForState(t, wrapper, func(state string) bool { return state == "T" }, "the wrapper to stop with its harness")
		// The thread that stops the wrapper may run a moment longer than the one
		// /proc reports on; a continue it sends in that moment lands here.
		time.Sleep(20 * time.Millisecond)
		if state, _ := proc.State(harness); state != "T" {
			t.Fatalf("round %d: the harness is %q while its wrapper is stopped: nothing serves its mailbox and nothing says so", round+1, state)
		}
		if err := syscall.Kill(wrapper, syscall.SIGCONT); err != nil {
			t.Fatalf("continue the wrapper: %v", err)
		}
		waitForState(t, harness, func(state string) bool { return state != "T" }, "the harness to be continued with its wrapper")
		_ = syscall.Kill(harness, syscall.SIGKILL)
	}
}

// followOneHarness is the wrapper's side: it starts a harness, names its pid
// and follows it until it ends.
func followOneHarness(t *testing.T) {
	harness := exec.Command("sleep", "30")
	if err := harness.Start(); err != nil {
		t.Fatalf("start the harness: %v", err)
	}
	_, _ = os.Stdout.WriteString(strconv.Itoa(harness.Process.Pid) + "\n")
	waitForHarness(harness.Process.Pid)
}

// startFollower starts the wrapper side in a process of its own and returns its
// pid and its harness's.
func startFollower(t *testing.T) (int, int) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.timeout=20s")
	command.Env = append(os.Environ(), stopFollowerEnv+"=1")
	out, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start the wrapper: %v", err)
	}
	reader := bufio.NewReader(out)
	line, err := reader.ReadString('\n')
	harness, convErr := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || convErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("read the harness pid: %q, %v %v", line, err, convErr)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(harness, syscall.SIGKILL)
		_ = command.Process.Kill()
		_, _ = io.Copy(io.Discard, reader)
		_ = command.Wait()
	})
	return command.Process.Pid, harness
}

// waitForState waits, within a bound, for a process to reach a state.
func waitForState(t *testing.T, pid int, reached func(string) bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, err := proc.State(pid)
		if err == nil && reached(state) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waited 5s for %s: state %q, %v", what, state, err)
		}
		time.Sleep(time.Millisecond)
	}
}

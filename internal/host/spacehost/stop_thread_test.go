package spacehost

import (
	"bufio"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// threadWorkerEnv makes the test binary a worker whose leader thread ends
// while its other threads run on, ignoring the hangup and the request to end.
const threadWorkerEnv = "REWAKE_TEST_THREAD_WORKER"

func init() {
	// The main goroutine stays on the leader thread, so the worker can end
	// that thread alone.
	if os.Getenv(threadWorkerEnv) != "" {
		runtime.LockOSThread()
	}
}

func TestMain(m *testing.M) {
	if os.Getenv(threadWorkerEnv) != "" {
		signal.Ignore(syscall.SIGHUP, syscall.SIGTERM)
		go func() {
			for {
				time.Sleep(10 * time.Millisecond)
			}
		}()
		// exit(2), not exit_group(2): the leader thread ends and is shown
		// as a zombie, the process goes on.
		_, _, _ = syscall.Syscall(syscall.SYS_EXIT, 0, 0, 0)
	}
	os.Exit(m.Run())
}

// A worker whose leader thread ended with the rest of it running is not a
// group of zombies: seen in review as a space closed after the hangup with
// its worker still running. It gets the request to end and the kill.
func TestTheCloseSequenceKillsAWorkerWhoseLeaderThreadEnded(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `"$0" -test.run='^$' & echo $!; wait`, binary)
	cmd.Env = append(os.Environ(), threadWorkerEnv+"=1")
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
	line, err := bufio.NewReader(out).ReadString('\n')
	worker, convErr := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || convErr != nil {
		t.Fatalf("the worker did not start: %q, %v", line, err)
	}
	if !waitFor(5*time.Second, func() bool { return leaderEndedThreadsRun(worker) }) {
		t.Fatal("the worker's leader thread never ended with another running")
	}

	times := stopTimes{hangup: 100 * time.Millisecond, term: 100 * time.Millisecond, kill: 2 * time.Second}
	if took := stopGroup(cmd.Process.Pid, reaped, times); took != "SIGKILL" {
		t.Fatalf("a worker with a running thread ended after %q", took)
	}
	if leaderEndedThreadsRun(worker) {
		t.Fatal("a thread of the worker still runs")
	}
}

func waitFor(limit time.Duration, done func() bool) bool {
	for deadline := time.Now().Add(limit); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if done() {
			return true
		}
	}
	return false
}

// leaderEndedThreadsRun says whether pid's leader thread is a zombie while
// another of its threads is not.
func leaderEndedThreadsRun(pid int) bool {
	state := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		fields := strings.Fields(string(raw[strings.LastIndex(string(raw), ")")+1:]))
		if len(fields) == 0 {
			return ""
		}
		return fields[0]
	}
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	if state(filepath.Join(dir, "stat")) != "Z" {
		return false
	}
	tasks, _ := os.ReadDir(filepath.Join(dir, "task"))
	for _, task := range tasks {
		if s := state(filepath.Join(dir, "task", task.Name(), "stat")); s != "" && s != "Z" && s != "X" {
			return true
		}
	}
	return false
}

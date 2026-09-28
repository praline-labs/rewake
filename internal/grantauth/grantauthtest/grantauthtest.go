// Package grantauthtest gives tests a run of a recipient that confirms a
// grant at delivery and then ends: main's wrapper takes a grant for delivered
// only from the wrapper of the run it was granted to, so a test that needs
// such a grant handed over after a resume needs a process to be that run.
package grantauthtest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/proc"
)

// runEnv marks a test binary started again to be a run.
const runEnv = "REWAKE_GRANTAUTH_TEST_RUN"

// Delivery is what the run confirms, in whatever form the caller's own
// confirmation takes: each of Grants — a message and the directories it
// grants — once for each of Threads, in order, as deliveries retried in
// conversations the session named each time.
type Delivery struct {
	Dir       string
	Address   string
	MainPID   int
	MainStart uint64
	From      string
	FromEpoch string
	To        string
	Grants    map[string][]string
	Threads   []string
}

// Child is the run a test started with LiveRun: it names itself, confirms
// the delivery it is given, says how that went, and exits once the test is
// done with it. In any other process it returns at once. Call it first in
// TestMain.
func Child(confirm func(run, id, thread string, delivery Delivery) error) {
	if os.Getenv(runEnv) == "" {
		return
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		fmt.Println("start time:", err)
		os.Exit(0)
	}
	run := fmt.Sprintf("%d.%d", os.Getpid(), start)
	fmt.Println(run)
	var delivery Delivery
	if err := json.NewDecoder(os.Stdin).Decode(&delivery); err != nil {
		fmt.Println("delivery:", err)
		os.Exit(0)
	}
	ids := slices.Sorted(maps.Keys(delivery.Grants))
	for _, thread := range delivery.Threads {
		for _, id := range ids {
			if err := confirm(run, id, thread, delivery); err != nil {
				fmt.Println(strings.ReplaceAll(err.Error(), "\n", " "))
				os.Exit(0)
			}
		}
	}
	fmt.Println("ok")
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

// LiveRun starts this test binary again as a run, has register see that run
// before anything is confirmed — a grant names its recipient's run — and has
// the run confirm the delivery. The run stays up until end is called, and is
// ended when the test is.
func LiveRun(t *testing.T, register func(run string), delivery Delivery) (run string, end func()) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^$")
	command.Env = append(os.Environ(), runEnv+"=1")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	var ended error
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = stdin.Close()
			ended = command.Wait()
		})
	}
	t.Cleanup(func() { _ = command.Process.Kill(); stop() })
	lines := bufio.NewReader(stdout)
	run, _ = lines.ReadString('\n')
	run = strings.TrimSpace(run)
	if _, _, found := strings.Cut(run, "."); !found {
		t.Fatalf("the run did not name itself: %q", run)
	}
	register(run)
	if err := json.NewEncoder(stdin).Encode(delivery); err != nil {
		t.Fatal(err)
	}
	outcome, _ := lines.ReadString('\n')
	if outcome = strings.TrimSpace(outcome); outcome != "ok" {
		t.Fatalf("the run %s did not confirm its grants: %s", run, outcome)
	}
	return run, func() {
		t.Helper()
		stop()
		if ended != nil {
			t.Fatalf("the run %s: %v", run, ended)
		}
	}
}

// EndedRun is LiveRun for a run that has ended by the time it returns.
func EndedRun(t *testing.T, register func(run string), delivery Delivery) string {
	t.Helper()
	run, end := LiveRun(t, register, delivery)
	end()
	return run
}

package workflow

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Cases run side by side, and every one of them is a descendant of this
// process. These are the two ways that goes wrong without owner labels: one
// case's cleanup ends a neighbour's work, and a sweep's Wait4 takes the exit
// status a neighbour's own Wait is there to collect.

// escapeScript starts a descendant that leaves its group through setsid,
// writes its pid, and exits with the code given, leaving the descendant
// running.
const escapeScript = `setsid /bin/sh -c 'echo $$ > "$1"; exec sleep 30' sh "$1" >/dev/null 2>&1 & ` +
	`while [ ! -s "$1" ]; do sleep .01; done; exit "$2"`

func TestACaseSweepLeavesItsNeighbourAlone(t *testing.T) {
	dir := t.TempDir()
	minePID, theirsPID := filepath.Join(dir, "mine"), filepath.Join(dir, "theirs")
	mine, err := startGroup("mine", newOwnerLabel(), exec.Command("/bin/sh", "-c", escapeScript, "sh", minePID, "0"))
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := startGroup("theirs", newOwnerLabel(), exec.Command("/bin/sh", "-c", escapeScript, "sh", theirsPID, "3"))
	if err != nil {
		t.Fatal(err)
	}
	mineEscaped, theirsEscaped := readPID(t, minePID), readPID(t, theirsPID)
	defer reap(mineEscaped)
	defer reap(theirsEscaped)

	left := terminate(mine, mine.label, nil)
	if len(left) == 0 {
		t.Error("the sweep named nothing, though its own escaped descendant was running")
	}
	if processRunning(mineEscaped) {
		t.Error("the sweep left its own escaped descendant running")
	}
	if !processRunning(theirsEscaped) {
		t.Fatal("one case's sweep ended its neighbour's escaped descendant")
	}

	// The neighbour's exit status reaches its own Wait, whatever the sweep
	// buried meanwhile. Asked before the neighbour's own cleanup, whose
	// reapGroup may collect its leader itself (owner_labels_test.go).
	var exit *exec.ExitError
	if err := theirs.wait(); !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Errorf("the neighbour's Wait answered %v, want its exit status 3", err)
	}
	terminate(theirs, theirs.label, nil)
	if processRunning(theirsEscaped) {
		t.Error("the neighbour's own sweep left its escaped descendant running")
	}
}

// What no case's label names is left to the sweep after every case, which
// ends it and names it: the guarantee that nothing outlives the run.
func TestTheFinalSweepTakesWhatNoCaseOwns(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "pid")
	// Started without startGroup, so it carries no label: a process a scenario
	// started around the case's accounting.
	if err := exec.Command("/bin/sh", "-c", escapeScript, "sh", pidPath, "0").Run(); err != nil {
		t.Fatal(err)
	}
	escaped := readPID(t, pidPath)
	defer reap(escaped)

	if left := terminate(nil, newOwnerLabel(), nil); len(left) != 0 {
		t.Errorf("a labeled sweep took what its label does not name: %v", left)
	}
	if !processRunning(escaped) {
		t.Fatal("a labeled sweep ended an unlabeled process")
	}
	// Only this process: anything else it named was left by an earlier test,
	// and ending it here must not hide that from the run.
	left := finalSweep()
	own := fmt.Sprintf("pid %d ", escaped)
	if !slices.ContainsFunc(left, func(problem string) bool { return strings.HasPrefix(problem, own) }) {
		t.Error("the final sweep did not name the unlabeled process")
	}
	for _, problem := range left {
		if !strings.HasPrefix(problem, own) {
			t.Errorf("the final sweep found what an earlier test left: %s", problem)
		}
	}
	if processRunning(escaped) {
		t.Error("the final sweep left the unlabeled process running")
	}
}

// plantLeakEnv makes TestAPlantedLeak leave a process behind, carrying the
// value in its command line. Only tools/checksummary sets it, to see a run
// that fails on a leftover name the leftover in its summary; unset, the test
// skips, so the suite never plants one on its own.
const plantLeakEnv = "REWAKE_WORKFLOW_PLANT_LEAK"

func TestAPlantedLeak(t *testing.T) {
	marker := os.Getenv(plantLeakEnv)
	if marker == "" {
		t.Skipf("nothing planted: %s is set only by the summarizer's test", plantLeakEnv)
	}
	// Started around every case's accounting and never waited for: what a
	// scenario that forgot its cleanup would leave, for the final sweep alone.
	leak := exec.Command("/bin/sh", "-c", "sleep 30; exit 0", marker)
	if err := leak.Start(); err != nil {
		t.Fatal(err)
	}
}

// go test's own check of -parallel never runs, because the pool lifts the
// limit first; a width of none would hold every case until -timeout.
func TestAPoolOfNoneIsRefused(t *testing.T) {
	width := flag.Lookup("test.parallel")
	before, size := width.Value.String(), pool.size
	defer func() { _ = width.Value.Set(before) }()
	for _, value := range []string{"0", "-2"} {
		// Through the flag set, so the pool sees it as given on the command line.
		if err := flag.Set("test.parallel", value); err != nil {
			t.Fatal(err)
		}
		if err := takePoolWidth(); err == nil {
			t.Errorf("-parallel %s was taken", value)
		}
		if pool.size != size {
			t.Errorf("-parallel %s changed the pool to %d", value, pool.size)
		}
	}
}

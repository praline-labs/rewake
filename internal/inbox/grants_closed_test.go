package inbox

import (
	"os"
	"os/exec"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
)

// runOf names a run of a process: this one while the test runs, or one that
// has exited.
func runOf(t *testing.T, live bool) string {
	t.Helper()
	pid := os.Getpid()
	if !live {
		child := exec.Command("true")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		pid = child.Process.Pid
		start, err := proc.StartTime(pid)
		_ = child.Wait()
		if err != nil {
			start = 1
		}
		return strconv.Itoa(pid) + "." + strconv.FormatUint(start, 10)
	}
	start, err := proc.StartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(pid) + "." + strconv.FormatUint(start, 10)
}

// A task not read yet is open only while the run it was written for runs: a
// run resuming the conversation cannot read it, and its sender is told no
// report is coming, so main lets its grant go rather than hand it to a resume
// that will never see the task.
func TestAnUnreadTaskOfAnEndedRunIsClosed(t *testing.T) {
	dir := stateDir(t)
	for _, live := range []bool{true, false} {
		for _, delivered := range []bool{false, true} {
			m := message("task")
			m.Kind, m.FromEpoch, m.ToEpoch, m.GrantDirs = Task, "sender-epoch", runOf(t, live), []string{"/work/lib"}
			if err := Put(dir, m); err != nil {
				t.Fatal(err)
			}
			if delivered {
				if err := linkUnread(dir, "api", m.ID); err != nil {
					t.Fatal(err)
				}
				if err := writeStatus(dir, "api", m.ID, Result{State: Delivered}); err != nil {
					t.Fatal(err)
				}
			}
			if open, found := TaskOpen(dir, "api", m.ID); open != live || !found {
				t.Errorf("live %v, delivered %v: open %v found %v", live, delivered, open, found)
			}
		}
	}
}

// A status saying the task was taken back, or never arrived, closes it
// whatever a wait record says: no reader read it, and a wait naming it is
// one a worker wrote to keep its grant.
func TestATaskTakenBackOrFailedIsClosedWhateverTheWaitsSay(t *testing.T) {
	dir := stateDir(t)
	tasks := grantPending(t, dir, true, true)
	for index, result := range []Result{{State: Failed, Detail: "taken back", Withdrawn: true}, {State: Failed, Detail: "the notice was refused"}} {
		task := tasks[index]
		readIn(t, dir, "receiver-epoch", "thread-a", task)
		if err := writeStatus(dir, "api", task.ID, result); err != nil {
			t.Fatal(err)
		}
		if open, found := TaskOpen(dir, "api", task.ID); open || !found {
			t.Errorf("%s: open %v found %v", result.Detail, open, found)
		}
		if !Settled(dir, "api", task.ID) {
			t.Errorf("%s: not settled", result.Detail)
		}
	}
}

// A task whose delivery failed after its grant was taken is settled: the
// grant a hook holds for it goes back at once, rather than when the status
// is swept a day later.
func TestAFailedDeliveryIsSettled(t *testing.T) {
	dir := stateDir(t)
	task := grantPending(t, dir, true)[0]
	if err := linkUnread(dir, "api", task.ID); err != nil {
		t.Fatal(err)
	}
	if Settled(dir, "api", task.ID) {
		t.Fatal("a task on its way is settled")
	}
	if err := writeStatus(dir, "api", task.ID, Result{State: Failed, Detail: "the notice was refused"}); err != nil {
		t.Fatal(err)
	}
	if !Settled(dir, "api", task.ID) {
		t.Error("a failed delivery is not settled")
	}
}

// Taking a wait over does not start the resume window again: counted from
// the first reading, a chain of resumes cannot keep a task, and its grant,
// owed past a day.
func TestAWaitTakenOverKeepsItsReading(t *testing.T) {
	dir := stateDir(t)
	task := grantPending(t, dir, true)[0]
	readIn(t, dir, "receiver-epoch", "thread-a", task)
	agedWaits(t, dir, "receiver-epoch", resumeWindow-time.Hour)
	if adopted := must(AdoptWaits(dir, "api", "second-epoch", "thread-a")); !slices.Equal(adopted, []string{task.ID}) {
		t.Fatalf("the first resume took over %v", adopted)
	}
	sweepAwaiting(dir, "api", "second-epoch")
	waits := Waiters(dir, "api", "second-epoch")
	if len(waits) != 1 || time.Since(time.Unix(0, waits[0].readAt(0))) < resumeWindow-time.Hour {
		t.Fatalf("the wait taken over was read at %+v", waits)
	}
	agedWaits(t, dir, "second-epoch", 2*time.Hour)
	if open, _ := TaskOpen(dir, "api", task.ID); open {
		t.Error("a day after it was read, the task is still open")
	}
	if adopted := must(AdoptWaits(dir, "api", "third-epoch", "thread-a")); len(adopted) != 0 {
		t.Errorf("a day after it was read, a second resume took over %v", adopted)
	}
}
